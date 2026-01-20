# KQL Parser

[![Go Reference](https://pkg.go.dev/badge/github.com/craftedsignal/kql-parser.svg)](https://pkg.go.dev/github.com/craftedsignal/kql-parser)
[![Go Report Card](https://goreportcard.com/badge/github.com/craftedsignal/kql-parser)](https://goreportcard.com/report/github.com/craftedsignal/kql-parser)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A production-ready Go parser for Kusto Query Language (KQL), built with ANTLR4. This parser extracts conditions, fields, and table references from KQL queries used in Microsoft Sentinel, Azure Data Explorer, and other Microsoft security products.

## Features

- **Full KQL Grammar Support**: Parses complex KQL queries including joins, unions, let statements, and nested expressions
- **Condition Extraction**: Extracts filter conditions with field names, operators, and values
- **Field Discovery**: Identifies all fields referenced in queries
- **Table References**: Extracts source tables and their relationships
- **Error Recovery**: Graceful handling of malformed queries with detailed error reporting
- **High Performance**: Optimized for processing large volumes of queries

## Installation

```bash
go get github.com/craftedsignal/kql-parser
```

## Usage

### Basic Condition Extraction

```go
package main

import (
    "fmt"
    kql "github.com/craftedsignal/kql-parser"
)

func main() {
    query := `
        SecurityEvent
        | where EventID == 4624
        | where AccountType == "User"
        | where TimeGenerated > ago(1h)
    `

    result := kql.ExtractConditions(query)

    fmt.Printf("Found %d conditions:\n", len(result.Conditions))
    for _, cond := range result.Conditions {
        fmt.Printf("  Field: %s, Operator: %s, Value: %s\n",
            cond.Field, cond.Operator, cond.Value)
    }

    if len(result.Errors) > 0 {
        fmt.Printf("Warnings: %v\n", result.Errors)
    }
}
```

### Output

```
Found 3 conditions:
  Field: EventID, Operator: ==, Value: 4624
  Field: AccountType, Operator: ==, Value: User
  Field: TimeGenerated, Operator: >, Value: ago(1h)
```

### Advanced Usage

```go
// Extract with full context
result := kql.ExtractConditions(query)

// Access extracted data
for _, cond := range result.Conditions {
    fmt.Printf("Condition: %s %s %s (negated: %v)\n",
        cond.Field, cond.Operator, cond.Value, cond.Negated)
}

// Get referenced tables
for _, table := range result.Tables {
    fmt.Printf("Table: %s\n", table)
}

// Get all fields
for _, field := range result.Fields {
    fmt.Printf("Field: %s\n", field)
}
```

## Supported KQL Features

| Feature | Status |
|---------|--------|
| where clauses | Supported |
| project / extend | Supported |
| summarize | Supported |
| join / union | Supported |
| let statements | Supported |
| Scalar functions | Supported |
| Aggregation functions | Supported |
| Time expressions | Supported |
| Regular expressions | Supported |
| Dynamic arrays/objects | Supported |
| Comments | Supported |

## API Reference

### Types

```go
// ExtractionResult contains all extracted information from a KQL query
type ExtractionResult struct {
    Conditions []Condition  // Extracted filter conditions
    Tables     []string     // Referenced table names
    Fields     []string     // All field references
    Errors     []string     // Non-fatal parsing warnings
}

// Condition represents a single filter condition
type Condition struct {
    Field      string   // Field name being filtered
    Operator   string   // Comparison operator (==, !=, >, <, contains, etc.)
    Value      string   // Filter value
    Values     []string // Multiple values for 'in' operator
    Negated    bool     // Whether condition is negated (not, !)
    Function   string   // Function wrapping the condition (tolower, toupper, etc.)
}
```

### Functions

```go
// ExtractConditions parses a KQL query and extracts all conditions
func ExtractConditions(query string) *ExtractionResult
```

## Performance

Benchmarks on the curated test suite (90 queries covering real-world patterns and edge cases):

| Metric | Value |
|--------|-------|
| Parse Success Rate | 100% |
| Condition Extraction | 97% |
| Avg Parse Time | <1ms |
| Queries/Second | >1,000 |

*Note: The 3% without extracted conditions are queries that legitimately have no filter conditions (metadata queries, aggregation-only queries, or schema exploration commands).*

## Grammar

This parser uses ANTLR4 with a comprehensive KQL grammar. The grammar files are included:

- `KQLLexer.g4` - Lexer rules
- `KQLParser.g4` - Parser rules

To regenerate the parser after grammar changes:

```bash
make generate
```

## Contributing

Contributions are welcome! Please ensure:

1. All tests pass: `make test`
2. Code is formatted: `make fmt`
3. Linter passes: `make lint`

## License

MIT License - see [LICENSE](LICENSE) for details.

## Related Projects

- [spl-parser](https://github.com/craftedsignal/spl-parser) - Splunk SPL parser
- [CraftedSignal](https://craftedsignal.com) - Detection engineering platform
