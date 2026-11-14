# Parsing limits

`ExtractConditions` and `AnalyzeQuery` retain their signatures. Their context
variants, `ExtractConditionsContext(ctx, query)` and
`AnalyzeQueryContext(ctx, query)`, accept caller cancellation.
`ValidateContext(ctx, query)` also applies these limits.

Each call uses one private budget, shared with recursive subqueries and semantic
conversion. The default accepted-input limits are 128 KiB for both raw and
normalized query text, 16 nested syntax delimiters, and 4,096 tokens. Real lexer
tokens distinguish syntax delimiters from delimiters inside literals and
comments. The complete bounded token list is checked before parsing begins.
Cooperative work is limited to 1,000,000 checks, and guarded condition/semantic
output growth is limited to 4,096 charged items. Intermediate semantic clones
also consume that output budget. Unevaluated scalar leaves and their clones
consume it too; typed operand recognition, scalar expression scans, let shadowing
checks, and generated subquery-marker provenance use the same work budget.
Syntax diagnostics are capped at 32 per listener, with each message truncated to 1,024 bytes.

Parsing runs synchronously: a returned cancellation or limit error leaves no
parsing worker running in the background. Cancellation and limit failures return
an error without partial conditions or semantic results and do not enter the
normal syntax-recovery fallback.

`MaxParseTime` retains its existing default of five seconds and caps cooperative
checks alongside the caller's deadline. It is not a hard wall-clock, CPU, or heap
limit: ANTLR prediction and other work can continue between checks. Deployments
that require hard isolation still need an external process boundary and resource
limits. Unexpected programming panics retain the public failure-only contract: they
return an error (or nil for the legacy classifier), without partial parse results
or a portable fallback retry.
