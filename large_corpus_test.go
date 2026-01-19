package kql

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// QueryEntry represents a single query from the corpus
type QueryEntry struct {
	Source string `json:"source"`
	Name   string `json:"name"`
	Query  string `json:"query"`
}

// TestLargeCorpusFuzz tests the KQL parser against a large corpus of real-world queries
func TestLargeCorpusFuzz(t *testing.T) {
	corpusPath := "testdata/corpus.json"

	// Check if corpus exists
	if _, err := os.Stat(corpusPath); os.IsNotExist(err) {
		t.Skip("Large corpus not available at testdata/corpus.json - run extraction script first")
	}

	// Load corpus
	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("Failed to read corpus: %v", err)
	}

	var queries []QueryEntry
	if err := json.Unmarshal(data, &queries); err != nil {
		t.Fatalf("Failed to parse corpus: %v", err)
	}

	t.Logf("Loaded %d queries from corpus", len(queries))

	// Statistics
	var (
		totalQueries     int64
		successCount     int64
		partialCount     int64
		failedCount      int64
		panicCount       int64
		totalConditions  int64
		totalParseTime   int64
		longestQuery int
	)

	// Process queries in parallel for speed
	numWorkers := 8
	queryChan := make(chan QueryEntry, 100)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for q := range queryChan {
				// Recover from panics
				func() {
					defer func() {
						if r := recover(); r != nil {
							atomic.AddInt64(&panicCount, 1)
						}
					}()

					start := time.Now()
					result := ExtractConditions(q.Query)
					elapsed := time.Since(start)

					atomic.AddInt64(&totalQueries, 1)
					atomic.AddInt64(&totalParseTime, int64(elapsed))

					if len(result.Conditions) > 0 && len(result.Errors) == 0 {
						atomic.AddInt64(&successCount, 1)
					} else if len(result.Conditions) > 0 {
						atomic.AddInt64(&partialCount, 1)
					} else {
						atomic.AddInt64(&failedCount, 1)
					}

					atomic.AddInt64(&totalConditions, int64(len(result.Conditions)))
				}()

				if len(q.Query) > longestQuery {
					longestQuery = len(q.Query)
				}
			}
		}()
	}

	// Feed queries to workers
	startTime := time.Now()
	for _, q := range queries {
		queryChan <- q
	}
	close(queryChan)
	wg.Wait()
	totalTime := time.Since(startTime)

	// Report results
	t.Logf("\n=== Large Corpus Fuzz Test Results ===")
	t.Logf("Total queries tested: %d", totalQueries)
	t.Logf("Full success (conditions, no errors): %d (%.1f%%)", successCount, float64(successCount)*100/float64(totalQueries))
	t.Logf("Partial success (conditions with errors): %d (%.1f%%)", partialCount, float64(partialCount)*100/float64(totalQueries))
	t.Logf("Failed (no conditions): %d (%.1f%%)", failedCount, float64(failedCount)*100/float64(totalQueries))
	t.Logf("Panics: %d", panicCount)
	t.Logf("Total conditions extracted: %d", totalConditions)
	t.Logf("Average conditions per query: %.2f", float64(totalConditions)/float64(totalQueries))
	t.Logf("Total parse time: %v", totalTime)
	t.Logf("Average parse time per query: %v", time.Duration(totalParseTime/totalQueries))
	t.Logf("Queries per second: %.0f", float64(totalQueries)/totalTime.Seconds())
	t.Logf("Longest query: %d characters", longestQuery)

	// Fail if we have panics
	if panicCount > 0 {
		t.Errorf("Parser panicked on %d queries - this must be fixed!", panicCount)
	}

	// Warn if success rate is too low
	successRate := float64(successCount+partialCount) * 100 / float64(totalQueries)
	t.Logf("\nOverall success rate: %.1f%%", successRate)
}

// TestLargeCorpusSample tests a random sample of queries for faster iteration
func TestLargeCorpusSample(t *testing.T) {
	corpusPath := "testdata/corpus.json"

	if _, err := os.Stat(corpusPath); os.IsNotExist(err) {
		t.Skip("Large corpus not available")
	}

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("Failed to read corpus: %v", err)
	}

	var queries []QueryEntry
	if err := json.Unmarshal(data, &queries); err != nil {
		t.Fatalf("Failed to parse corpus: %v", err)
	}

	// Test every 100th query for a quick sample
	sampleSize := len(queries) / 100
	if sampleSize > 1000 {
		sampleSize = 1000
	}

	t.Logf("Testing sample of %d queries (from %d total)", sampleSize, len(queries))

	var success, partial, failed, panics int
	step := len(queries) / sampleSize

	for i := 0; i < len(queries); i += step {
		q := queries[i]

		func() {
			defer func() {
				if r := recover(); r != nil {
					panics++
					t.Logf("PANIC on query %s: %v", q.Name, r)
				}
			}()

			result := ExtractConditions(q.Query)

			if len(result.Conditions) > 0 && len(result.Errors) == 0 {
				success++
			} else if len(result.Conditions) > 0 {
				partial++
			} else {
				failed++
			}
		}()
	}

	total := success + partial + failed + panics
	t.Logf("Sample results: success=%d (%.1f%%), partial=%d (%.1f%%), failed=%d (%.1f%%), panics=%d",
		success, float64(success)*100/float64(total),
		partial, float64(partial)*100/float64(total),
		failed, float64(failed)*100/float64(total),
		panics)

	if panics > 0 {
		t.Errorf("Parser panicked on %d queries!", panics)
	}
}

// TestLargeCorpusBySource tests queries grouped by source to identify problematic sources
func TestLargeCorpusBySource(t *testing.T) {
	corpusPath := "testdata/corpus.json"

	if _, err := os.Stat(corpusPath); os.IsNotExist(err) {
		t.Skip("Large corpus not available")
	}

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("Failed to read corpus: %v", err)
	}

	var queries []QueryEntry
	if err := json.Unmarshal(data, &queries); err != nil {
		t.Fatalf("Failed to parse corpus: %v", err)
	}

	// Group queries by source repo
	bySource := make(map[string][]QueryEntry)
	for _, q := range queries {
		// Extract repo name from source path
		source := "unknown"
		if strings.Contains(q.Source, "kql_corpus/") {
			parts := strings.Split(q.Source, "kql_corpus/")
			if len(parts) > 1 {
				subparts := strings.Split(parts[1], "/")
				if len(subparts) > 0 {
					source = subparts[0]
				}
			}
		}
		bySource[source] = append(bySource[source], q)
	}

	t.Logf("Queries by source:")
	for source, qs := range bySource {
		var success, partial, failed int
		for _, q := range qs {
			result := ExtractConditions(q.Query)
			if len(result.Conditions) > 0 && len(result.Errors) == 0 {
				success++
			} else if len(result.Conditions) > 0 {
				partial++
			} else {
				failed++
			}
		}
		total := success + partial + failed
		successRate := float64(success+partial) * 100 / float64(total)
		t.Logf("  %s: %d queries, %.1f%% success", source, total, successRate)
	}
}

// BenchmarkLargeCorpus benchmarks parsing speed on the large corpus
func BenchmarkLargeCorpus(b *testing.B) {
	corpusPath := "testdata/corpus.json"

	if _, err := os.Stat(corpusPath); os.IsNotExist(err) {
		b.Skip("Large corpus not available")
	}

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		b.Fatalf("Failed to read corpus: %v", err)
	}

	var queries []QueryEntry
	if err := json.Unmarshal(data, &queries); err != nil {
		b.Fatalf("Failed to parse corpus: %v", err)
	}

	// Use a subset for benchmarking
	sampleSize := 1000
	if len(queries) < sampleSize {
		sampleSize = len(queries)
	}
	sample := queries[:sampleSize]

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, q := range sample {
			_ = ExtractConditions(q.Query)
		}
	}
}

// TestFindFailingQueries identifies specific queries that fail to parse
func TestFindFailingQueries(t *testing.T) {
	corpusPath := "testdata/corpus.json"

	if _, err := os.Stat(corpusPath); os.IsNotExist(err) {
		t.Skip("Large corpus not available")
	}

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("Failed to read corpus: %v", err)
	}

	var queries []QueryEntry
	if err := json.Unmarshal(data, &queries); err != nil {
		t.Fatalf("Failed to parse corpus: %v", err)
	}

	// Collect unique error types
	errorTypes := make(map[string]int)
	var failedQueries []QueryEntry

	for _, q := range queries {
		result := ExtractConditions(q.Query)
		if len(result.Conditions) == 0 && len(result.Errors) > 0 {
			failedQueries = append(failedQueries, q)
			for _, err := range result.Errors {
				// Extract error type (first few words)
				words := strings.Fields(err)
				if len(words) > 3 {
					errorType := strings.Join(words[:3], " ")
					errorTypes[errorType]++
				}
			}
		}
	}

	t.Logf("Found %d completely failed queries", len(failedQueries))
	t.Logf("\nTop error types:")
	for errType, count := range errorTypes {
		if count > 10 {
			t.Logf("  %s: %d occurrences", errType, count)
		}
	}

	// Show a few example failures
	if len(failedQueries) > 0 {
		t.Logf("\nExample failed queries:")
		for i, q := range failedQueries {
			if i >= 5 {
				break
			}
			truncated := q.Query
			if len(truncated) > 100 {
				truncated = truncated[:100] + "..."
			}
			t.Logf("  [%s] %s", q.Name, truncated)
		}
	}
}
