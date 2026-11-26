# KQL Parser

[![Go Reference](https://pkg.go.dev/badge/github.com/craftedsignal/kql-parser.svg)](https://pkg.go.dev/github.com/craftedsignal/kql-parser)
[![Go Report Card](https://goreportcard.com/badge/github.com/craftedsignal/kql-parser)](https://goreportcard.com/report/github.com/craftedsignal/kql-parser)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)

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

See [predicate migration](MIGRATION.md) before consuming expression trees
or upgrading an adapter that uses only flat conditions.

Explicit `search` filters are preserved as opaque `BooleanSearch` leaves with a
`SearchPredicate` payload. For `T | search "alpha" or not("beta")`, `Text` is
exactly `"alpha" or not("beta")`, `Scope` is `input`, and the leaf retains its
pipeline and stage. Search kind and ordered unresolved table selectors are also
retained. Consumers must handle or reject these leaves: search no longer emits
fabricated `_keyword_` Conditions. See the migration guide for failure semantics
and the distinction from normalized ordinary scalar predicates.

OR grouping and `DeduplicateConditions` now distinguish case-sensitive column
names, right-hand field references, stages, and pipelines. Literal equality to
`"*"` is retained. The dedup helper keeps only the latest stage for each exact
field and pipeline in source order; use `Expression` for Boolean semantics.

`ExtractConditions` and `AnalyzeQuery` expose an additive `Dataflow` graph.
Its ordered stages retain authored command/source text, immutable input and
output scopes, and bindings for supported assignments, projections, renames,
and summaries. A condition's `StageID`, `InputScopeID`, and `FieldBinding`
identify the field version it reads; typed field operands carry their own
`FieldBinding`. A `joins/N` input points to the corresponding child result's
output scope. Check `Dataflow.Complete` and `Warnings` before using the graph:
an opaque transform keeps its original stage and an unknown output scope.
The graph describes provenance; it does not execute KQL predicates.

An unaliased integer literal in `project` has a generated output name outside
this graph's finite naming contract, so the whole projection remains opaque.
Explicit aliases and quoted numeric column references remain distinct supported
paths. Legacy `ProjectedFields` stays available and does not prove a complete
output schema when the graph is incomplete.

An unaliased standalone integer literal in a `summarize` BY list also has
an unrepresented generated output name. The whole summarize stage is opaque
before publishing any metric or group bindings; authored source and legacy
aggregation/group summaries remain available. Explicit group aliases, quoted
numeric field names, and ordinary field groups keep their existing behavior.

Parser-produced `ComputedFields` and `ComputedExpressions` use exact-case keys.
For `extend Fresh=src`, look up `"Fresh"`, not `"fresh"`; a separate raw field
named `fresh` remains distinct. These legacy maps describe the latest value
for each exact name. Use `Dataflow.Bindings` to retain successive assignments.

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

Null and empty filter functions retain their KQL names in `Condition.Operator`:
`isnull`, `isnotnull`, `isempty`, and `isnotempty`. Empty checks include the
empty string and are distinct from null checks. This corrects earlier output
that mapped `isempty` to `isnull` and `isnotempty` to `isnotnull`; consumers
matching operator names should handle all four. Boolean comparisons and `not`
are reflected in `Condition.Negated`. Calls used to compute columns or aggregate
values do not create row filters.

```go
// ExtractConditions parses a KQL query and extracts all conditions
func ExtractConditions(query string) *ExtractionResult
```

## Query corpus coverage

Collected Azure Monitor, SlimKQL, and Intune queries, together with focused
regression tests, cover filters, scalar expressions, nested Boolean logic,
literals, functions, tabular operators, joins, projections, aggregations, and
normalization boundaries. The corpus test excludes template-generated rows and
distinguishes extracted expressions from valid no-predicate queries and
unsupported syntax.

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

AGPL-3.0 - see [LICENSE](LICENSE) for details.

## Related Projects

- [spl-parser](https://github.com/craftedsignal/spl-parser) - Splunk SPL parser
- [CraftedSignal](https://craftedsignal.com) - Detection engineering platform

### Summarize output boundaries

Represented summarize columns use case-sensitive unique output names, reserving
BY outputs before allocating metrics in authored order (for example, repeated
`sum(Bytes)` outputs `sum_Bytes` and `sum_Bytes1`). Duplicate declarations and
unaliased groups with unknown output names keep the whole stage opaque; legacy
summaries remain available without invented finite bindings.

Dataflow leaves a whole `summarize` stage opaque when documented row-returning
`arg_max`/`arg_min` (including `argmax`/`argmin`) or multi-expression/wildcard
`take_any`/`any` cannot be represented as one finite aggregate column. An
unaliased single-expression `take_any` is also opaque because its default output
name is not represented by the legacy summary. Explicitly aliased single-value
`take_any`, direct-column `any(Name)`, `take_anyif`, `count()`, and scalar aggregates
retain their existing graph behavior. Legacy aggregation summaries and authored
stage text remain available; opaque stages do not publish fabricated bindings.
This boundary does not validate backend function signatures or execution.

Plural `percentiles`/`percentilesw` with multiple percentile ranks also make the
whole summarize stage opaque. An explicitly aliased one-rank plural aggregation
keeps its existing single-column graph behavior. Unaliased `percentile`,
one-rank plural forms, and `percentiles_array` remain opaque because their
rank-dependent/default names are not represented by the legacy summary.
Explicitly aliased scalar and array-returning forms (including the documented
dynamic percentile-array argument) keep their existing graph behavior; multiple
arguments alone do not imply multiple outputs. This narrow boundary preserves
legacy names and does not validate signatures.

Unaliased three-argument `percentilew` and `percentilesw_array` with at least
three positional arguments also leave the whole stage opaque: their default
output names are not faithfully represented by the legacy summary. The weighted
array remains one output, including its dynamic percentile-list form; opacity
does not assert multiple outputs or choose a disputed default name. Explicit
aliases retain their existing graph behavior. This rule does not extend to named
arguments, other functions or unsupported arities.

For exact lowercase `make_bag` with one or two positional arguments and an
ordinary original unquoted column such as `p`, the unaliased output is `bag_p`.
The legacy aggregate and finite graph binding share that identity. Other
unaliased `make_bag` argument shapes (including paths, quoted names, computed
values and normalized origins) leave the whole summarize stage opaque before
any metric or group bindings; their authored source and legacy summaries remain.
Explicit aliases keep their existing output spelling and representation. This
narrow naming contract does not validate input types or backend signatures.

Documented scalar aggregate names use their language result-name policy:
`buildschema` uses `schema_`, conditional list/set/bag builders use `list_`,
`set_` or `bag_`, and `tdigest_merge`/`merge_tdigest` use `merge_tdigests_`.
`make_list_with_nulls` uses `list_`; `binary_all_and`/`or`/`xor` and `take_anyif`
use the first argument's result name. Quoted/parenthesized direct names and
provably unnamed prefix expressions are represented. Unaliased names requiring
schema-dependent path naming, uncatalogued nested function policies or generated
`ColumnN` allocation make the whole summarize dataflow opaque; explicit aliases
retain their authored output identity. Legacy summaries remain available.

### Ordered aggregate results

Summarize keeps one `Aggregation` per authored call. Parenthesized declarations
and `percentile`, `percentiles`, `percentilew`, and `percentilesw` additionally
carry optional `result` metadata: the zero-based `call_ordinal` among all calls
in that stage, exact authored RHS `expression`, `complete`, and ordered
`outputs` (`ordinal`, `column`, `binding_id`). Integer literal ranks have proven
default names; fully supplied aliases can also name decimal/exponent ranks.
Unproved defaults, unsupported calls, invalid declarations or an opaque sibling
keep the result incomplete with no output IDs.

A committed single output mirrors legacy `Column`/`BindingID`. Multiple outputs
leave both legacy fields empty and retain `StageID`; consumers must read the
result members. Complete means the containing stage committed every member,
including historical definitions later replaced by a simple declaration. Final
visible `FieldScope.BindingIDs` excludes replaced definitions; historical stage
IDs and immutable result ordinals remain. Semantic conversion deep-clones the
result and its outputs. Ordinary scalar calls retain a nil result. Adding the
optional struct field affects external unkeyed `Aggregation` literals; keyed
literals and field readers remain source-compatible. This metadata describes
provenance, not percentile execution or a consumer execution catalog.

Non-STAR multi-expression `take_any` also uses ordered result members, one per
original argument. Direct fields keep their names, inner declarations provide
names, and proven unnamed expressions use `any_arg<i>` with the original
zero-based argument index. Outer aliases override successive members; partial
aliases require proven remaining names. Unknown defaults, empty/malformed
labels, STAR and multi-output AS retain whole-stage opacity. Ordinary
single-expression calls keep `Result=nil` and existing scalar compatibility,
including explicitly named unknown expressions and AS suffix allocation.
Every member remains an aggregate from the same sampled-row call, with the
same exact whole-call expression; members are not independent sampling calls
or passthroughs. This adds provenance, not execution or deterministic sampling.

Modern lowercase non-STAR `arg_min` and `arg_max` expose one ordered Result
member per argument, including the selector at ordinal zero. Direct and inner
names are retained; unnamed selectors use `min_`/`max_`, and later unnamed
members use `min_<original-selector-name>_arg<i>` or its `max_` equivalent.
Outer declarations override successive names; unknown required defaults keep
the whole stage opaque. Selector-only calls also expose Result. Simple ASSIGN
uses tuple replacement, while AS keeps suffix allocation and remains opaque
for multiple outputs. STAR, deprecated spellings and unknown cardinalities
remain unsupported. Each member retains the exact whole-call source and common
input ownership; this adds provenance, not row-selection execution guarantees.
