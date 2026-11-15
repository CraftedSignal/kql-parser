# Scalar filter predicates

`ConditionExpression` and `SemanticExpression` now have a `BooleanScalar`
(`"scalar"`) leaf with a `Scalar *ScalarPredicate` payload. A scalar predicate
preserves a complete, unevaluated filter, including constant-only filters and
comparisons whose operands cannot be represented faithfully as a `Condition`.

For example, `T | where a == 1 or false` has an OR expression containing a
condition leaf and a scalar leaf with `Text: "false"`. `T | where 5 > 10` has
zero `Conditions` and a non-nil scalar `Expression`. Numeric literals are no
longer emitted as field names; numeric column names must be quoted, e.g.
`['5']`. Standalone quoted field names retain their decoded identity without
sanitization collisions. Literal-left comparisons with a reference on the
right now emit the reference as `Field` and reverse relational operators.

Consumers must walk `Expression`, handle `BooleanScalar`, and preserve its
`Text`, `Negated`, `PipeStage`, and `Pipeline`. `Text` is an exact slice of the
normalized parser input for ordinary scalars. An inline tabular membership
instead retains the complete original authored atom, including its left
operand, operator spelling, subquery, comments and internal whitespace. The
parser does not evaluate or fold scalar predicates. Applying `Negated` means negating
the complete predicate. The semantic API clones scalar payloads.

A consumer that cannot represent or execute scalar leaves must explicitly
report them as unsupported. Never discard a scalar or unknown leaf, skip it
while converting an AND/OR node, or fall back to the flat `Conditions` list:
that can turn `a == 1 and false` into `a == 1` and change which rows match.
Zero conditions does not mean there is no filter. Repeated scalar occurrences
are preserved because their functions may be nondeterministic.

Known let references that have not been proven to bind a literal in the
applicable scope remain scalar predicates. `LetStatements` retains shadowed
bindings in source order. Constant substitution is limited to later top-level
statements with unambiguous bindings; function scopes and column assignments
that may shadow a binding disable it. Consumers need the retained bindings to
interpret unresolved scalar text.

Null/empty checks with constant or computed arguments, and list operators
with computed or reference operands, now remain complete scalar predicates.
Literal list elements remain condition values; quoted literals never become
references. Representable range bounds use `ValueReference` for columns and
`Value` for literals.

Inline tabular membership such as `Account in ((SigninLogs | project Account))`
is now a `BooleanScalar` leaf with that complete source as `Text`. It does not
emit a synthetic `Condition` or a literal `"SubqueryResult"` value. Different
subqueries, repeated occurrences, and an authored `"SubqueryResult"` literal
remain distinct in the public parse and semantic models. `!in`/`!in~` stay in
the authored text; `Negated` records only an enclosing Boolean NOT. Membership
inside a larger atomic scalar, such as `isnull(Account in (...))`, preserves the
whole atom. AND/OR grouping and other condition leaves remain structured.

The exported `SubqueryPlaceholder` constant and `NormalizeQueryForDebug`'s
legacy placeholder output are retained for compatibility with normalization
tools; neither represents an executable membership condition. A consumer must
handle or explicitly reject the opaque scalar. Malformed tabular RHS syntax or
lost source association yields a failure-only result without partial filters.

This parser change does not update or release the platform dependency. Before
adopting it, the platform adapter must handle scalar leaves or return an
unsupported-expression error. In particular, `kqlExpressionToDomain` must
reject unknown leaves instead of returning nil, `convertParserBooleanChildren`
must not skip nil AND children, and adapter fallback must not discard scalar
branches by rebuilding an expression from flat conditions.

## Opaque search predicates

Explicit `search` operators now produce a `BooleanSearch` (`"search"`) leaf in
both expression APIs, with `Search *SearchPredicate`. Its `Text` is the complete
original search predicate source, including Boolean syntax, literal spelling,
comments and internal whitespace. This is stronger than the normalized-source
contract of ordinary scalar leaves. `Kind` retains the authored search mode;
`Scope` is `input`, `database`, or `tables`. `TableSources` preserves ordered,
unresolved selector fragments. `PipeStage` and `Pipeline` retain ownership.
Semantic conversion deep-clones selector slices.

Search terms are no longer fabricated as `_keyword_ contains` Conditions.
Consumers must handle the search leaf or explicitly reject it; rebuilding from
flat Conditions or treating its Text as an ordinary where scalar loses search
scope, mode, matching rules and output-schema effects. Consecutive filter stages
are connected by AND, but pipeline/stage metadata does not authorize moving them
across transformations. Search predicates remain opaque; repeated occurrences
are preserved and no evaluator, wildcard compiler or schema lookup is provided.

Malformed search syntax, unsupported modes/selectors, and input combined with
an explicit table scope return failure-only results, consistently with Validate.
There is no first-term keyword or partial-where recovery for these searches.
Bare `not term` is retained as a local compatibility form; this does not assert
that Kusto services accept it. No consumer dependency is updated by this change.

## Condition postprocessing identity correction

OR alternatives now combine only literal predicates with the same exact column,
operator, source field, pipe stage, and pipeline. Case-distinct columns and
right-hand field references remain separate expression leaves. This corrects
previously lossy grouping such as `User == "a" or user == "b"` and
`x == y or x == z`.

`DeduplicateConditions` uses complete condition identity and keeps source order.
Its existing latest-stage projection now runs independently for each exact
field and pipeline. It preserves literal equality to `"*"`; that value is a
literal, not an unrestricted search. This helper projects flat conditions and
does not replace the expression tree or simplify its Boolean semantics.
