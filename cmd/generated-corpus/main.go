package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	kql "github.com/craftedsignal/kql-parser"
	sigma "github.com/craftedsignal/sigma-parser"
	"gopkg.in/yaml.v3"
)

type generatedCase struct {
	ID       int64               `json:"id"`
	Seed     int64               `json:"seed"`
	Query    string              `json:"query"`
	Expected []expectedCondition `json:"expected"`
}

type expectedCondition struct {
	Field        string   `json:"field"`
	Operator     string   `json:"operator"`
	Value        string   `json:"value,omitempty"`
	Alternatives []string `json:"alternatives,omitempty"`
	Negated      bool     `json:"negated,omitempty"`
}

type queryEntry struct {
	Source string `json:"source"`
	Name   string `json:"name"`
	Query  string `json:"query"`
}

type runResult struct {
	ID        int64       `json:"id"`
	Query     string      `json:"query,omitempty"`
	SigmaYAML string      `json:"sigma_yaml,omitempty"`
	BackKQL   string      `json:"back_kql,omitempty"`
	Entry     *queryEntry `json:"entry,omitempty"`
	Failure   *failure    `json:"failure,omitempty"`
}

type failure struct {
	Stage    string   `json:"stage"`
	Reason   string   `json:"reason"`
	Errors   []string `json:"errors,omitempty"`
	Expected []string `json:"expected,omitempty"`
	Actual   []string `json:"actual,omitempty"`
	Missing  []string `json:"missing,omitempty"`
	Extra    []string `json:"extra,omitempty"`
}

type summary struct {
	Total             int64         `json:"total"`
	Passed            int64         `json:"passed"`
	Failed            int64         `json:"failed"`
	DuplicateQueries  int64         `json:"duplicate_queries"`
	KQLParseErrors    int64         `json:"kql_parse_errors"`
	ExpectedMismatch  int64         `json:"expected_mismatch"`
	SigmaParseErrors  int64         `json:"sigma_parse_errors"`
	SigmaMismatch     int64         `json:"sigma_mismatch"`
	BackKQLParseError int64         `json:"back_kql_parse_errors"`
	BackKQLMismatch   int64         `json:"back_kql_mismatch"`
	Duration          time.Duration `json:"duration"`
}

type corpusWriter struct {
	file  *os.File
	enc   *json.Encoder
	first bool
}

type failureWriter struct {
	file *os.File
	enc  *json.Encoder
}

type sigmaRule struct {
	Title     string         `yaml:"title"`
	Status    string         `yaml:"status"`
	Logsource sigmaLogsource `yaml:"logsource"`
	Detection map[string]any `yaml:"detection"`
	Fields    []string       `yaml:"fields,omitempty"`
}

type sigmaLogsource struct {
	Category string `yaml:"category,omitempty"`
	Product  string `yaml:"product,omitempty"`
	Service  string `yaml:"service,omitempty"`
}

type sigmaSelection struct {
	Raw     any
	Negated bool
	Name    string
}

type normCondition struct {
	Field   string
	Op      string
	Value   string
	Negated bool
}

var (
	tables = []string{"SecurityEvent", "DeviceProcessEvents", "DeviceNetworkEvents", "SigninLogs", "AuditLogs"}
	fields = []string{
		"EventID", "EventCode", "ActionType", "Status", "ResultType", "AccountName",
		"TargetUserName", "UserPrincipalName", "FileName", "ProcessCommandLine",
		"CommandLine", "InitiatingProcessFileName", "DeviceName", "Computer",
		"RemoteIP", "LocalIP", "IPAddress", "RemotePort", "LocalPort", "FolderPath",
		"SHA256", "AppDisplayName", "Location", "Message", "Count", "Duration", "Bytes",
	}
	numericFields = []string{"EventID", "EventCode", "RemotePort", "LocalPort", "Count", "Duration", "Bytes", "Level", "FailedAttempts"}
	stringFields  = []string{"ActionType", "Status", "ResultType", "AccountName", "TargetUserName", "UserPrincipalName", "FileName", "ProcessCommandLine", "CommandLine", "InitiatingProcessFileName", "DeviceName", "Computer", "RemoteIP", "LocalIP", "IPAddress", "FolderPath", "SHA256", "AppDisplayName", "Location", "Message"}
)

func main() {
	var (
		total       = flag.Int64("n", 1_000_000, "number of generated KQL queries to test")
		seed        = flag.Int64("seed", 4242, "base random seed; use a negative value for crypto-random choices")
		workers     = flag.Int("workers", runtime.NumCPU(), "parallel worker count")
		corpusPath  = flag.String("corpus", "testdata/generated/kql_sigma_roundtrip_corpus.json", "verified generated corpus JSON array path; empty disables writing")
		failPath    = flag.String("failures", "testdata/generated/kql_sigma_roundtrip_failures.jsonl", "failure JSONL path; empty disables writing")
		failLimit   = flag.Int("failure-limit", 1000, "maximum failure records to write")
		progress    = flag.Int64("progress", 10000, "print progress every N completed cases")
		strict      = flag.Bool("strict", false, "exit non-zero if any generated case fails")
		stopOnFirst = flag.Bool("stop-on-first", false, "stop scheduling new work after first failure")
		unique      = flag.Bool("unique", false, "write only unique query strings; keep generating until n unique verified queries are written")
	)
	flag.Parse()

	if *total < 0 {
		fatalf("-n must be non-negative")
	}
	if *workers <= 0 {
		*workers = 1
	}

	cw, err := newCorpusWriter(*corpusPath)
	if err != nil {
		fatalf("open corpus writer: %v", err)
	}
	defer cw.close()

	fw, err := newFailureWriter(*failPath)
	if err != nil {
		fatalf("open failure writer: %v", err)
	}
	defer fw.close()

	start := time.Now()
	s, failureRecords := runGenerated(*seed, *total, *workers, *failLimit, *progress, *stopOnFirst, *unique, cw, fw, start)
	printSummary(s, *corpusPath, *failPath, failureRecords)
	if *strict && s.Failed > 0 {
		os.Exit(1)
	}
}

func runGenerated(seed, target int64, workers, failLimit int, progress int64, stopOnFirst, unique bool, cw *corpusWriter, fw *failureWriter, start time.Time) (summary, int) {
	if unique {
		return runGeneratedUnique(seed, target, workers, failLimit, progress, stopOnFirst, cw, fw, start)
	}
	return runGeneratedFixed(seed, target, workers, failLimit, progress, stopOnFirst, cw, fw, start)
}

func runGeneratedFixed(seed, total int64, workers, failLimit int, progress int64, stopOnFirst bool, cw *corpusWriter, fw *failureWriter, start time.Time) (summary, int) {
	jobs := make(chan int64, workers*2)
	results := make(chan runResult, workers*2)
	done := make(chan struct{})
	var doneOnce sync.Once
	stop := func() { doneOnce.Do(func() { close(done) }) }

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				select {
				case <-done:
					return
				default:
				}
				results <- processID(seed, id)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for i := int64(0); i < total; i++ {
			select {
			case <-done:
				return
			case jobs <- i:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	var s summary
	var failureRecords int
	for result := range results {
		acceptResult(result, &s, &failureRecords, failLimit, cw, fw)
		if result.Failure != nil && stopOnFirst {
			stop()
		}
		if progress > 0 && s.Total%progress == 0 {
			printProgress(s, total, start)
		}
		if stopOnFirst && s.Failed > 0 {
			break
		}
	}
	s.Duration = time.Since(start)
	return s, failureRecords
}

func runGeneratedUnique(seed, target int64, workers, failLimit int, progress int64, stopOnFirst bool, cw *corpusWriter, fw *failureWriter, start time.Time) (summary, int) {
	jobs := make(chan int64, workers*2)
	results := make(chan runResult, workers*2)
	seen := make(map[[32]byte]struct{}, mapCapacityHint(target))

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				results <- processID(seed, id)
			}
		}()
	}

	var s summary
	var failureRecords int
	nextID := int64(0)
	active := 0
	maxActive := workers * 2
	jobsClosed := false
	closeJobs := func() {
		if !jobsClosed {
			close(jobs)
			jobsClosed = true
		}
	}
	schedule := func() {
		for !jobsClosed && active < maxActive && s.Passed+int64(active) < target {
			jobs <- nextID
			nextID++
			active++
		}
	}

	schedule()
	for active > 0 {
		result := <-results
		active--
		s.Total++
		if result.Failure == nil {
			hash := sha256.Sum256([]byte(result.Query))
			if _, ok := seen[hash]; ok {
				s.DuplicateQueries++
			} else {
				seen[hash] = struct{}{}
				s.Passed++
				if result.Entry != nil {
					if err := cw.write(*result.Entry); err != nil {
						closeJobs()
						fatalf("write corpus: %v", err)
					}
				}
			}
		} else {
			recordFailure(result, &s, &failureRecords, failLimit, fw)
			if stopOnFirst {
				closeJobs()
			}
		}

		if !jobsClosed {
			schedule()
		}
		if s.Passed == target {
			closeJobs()
		}
		if progress > 0 && (s.Total%progress == 0 || s.Passed == target) {
			printProgress(s, target, start)
		}
	}
	closeJobs()
	wg.Wait()
	s.Duration = time.Since(start)
	return s, failureRecords
}

func acceptResult(result runResult, s *summary, failureRecords *int, failLimit int, cw *corpusWriter, fw *failureWriter) {
	s.Total++
	if result.Failure == nil {
		s.Passed++
		if result.Entry != nil {
			if err := cw.write(*result.Entry); err != nil {
				fatalf("write corpus: %v", err)
			}
		}
		return
	}
	recordFailure(result, s, failureRecords, failLimit, fw)
}

func recordFailure(result runResult, s *summary, failureRecords *int, failLimit int, fw *failureWriter) {
	s.Failed++
	countFailureStage(s, result.Failure.Stage)
	if *failureRecords < failLimit {
		if err := fw.write(result); err != nil {
			fatalf("write failure: %v", err)
		}
		(*failureRecords)++
	}
}

func processCase(tc generatedCase) runResult {
	result := runResult{ID: tc.ID, Query: tc.Query}

	kqlResult := kql.ExtractConditions(tc.Query)
	if kqlResult == nil {
		result.Failure = &failure{Stage: "kql_parse", Reason: "ExtractConditions returned nil"}
		return result
	}
	if len(kqlResult.Errors) > 0 {
		result.Failure = &failure{Stage: "kql_parse", Reason: "KQL parser returned errors", Errors: kqlResult.Errors}
		return result
	}

	expected := normalizeExpected(tc.Expected)
	actualKQL := normalizeKQLConditions(flattenKQLConditions(kqlResult))
	if missing, extra := compareConditionSets(expected, actualKQL); len(missing) > 0 || len(extra) > 0 {
		result.Failure = &failure{
			Stage:    "expected_mismatch",
			Reason:   "KQL parser extraction differed from generated semantic oracle",
			Expected: conditionKeys(expected),
			Actual:   conditionKeys(actualKQL),
			Missing:  conditionKeys(missing),
			Extra:    conditionKeys(extra),
		}
		return result
	}

	sigmaYAML, err := kqlResultToSigmaYAML(tc.ID, kqlResult)
	if err != nil {
		result.Failure = &failure{Stage: "sigma_build", Reason: err.Error()}
		return result
	}
	result.SigmaYAML = sigmaYAML

	sigmaResult := sigma.ExtractConditions(sigmaYAML)
	if sigmaResult == nil {
		result.Failure = &failure{Stage: "sigma_parse", Reason: "Sigma ExtractConditions returned nil"}
		return result
	}
	if len(sigmaResult.Errors) > 0 {
		result.Failure = &failure{Stage: "sigma_parse", Reason: "Sigma parser returned errors", Errors: sigmaResult.Errors}
		return result
	}

	actualSigma := normalizeSigmaConditions(sigmaResult.Conditions)
	if missing, extra := compareConditionSets(actualKQL, actualSigma); len(missing) > 0 || len(extra) > 0 {
		result.Failure = &failure{
			Stage:    "sigma_mismatch",
			Reason:   "KQL parse result and Sigma parser result differ",
			Expected: conditionKeys(actualKQL),
			Actual:   conditionKeys(actualSigma),
			Missing:  conditionKeys(missing),
			Extra:    conditionKeys(extra),
		}
		return result
	}

	backKQL, err := sigmaResultToKQL(sigmaResult)
	if err != nil {
		result.Failure = &failure{Stage: "back_kql_build", Reason: err.Error()}
		return result
	}
	result.BackKQL = backKQL

	backResult := kql.ExtractConditions(backKQL)
	if backResult == nil {
		result.Failure = &failure{Stage: "back_kql_parse", Reason: "KQL parser returned nil for back-converted KQL"}
		return result
	}
	if len(backResult.Errors) > 0 {
		result.Failure = &failure{Stage: "back_kql_parse", Reason: "KQL parser returned errors for back-converted KQL", Errors: backResult.Errors}
		return result
	}
	actualBack := normalizeKQLConditions(flattenKQLConditions(backResult))
	if missing, extra := compareConditionSets(actualSigma, actualBack); len(missing) > 0 || len(extra) > 0 {
		result.Failure = &failure{
			Stage:    "back_kql_mismatch",
			Reason:   "Sigma parser result and back-converted KQL parser result differ",
			Expected: conditionKeys(actualSigma),
			Actual:   conditionKeys(actualBack),
			Missing:  conditionKeys(missing),
			Extra:    conditionKeys(extra),
		}
		return result
	}

	result.Entry = &queryEntry{
		Source: "generated_kql_sigma_roundtrip",
		Name:   fmt.Sprintf("generated_%09d", tc.ID),
		Query:  tc.Query,
	}
	return result
}

func processID(seed, id int64) (result runResult) {
	result = runResult{ID: id}
	defer func() {
		if r := recover(); r != nil {
			result.Failure = &failure{Stage: "generator_panic", Reason: fmt.Sprintf("panic while generating or processing case: %v", r)}
		}
	}()
	return processCase(generateCase(seed, id))
}

func generateCase(seed, id int64) generatedCase {
	return generateBoundedSemanticCase(seed, id)
}

func generateSimpleSearch(rng *rand.Rand) (string, []expectedCondition) {
	table := oneOf(rng, tables...)
	conds := make([]expectedCondition, 0, rng.Intn(4)+2)
	for i := 0; i < cap(conds); i++ {
		conds = append(conds, randomCondition(rng))
	}
	return fmt.Sprintf("%s | where %s", table, joinExpected(conds, "and")), conds
}

func generateBooleanSearch(rng *rand.Rand) (string, []expectedCondition) {
	table := oneOf(rng, tables...)
	field := oneOf(rng, stringFields...)
	values := uniqueValues(rng, valuesForField(field), rng.Intn(3)+2)
	orConds := make([]expectedCondition, 0, len(values))
	for _, value := range values {
		orConds = append(orConds, expectedCondition{Field: field, Operator: "==", Value: value})
	}
	left := "(" + joinExpected(orConds, "or") + ")"

	right := randomCondition(rng)
	expected := append([]expectedCondition(nil), orConds...)
	expected = append(expected, right)

	if rng.Intn(4) == 0 {
		for i := range orConds {
			orConds[i].Negated = true
		}
		expected = append([]expectedCondition(nil), orConds...)
		expected = append(expected, right)
		left = "not(" + left + ")"
	}

	query := fmt.Sprintf("%s | where %s and %s", table, left, renderCondition(right))
	return query, expected
}

func generatePipelineSearch(rng *rand.Rand) (string, []expectedCondition) {
	table := oneOf(rng, tables...)
	first := randomCondition(rng)
	second := randomCondition(rng)
	third := randomCondition(rng)
	query := fmt.Sprintf("%s | where %s | extend Computed_%d = strcat(%s, \"_x\") | where %s | project TimeGenerated, %s, %s | where %s | take %d",
		table, renderCondition(first), rng.Intn(1000), oneOf(rng, stringFields...), renderCondition(second), first.Field, second.Field, renderCondition(third), rng.Intn(500)+1)
	return query, []expectedCondition{first, second, third}
}

func generateJoinSearch(rng *rand.Rand) (string, []expectedCondition) {
	main := randomCondition(rng)
	sub := randomCondition(rng)
	if main.Field == "DeviceId" || sub.Field == "DeviceId" {
		main = randomCondition(rng)
		sub = randomCondition(rng)
	}
	leftTable := oneOf(rng, "SecurityEvent", "DeviceNetworkEvents", "SigninLogs")
	rightTable := oneOf(rng, "DeviceProcessEvents", "DeviceNetworkEvents", "AuditLogs")
	query := fmt.Sprintf("%s | where %s | join kind=%s (%s | where %s | project DeviceId, %s) on DeviceId | project DeviceId, %s",
		leftTable, renderCondition(main), oneOf(rng, "inner", "leftouter", "innerunique"), rightTable, renderCondition(sub), sub.Field, main.Field)
	return query, []expectedCondition{main, sub}
}

func generateStringOperatorSearch(rng *rand.Rand) (string, []expectedCondition) {
	table := oneOf(rng, tables...)
	field := oneOf(rng, "CommandLine", "ProcessCommandLine", "Message", "FileName", "FolderPath", "AppDisplayName")
	a := expectedCondition{Field: field, Operator: oneOf(rng, "contains", "has", "startswith", "endswith"), Value: randomValueForField(rng, field)}
	b := expectedCondition{Field: field, Operator: oneOf(rng, "contains", "has", "startswith", "endswith"), Value: randomValueForField(rng, field), Negated: rng.Intn(2) == 0}
	query := fmt.Sprintf("%s | where %s or %s", table, renderCondition(a), renderCondition(b))
	return query, []expectedCondition{a, b}
}

func generateAggregationSearch(rng *rand.Rand) (string, []expectedCondition) {
	table := oneOf(rng, tables...)
	pre := randomCondition(rng)
	post := expectedCondition{Field: "FailedAttempts", Operator: ">", Value: strconv.Itoa(rng.Intn(20) + 3)}
	groupField := oneOf(rng, "AccountName", "TargetUserName", "DeviceName", "RemoteIP")
	query := fmt.Sprintf("%s | where %s | summarize FailedAttempts=count() by %s | where %s | sort by FailedAttempts desc",
		table, renderCondition(pre), groupField, renderCondition(post))
	return query, []expectedCondition{pre, post}
}

func generateFormattedSearch(rng *rand.Rand) (string, []expectedCondition) {
	query, expected := generatePipelineSearch(rng)
	query = strings.ReplaceAll(query, " | ", "\n| ")
	query = strings.ReplaceAll(query, " and ", "\n    and ")
	query = strings.ReplaceAll(query, " or ", "\n    or ")
	return query, expected
}

func randomCondition(rng *rand.Rand) expectedCondition {
	field := oneOf(rng, fields...)
	if contains(numericFields, field) {
		return expectedCondition{Field: field, Operator: oneOf(rng, "==", "!=", ">", "<", ">=", "<="), Value: strconv.Itoa(rng.Intn(9000) + 1)}
	}
	if rng.Intn(5) == 0 {
		values := uniqueValues(rng, valuesForField(field), rng.Intn(4)+2)
		return expectedCondition{Field: field, Operator: "==", Value: values[0], Alternatives: values, Negated: rng.Intn(4) == 0}
	}
	op := oneOf(rng, "==", "!=", "contains", "has", "startswith", "endswith")
	return expectedCondition{Field: field, Operator: op, Value: randomValueForField(rng, field)}
}

func renderCondition(cond expectedCondition) string {
	if len(cond.Alternatives) > 0 {
		if cond.Negated {
			return fmt.Sprintf("%s !in (%s)", cond.Field, joinKQLValues(cond.Alternatives))
		}
		return fmt.Sprintf("%s in (%s)", cond.Field, joinKQLValues(cond.Alternatives))
	}
	body := ""
	switch cond.Operator {
	case "contains", "has", "startswith", "endswith":
		body = fmt.Sprintf("%s %s %s", cond.Field, cond.Operator, formatKQLValue(cond.Value))
	default:
		body = fmt.Sprintf("%s %s %s", cond.Field, cond.Operator, formatKQLValue(cond.Value))
	}
	if cond.Negated {
		return "not(" + body + ")"
	}
	return body
}

func joinExpected(conditions []expectedCondition, op string) string {
	parts := make([]string, 0, len(conditions))
	for _, cond := range conditions {
		parts = append(parts, renderCondition(cond))
	}
	return strings.Join(parts, " "+op+" ")
}

func kqlResultToSigmaYAML(id int64, result *kql.ParseResult) (string, error) {
	conditions := flattenKQLConditions(result)
	if len(conditions) == 0 {
		return "", fmt.Errorf("no KQL conditions to convert")
	}
	rule := sigmaRule{
		Title:     fmt.Sprintf("Generated KQL Roundtrip %d", id),
		Status:    "test",
		Logsource: inferLogsource(conditions),
		Detection: make(map[string]any, len(conditions)+1),
	}
	conditionParts := make([]string, 0, len(conditions)*2)
	fieldSet := make(map[string]bool)
	for i, cond := range conditions {
		selection := kqlConditionToSigmaSelection(cond, fmt.Sprintf("selection_%06d", i))
		rule.Detection[selection.Name] = selection.Raw
		if len(conditionParts) > 0 {
			op := strings.ToLower(cond.LogicalOp)
			if op != "or" {
				op = "and"
			}
			conditionParts = append(conditionParts, op)
		}
		ref := selection.Name
		if selection.Negated {
			ref = "not " + ref
		}
		conditionParts = append(conditionParts, ref)
		if cond.Field != "" {
			fieldSet[cond.Field] = true
		}
	}
	rule.Detection["condition"] = strings.Join(conditionParts, " ")
	for field := range fieldSet {
		rule.Fields = append(rule.Fields, field)
	}
	sort.Strings(rule.Fields)
	data, err := yaml.Marshal(rule)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func kqlConditionToSigmaSelection(cond kql.Condition, name string) sigmaSelection {
	field := cond.Field
	op, negated := canonicalKQLOperator(cond.Operator, cond.Negated)
	values := conditionValues(cond.Value, cond.Alternatives)
	if field == "" || op == "keyword" {
		if len(values) > 1 {
			return sigmaSelection{Name: name, Raw: values, Negated: negated}
		}
		return sigmaSelection{Name: name, Raw: first(values), Negated: negated}
	}
	key := field
	var value any = sigmaValue(values)
	switch op {
	case "=":
	case "contains":
		key += "|contains"
	case "startswith":
		key += "|startswith"
	case "endswith":
		key += "|endswith"
	case ">":
		key += "|gt"
	case ">=":
		key += "|gte"
	case "<":
		key += "|lt"
	case "<=":
		key += "|lte"
	default:
		key += "|" + op
	}
	return sigmaSelection{Name: name, Raw: map[string]any{key: value}, Negated: negated}
}

func sigmaResultToKQL(result *sigma.ParseResult) (string, error) {
	if result == nil || len(result.Conditions) == 0 {
		return "", fmt.Errorf("no Sigma conditions to convert")
	}
	parts := make([]string, 0, len(result.Conditions)*2)
	for i, cond := range result.Conditions {
		if i > 0 {
			logical := strings.ToLower(cond.LogicalOp)
			if logical != "or" {
				logical = "and"
			}
			parts = append(parts, logical)
		}
		expr := sigmaConditionToKQL(cond)
		if cond.Negated {
			expr = "not(" + expr + ")"
		}
		parts = append(parts, expr)
	}
	return "SecurityEvent | where " + strings.Join(parts, " "), nil
}

func sigmaConditionToKQL(cond sigma.Condition) string {
	values := conditionValues(cond.Value, cond.Alternatives)
	if cond.Field == "" || cond.Operator == "keyword" {
		return joinAlternativeExpressions(values, func(v string) string {
			return "* contains " + formatKQLValue(v)
		})
	}
	switch cond.Operator {
	case "=":
		if len(values) > 1 {
			return fmt.Sprintf("%s in (%s)", cond.Field, joinKQLValues(values))
		}
		return fmt.Sprintf("%s == %s", cond.Field, formatKQLValue(first(values)))
	case "contains":
		return joinAlternativeExpressions(values, func(v string) string {
			return fmt.Sprintf("%s contains %s", cond.Field, formatKQLValue(v))
		})
	case "startswith":
		return joinAlternativeExpressions(values, func(v string) string {
			return fmt.Sprintf("%s startswith %s", cond.Field, formatKQLValue(v))
		})
	case "endswith":
		return joinAlternativeExpressions(values, func(v string) string {
			return fmt.Sprintf("%s endswith %s", cond.Field, formatKQLValue(v))
		})
	case ">":
		return joinAlternativeExpressions(values, func(v string) string {
			return fmt.Sprintf("%s > %s", cond.Field, formatKQLValue(v))
		})
	case ">=":
		return joinAlternativeExpressions(values, func(v string) string {
			return fmt.Sprintf("%s >= %s", cond.Field, formatKQLValue(v))
		})
	case "<":
		return joinAlternativeExpressions(values, func(v string) string {
			return fmt.Sprintf("%s < %s", cond.Field, formatKQLValue(v))
		})
	case "<=":
		return joinAlternativeExpressions(values, func(v string) string {
			return fmt.Sprintf("%s <= %s", cond.Field, formatKQLValue(v))
		})
	default:
		return fmt.Sprintf("%s == %s", cond.Field, formatKQLValue(first(values)))
	}
}

func flattenKQLConditions(result *kql.ParseResult) []kql.Condition {
	if result == nil {
		return nil
	}
	conditions := append([]kql.Condition(nil), result.Conditions...)
	for _, join := range result.Joins {
		conditions = append(conditions, flattenKQLConditions(join.Subsearch)...)
	}
	return conditions
}

func normalizeExpected(conditions []expectedCondition) []normCondition {
	normalized := make([]normCondition, 0, len(conditions))
	for _, cond := range conditions {
		op, negated := canonicalKQLOperator(cond.Operator, cond.Negated)
		for _, value := range conditionValues(cond.Value, cond.Alternatives) {
			normalized = append(normalized, normalizeParts(cond.Field, op, value, negated))
		}
	}
	return dedupeNorm(normalized)
}

func normalizeKQLConditions(conditions []kql.Condition) []normCondition {
	normalized := make([]normCondition, 0, len(conditions))
	for _, cond := range conditions {
		op, negated := canonicalKQLOperator(cond.Operator, cond.Negated)
		for _, value := range conditionValues(cond.Value, cond.Alternatives) {
			normalized = append(normalized, normalizeParts(cond.Field, op, value, negated))
		}
	}
	return dedupeNorm(normalized)
}

func normalizeSigmaConditions(conditions []sigma.Condition) []normCondition {
	normalized := make([]normCondition, 0, len(conditions))
	for _, cond := range conditions {
		op := cond.Operator
		if op == "keyword" {
			op = "contains"
		}
		for _, value := range conditionValues(cond.Value, cond.Alternatives) {
			normalized = append(normalized, normalizeParts(cond.Field, op, value, cond.Negated))
		}
	}
	return dedupeNorm(normalized)
}

func canonicalKQLOperator(op string, negated bool) (string, bool) {
	switch strings.ToLower(op) {
	case "==", "=~", "=":
		return "=", negated
	case "!=", "!~":
		return "=", !negated
	case "contains", "contains_cs", "has", "has_cs":
		return "contains", negated
	case "!contains", "!contains_cs", "!has", "!has_cs":
		return "contains", !negated
	case "startswith", "startswith_cs":
		return "startswith", negated
	case "!startswith", "!startswith_cs":
		return "startswith", !negated
	case "endswith", "endswith_cs":
		return "endswith", negated
	case "!endswith", "!endswith_cs":
		return "endswith", !negated
	case "in":
		return "=", negated
	default:
		return op, negated
	}
}

func normalizeParts(field, op, value string, negated bool) normCondition {
	return normCondition{Field: strings.ToLower(field), Op: op, Value: canonicalValue(value), Negated: negated}
}

func canonicalValue(value string) string {
	for strings.Contains(value, `\\`) {
		value = strings.ReplaceAll(value, `\\`, `\`)
	}
	return value
}

func dedupeNorm(conditions []normCondition) []normCondition {
	seen := make(map[normCondition]bool, len(conditions))
	result := make([]normCondition, 0, len(conditions))
	for _, cond := range conditions {
		if !seen[cond] {
			seen[cond] = true
			result = append(result, cond)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return conditionKey(result[i]) < conditionKey(result[j])
	})
	return result
}

func compareConditionSets(expected, actual []normCondition) ([]normCondition, []normCondition) {
	actualSet := make(map[normCondition]bool, len(actual))
	for _, cond := range actual {
		actualSet[cond] = true
	}
	var missing []normCondition
	for _, cond := range expected {
		if !actualSet[cond] {
			missing = append(missing, cond)
		}
	}
	expectedSet := make(map[normCondition]bool, len(expected))
	for _, cond := range expected {
		expectedSet[cond] = true
	}
	var extra []normCondition
	for _, cond := range actual {
		if !expectedSet[cond] {
			extra = append(extra, cond)
		}
	}
	return missing, extra
}

func missingConditions(expected, actual []normCondition) []normCondition {
	missing, _ := compareConditionSets(expected, actual)
	return missing
}

func conditionKeys(conditions []normCondition) []string {
	keys := make([]string, len(conditions))
	for i, cond := range conditions {
		keys[i] = conditionKey(cond)
	}
	sort.Strings(keys)
	return keys
}

func conditionKey(cond normCondition) string {
	prefix := ""
	if cond.Negated {
		prefix = "!"
	}
	return prefix + cond.Field + "|" + cond.Op + "|" + cond.Value
}

func conditionValues(value string, alternatives []string) []string {
	if len(alternatives) > 0 {
		out := append([]string(nil), alternatives...)
		sort.Strings(out)
		return out
	}
	return []string{value}
}

func sigmaValue(values []string) any {
	if len(values) > 1 {
		return values
	}
	return first(values)
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func joinAlternativeExpressions(values []string, render func(string) string) string {
	if len(values) == 0 {
		return render("")
	}
	if len(values) == 1 {
		return render(values[0])
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, render(value))
	}
	return "(" + strings.Join(parts, " or ") + ")"
}

func formatKQLValue(value string) string {
	if isNumeric(value) || strings.EqualFold(value, "true") || strings.EqualFold(value, "false") {
		return value
	}
	return strconv.Quote(value)
}

func joinKQLValues(values []string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, formatKQLValue(value))
	}
	return strings.Join(out, ", ")
}

func inferLogsource(conditions []kql.Condition) sigmaLogsource {
	for _, cond := range conditions {
		field := strings.ToLower(cond.Field)
		if strings.Contains(field, "process") || strings.EqualFold(cond.Field, "FileName") || strings.EqualFold(cond.Field, "CommandLine") {
			return sigmaLogsource{Category: "process_creation", Product: "windows"}
		}
		if strings.Contains(field, "ip") || strings.Contains(field, "port") {
			return sigmaLogsource{Category: "network_connection", Product: "windows"}
		}
	}
	return sigmaLogsource{Category: "generic", Product: "windows"}
}

func valuesForField(field string) []string {
	switch field {
	case "FileName", "InitiatingProcessFileName":
		return []string{"cmd.exe", "powershell.exe", "pwsh.exe", "rundll32.exe", "mshta.exe", "svchost.exe"}
	case "CommandLine", "ProcessCommandLine":
		return []string{"-enc", "downloadstring", "whoami", "net user", " /c ", "Invoke-WebRequest"}
	case "RemoteIP", "LocalIP", "IPAddress":
		return []string{"10.0.0.5", "192.168.1.10", "172.16.1.20", "8.8.8.8", "203.0.113.10"}
	case "ActionType":
		return []string{"ProcessCreated", "ConnectionSuccess", "FileCreated", "LogonFailed", "LogonSuccess"}
	case "Status", "ResultType":
		return []string{"Success", "Failure", "Failed", "0", "50074", "Denied"}
	case "FolderPath":
		return []string{`C:\Windows\System32`, `C:\Users\Public`, `C:\ProgramData`, `C:\Temp`}
	case "Message":
		return []string{"error", "critical", "failed password", "mfa required", "admin login"}
	default:
		return []string{"admin", "root", "SYSTEM", "svc_app", "web01", "test"}
	}
}

func randomValueForField(rng *rand.Rand, field string) string {
	values := valuesForField(field)
	return values[rng.Intn(len(values))]
}

func uniqueValues(rng *rand.Rand, pool []string, count int) []string {
	if count > len(pool) {
		count = len(pool)
	}
	perm := rng.Perm(len(pool))
	values := make([]string, 0, count)
	for i := 0; i < count; i++ {
		values = append(values, pool[perm[i]])
	}
	sort.Strings(values)
	return values
}

func oneOf[T any](rng *rand.Rand, values ...T) T {
	return values[rng.Intn(len(values))]
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func seedForCase(seed, id int64) int64 {
	x := uint64(seed) + 0x9e3779b97f4a7c15 + uint64(id)*0xbf58476d1ce4e5b9
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return int64(x ^ (x >> 31))
}

func newCorpusWriter(path string) (*corpusWriter, error) {
	if path == "" {
		return &corpusWriter{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if _, err := file.WriteString("[\n"); err != nil {
		file.Close()
		return nil, err
	}
	return &corpusWriter{file: file, enc: json.NewEncoder(file), first: true}, nil
}

func (w *corpusWriter) write(entry queryEntry) error {
	if w.file == nil {
		return nil
	}
	if !w.first {
		if _, err := w.file.WriteString(",\n"); err != nil {
			return err
		}
	}
	w.first = false
	return w.enc.Encode(entry)
}

func (w *corpusWriter) close() error {
	if w.file == nil {
		return nil
	}
	if _, err := w.file.WriteString("]\n"); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}

func newFailureWriter(path string) (*failureWriter, error) {
	if path == "" {
		return &failureWriter{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &failureWriter{file: file, enc: json.NewEncoder(file)}, nil
}

func (w *failureWriter) write(result runResult) error {
	if w.file == nil {
		return nil
	}
	return w.enc.Encode(result)
}

func (w *failureWriter) close() error {
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

func countFailureStage(s *summary, stage string) {
	switch stage {
	case "kql_parse":
		s.KQLParseErrors++
	case "expected_mismatch":
		s.ExpectedMismatch++
	case "sigma_parse":
		s.SigmaParseErrors++
	case "sigma_mismatch":
		s.SigmaMismatch++
	case "back_kql_parse":
		s.BackKQLParseError++
	case "back_kql_mismatch":
		s.BackKQLMismatch++
	}
}

func printProgress(s summary, total int64, start time.Time) {
	elapsed := time.Since(start)
	rate := float64(s.Total) / elapsed.Seconds()
	remaining := ""
	if rate > 0 && total > s.Total {
		remaining = fmt.Sprintf(", eta=%s", time.Duration(float64(total-s.Total)/rate)*time.Second)
	}
	duplicates := ""
	if s.DuplicateQueries > 0 {
		duplicates = fmt.Sprintf(" duplicates=%d", s.DuplicateQueries)
	}
	fmt.Fprintf(os.Stderr, "processed=%d/%d passed=%d failed=%d%s rate=%.0f/s%s\n", s.Total, total, s.Passed, s.Failed, duplicates, rate, remaining)
}

func printSummary(s summary, corpusPath, failPath string, failureRecords int) {
	fmt.Printf("generated=%d passed=%d failed=%d duplicates=%d duration=%s\n", s.Total, s.Passed, s.Failed, s.DuplicateQueries, s.Duration)
	fmt.Printf("failures: kql_parse=%d expected_mismatch=%d sigma_parse=%d sigma_mismatch=%d back_kql_parse=%d back_kql_mismatch=%d\n",
		s.KQLParseErrors, s.ExpectedMismatch, s.SigmaParseErrors, s.SigmaMismatch, s.BackKQLParseError, s.BackKQLMismatch)
	if corpusPath != "" {
		fmt.Printf("verified corpus: %s\n", corpusPath)
	}
	if failPath != "" {
		fmt.Printf("failure records: %s (%d written)\n", failPath, failureRecords)
	}
}

func mapCapacityHint(target int64) int {
	if target <= 0 {
		return 0
	}
	if int64(int(target)) != target {
		return 0
	}
	return int(target)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
