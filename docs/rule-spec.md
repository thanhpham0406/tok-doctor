# Rule Specification

TokDoctor rules detect token and context inefficiencies from normalized AI-agent data.

Rules are the diagnostic core of TokDoctor.

A good rule should answer:

> What is inefficient, what evidence supports that conclusion, how confident are we, what is the likely token impact, and what should the user change?

Rules should be deterministic by default and must operate only on TokDoctor's canonical model or derived analysis context.

## Scope

Rules may detect problems related to:

- tool usage
- tool output
- MCP definitions
- context growth
- repeated work
- instructions
- token usage
- caching
- compaction
- model or reasoning configuration
- other platform-independent efficiency issues

Rules must not:

- parse raw Codex, Claude, or 9Router formats
- read platform-specific source files directly
- mutate original session data
- silently modify agent configuration
- render terminal or Web UI output
- present estimates as exact provider measurements

## Rule Flow

```text
Canonical Session
       |
       v
Derived Analysis Context
       |
       v
      Rule
       |
       v
   Finding(s)
       |
       v
Analysis Result
```

Rules should remain independent from presentation and source adapters.

## Rule Contract

The exact Go interface may evolve as implementation needs become clear.

A conceptual rule contract is:

```go
type Rule interface {
	ID() string
	Analyze(context.Context, *analyze.Context) []model.Finding
}
```

Do not add interface methods speculatively.

A rule should return zero findings when the problem is not present.

## Rule IDs

Rule IDs must be stable.

Use a short category prefix and numeric identifier.

Recommended families:

```text
MCP001
MCP002

TOOL001
TOOL002
TOOL003

CTX001
CTX002

INST001
INST002

CACHE001

MODEL001
```

Example meanings:

```text
MCP001   unused-mcp
MCP002   oversized-mcp-schema

TOOL001  oversized-tool-output
TOOL002  repeated-tool-call
TOOL003  repeated-file-read

CTX001   runaway-context-growth
CTX002   stale-context

INST001  oversized-instructions
INST002  duplicated-instructions
```

Do not reuse a rule ID for a different diagnosis after release.

If rule behavior changes significantly, preserve backward compatibility where practical or introduce a new rule ID.

## Rule Naming

Use concise diagnostic names.

Prefer:

```text
oversized-tool-output
unused-mcp
runaway-context-growth
```

Avoid:

```text
check-tool-output-size-rule
token-optimization-analyzer
context-manager-rule
```

Rule names should describe the problem, not the implementation.

## Finding Model

A rule produces one or more findings.

A finding should contain enough information for a user to understand and act on the diagnosis.

Conceptually:

```go
type Finding struct {
	RuleID         string
	Severity       Severity
	Confidence     Confidence
	Title          string
	Description    string
	Impact         TokenRange
	Evidence       []Evidence
	Recommendation string
}
```

The final model may include additional fields when real requirements justify them.

## Severity

Severity describes how important the issue is to address.

Recommended levels:

```text
low
medium
high
critical
```

Severity should reflect practical token, cost, latency, or quality impact.

### low

Minor inefficiency with limited expected impact.

### medium

Noticeable inefficiency worth addressing.

### high

Significant recurring or per-session waste with clear optimization value.

### critical

Reserved for extreme cases such as runaway context, pathological loops, or behavior that can cause very large token consumption.

Do not mark findings as `high` or `critical` merely to attract attention.

Severity should be explainable from evidence.

## Confidence

Confidence describes how certain TokDoctor is that the diagnosis is correct.

Recommended levels:

```text
measured
high
medium
low
```

### measured

The conclusion is supported directly by authoritative provider or source measurements.

Example:

```text
provider input increased by 18,421 tokens
after a known context event
```

### high

The diagnosis is derived deterministically from reliable local evidence.

Example:

```text
an MCP server exposed tool definitions but was never called
```

### medium

The diagnosis is useful but involves meaningful inference.

Example:

```text
older context appears unrelated to later turns
```

### low

The finding relies on a heuristic or semantic estimate.

Example:

```text
instructions appear semantically redundant
```

Do not use `measured` when only local tokenization or heuristic estimation is available.

## Evidence

Every finding should include concrete evidence.

Evidence may include:

- event IDs
- turn IDs
- tool names
- output byte size
- call counts
- token usage
- context growth
- timestamps
- source references
- estimated token counts
- repeated content hashes
- before/after measurements

Prefer structured evidence over prose-only explanations.

Conceptually:

```go
type Evidence struct {
	Kind  string
	Label string
	Value any
}
```

Do not expose sensitive raw content when metadata is sufficient.

Prefer:

```text
tool: git diff
output_bytes: 128412
estimated_tokens: 18200
```

over printing the complete tool output.

## Token Impact

A finding should measure or estimate token impact when practical.

TokDoctor should distinguish:

```text
measured impact
estimated impact
potentially recoverable impact
```

### Measured Impact

Use when authoritative source data directly supports the value.

Example:

```text
observed downstream input increase: 18,421 tokens
```

### Estimated Impact

Use when TokDoctor derives the value from local content or heuristics.

Example:

```text
estimated output contribution: ~8,100-9,300 tokens
```

### Recoverable Tokens

Recoverable tokens are not automatically equal to the total size of a context item.

For example:

```text
MCP schema: 8,000 estimated tokens
```

does not necessarily mean:

```text
8,000 tokens are waste
```

A rule must avoid overstating recoverable savings.

Use ranges when uncertainty exists.

Example:

```text
potentially recoverable: ~4.5k-7.2k tokens
```

## Token Range

A token impact range may be represented conceptually as:

```go
type TokenRange struct {
	Min        int64
	Max        int64
	Confidence Confidence
}
```

Exact shape may evolve.

When `Min == Max`, the value may represent a measured or deterministic amount.

Do not create false precision.

## Recommendation

Every actionable finding should include a practical recommendation.

Recommendations should be:

- specific
- concise
- feasible
- proportional to the evidence
- safe by default

Prefer:

```text
Scope `git diff` to the files relevant to the current task.
```

over:

```text
Optimize tool usage.
```

Prefer:

```text
Disable the Figma MCP server when design tools are not needed.
```

over:

```text
Reduce MCP overhead.
```

Recommendations should not require users to understand TokDoctor internals.

## Recommendation Safety

A recommendation must not assume that fewer tokens always means better behavior.

Avoid recommendations that may reduce quality without acknowledging the tradeoff.

Example:

Bad:

```text
Remove all previous conversation history.
```

Better:

```text
Consider compacting older turns that are no longer relevant to the current task.
```

Rules should optimize efficiency, not token count in isolation.

## Deterministic-First Policy

Rules should be deterministic whenever possible.

Good deterministic inputs include:

- tool call count
- repeated command signature
- repeated file path
- output byte size
- provider token usage
- MCP usage count
- context growth rate
- compaction count
- content hashes

Do not use an LLM when a reliable deterministic rule can answer the question.

Reasons:

- lower runtime cost
- predictable behavior
- easier testing
- better privacy
- easier explanation
- no circular token waste

## Semantic Rules

Some future diagnoses may require semantic analysis.

Examples:

```text
stale-context
duplicated-instructions
semantic redundancy
quality-preserving prompt compression
```

Semantic rules should be treated differently from deterministic rules.

They should:

- be optional when practical
- clearly report lower confidence when appropriate
- avoid remote data transmission by default
- disclose when an LLM or semantic model was used
- never silently upgrade inferred data to measured data

Semantic analysis is not required for the initial MVP.

## Rule Categories

### MCP Rules

Examples:

```text
MCP001 unused-mcp
MCP002 oversized-mcp-schema
MCP003 excessive-tool-count
```

Possible evidence:

```text
server name
tool definition count
estimated schema tokens
call count
session count
```

### Tool Rules

Examples:

```text
TOOL001 oversized-tool-output
TOOL002 repeated-tool-call
TOOL003 repeated-file-read
```

Possible evidence:

```text
tool name
command signature
output bytes
estimated tokens
repeat count
turn IDs
```

### Context Rules

Examples:

```text
CTX001 runaway-context-growth
CTX002 stale-context
CTX003 excessive-compaction
```

Possible evidence:

```text
input tokens by turn
growth percentage
compaction events
context window usage
```

### Instruction Rules

Examples:

```text
INST001 oversized-instructions
INST002 duplicated-instructions
```

Possible evidence:

```text
instruction source
estimated token size
duplicate groups
semantic overlap
```

Instruction rules may require semantic analysis for stronger conclusions.

### Cache Rules

Examples:

```text
CACHE001 low-cache-reuse
CACHE002 repeated-uncached-context
```

Use only when the source exposes enough authoritative cache information.

### Model Rules

Examples:

```text
MODEL001 excessive-reasoning-budget
MODEL002 expensive-model-for-trivial-task
```

These rules require careful quality considerations and should not be part of the first deterministic MVP unless evidence is strong.

## Example: Oversized Tool Output

Rule:

```text
TOOL001
oversized-tool-output
```

Possible logic:

```text
if output_bytes > configured threshold:
    emit finding
```

Possible finding:

```text
Rule: TOOL001
Severity: HIGH
Confidence: HIGH

Tool:
git diff

Output:
128 KB

Estimated context contribution:
~18k-21k tokens

Recommendation:
Scope the diff to relevant files or paths.
```

If downstream provider usage confirms the increase:

```text
Confidence: MEASURED
Observed input increase: 18,421 tokens
```

## Example: Repeated File Read

Rule:

```text
TOOL003
repeated-file-read
```

Potential evidence:

```text
file: src/auth/service.go
read count: 6
turns: 3, 5, 7, 8, 10, 12
```

Recommendation:

```text
Reuse existing context when the file has not changed, or read only the required section.
```

The rule should account for legitimate rereads after file modifications when that information is available.

## Example: Unused MCP

Rule:

```text
MCP001
unused-mcp
```

Potential evidence:

```text
server: figma
tool definitions: 23
calls: 0
estimated schema contribution: ~8.4k tokens
```

Do not automatically claim the full schema contribution is recoverable.

Possible impact:

```text
potentially recoverable: ~6.5k-8.4k tokens
confidence: high
```

Recommendation:

```text
Disable or lazy-load the Figma MCP server when design tools are not needed.
```

## Example: Runaway Context Growth

Rule:

```text
CTX001
runaway-context-growth
```

Possible evidence:

```text
turn 12: 32k input
turn 13: 39k input
turn 14: 67k input
growth: +72%
```

The rule should identify abnormal growth relative to surrounding turns or configured thresholds.

Recommendation may suggest inspecting:

- large tool output
- repeated context
- compaction behavior
- oversized injected instructions

The context-growth rule should not duplicate lower-level findings if a more specific rule already explains the cause.

## Rule Coordination

Rules may identify related symptoms.

Example:

```text
TOOL001 oversized-tool-output
        |
        v
CTX001 runaway-context-growth
```

TokDoctor should avoid overwhelming users with redundant findings.

Possible future strategies:

- finding grouping
- root-cause linking
- suppression
- parent/child relationships

Do not build a complex correlation engine before real findings demonstrate the need.

## Thresholds

Thresholds should have sensible defaults.

Examples:

```text
tool output byte threshold
context growth percentage
repeat count
MCP schema size
```

Thresholds should be:

- documented
- testable
- configurable when real users need tuning

Do not expose configuration for every internal constant prematurely.

Prefer stable defaults first.

## Configuration

A rule may eventually support configuration.

Conceptually:

```toml
[rules.TOOL001]
enabled = true
max_output_bytes = 65536
```

Configuration should not be introduced until users have a real need to tune the rule.

Rules should have useful zero-config defaults.

## Rule Registry

Rules may be registered explicitly.

Example:

```go
rules := []Rule{
	tool.NewOversizedOutput(...),
	tool.NewRepeatedCall(...),
	context.NewRunawayGrowth(...),
}
```

Avoid reflection-based automatic registration.

Explicit registration keeps behavior predictable and easy to review.

## Rule Packages

Suggested package organization:

```text
internal/rule/
├── rule.go
├── registry.go
├── tool/
├── mcp/
├── context/
└── instruction/
```

Do not create packages solely to match this layout.

Keep related behavior together.

## State

Rules should be stateless when practical.

If a rule requires derived session state, compute that state in the analysis context or construct the rule with explicit dependencies.

Avoid global mutable state.

A rule invocation should produce the same result for the same input and configuration whenever possible.

## Error Handling

A rule should not normally fail the entire analysis because one optional signal is unavailable.

Prefer:

```text
required evidence missing -> no finding
```

over:

```text
entire doctor command fails
```

If a rule encounters corrupted required analysis data, return or surface the error through the analyzer using the project's normal error handling.

Do not silently produce misleading findings from incomplete evidence.

## Privacy

Rules operate on potentially sensitive data.

Prefer metadata-based analysis when possible.

Examples:

```text
hash repeated content instead of storing full content
compare byte sizes
compare event IDs
compare file paths only when needed
```

Do not include full prompt or source-code content in a finding unless explicitly required for the user-facing diagnosis.

Terminal, JSON, and Web UI output may be copied or shared. Findings should minimize accidental data leakage.

## Performance

Rules may run across large sessions.

Prefer:

- linear scans
- precomputed indexes
- hashes for repeat detection
- bounded memory
- shared derived metrics

Avoid repeatedly scanning the entire session independently for every rule when a shared analysis metric can be computed once.

Do not prematurely create complex optimization infrastructure.

Profile before optimizing.

## Testing Requirements

Every rule must include tests.

Minimum coverage:

```text
positive case
negative case
relevant edge cases
```

Where applicable, also test:

```text
threshold boundary
missing optional data
partial usage data
multiple findings
duplicate suppression
confidence level
token impact range
```

Prefer table-driven tests.

## Positive Case

A positive test proves the rule emits the expected finding.

Example:

```go
func TestOversizedToolOutput(t *testing.T) {
	// build normalized analysis input

	findings := rule.Analyze(context.Background(), input)

	// assert TOOL001
}
```

Focus assertions on behavior:

```text
rule ID
severity
confidence
evidence
impact
recommendation
```

Avoid coupling tests to private implementation details.

## Negative Case

Every rule must prove that normal behavior does not produce a false positive.

Example:

```text
tool output below threshold
-> no TOOL001 finding
```

Negative cases are especially important because excessive false positives reduce trust in TokDoctor.

## Edge Cases

Examples:

```text
empty session
missing token usage
zero-byte output
very large output
unknown tool
multiple identical calls
file modified between reads
source exposes no MCP data
```

Rules should degrade safely when evidence is incomplete.

## Fixture-Based Rule Tests

When useful, test rules through real normalized fixture flows:

```text
fixture
  |
  v
adapter
  |
  v
canonical session
  |
  v
analyzer
  |
  v
rule
  |
  v
finding
```

This is particularly useful for regression tests involving platform-format changes.

## Golden Tests

Golden tests may be used for stable serialized findings or CLI output.

Do not use golden files when direct structured assertions are clearer.

## False Positives

Reducing false positives is more important than maximizing finding count.

A rule that emits frequently but cannot justify its diagnosis is harmful.

Prefer:

```text
no finding
```

over:

```text
low-quality speculation
```

when evidence is insufficient.

Confidence should communicate uncertainty, but confidence labels are not a substitute for sound rule logic.

## False Precision

Avoid exact-looking numbers that are actually estimates.

Bad:

```text
You wasted 8,413 tokens.
```

when TokDoctor only estimated local content size.

Better:

```text
Estimated impact: ~7.8k-8.9k tokens.
```

Use exact values only when authoritative measurements support them.

## Rule Documentation

Every released rule should have a short description documenting:

- what it detects
- why it matters
- evidence used
- confidence behavior
- recommendation
- known limitations

This documentation may later power:

```bash
tok explain TOOL001
```

or Web UI help.

Do not duplicate large amounts of implementation detail in documentation.

## Rule Pull Request Checklist

A new rule pull request should answer:

- What problem does the rule detect?
- Is the diagnosis platform-independent?
- What canonical data does it require?
- Is the rule deterministic?
- What evidence supports the finding?
- How is severity determined?
- How is confidence determined?
- Is token impact measured or estimated?
- Can the recommendation reduce quality?
- What are known false-positive cases?
- Are positive and negative tests included?
- Are edge cases covered?
- Does the rule expose sensitive data?
- Is a new dependency required?

## MVP Rule Scope

The first version should focus on high-confidence deterministic rules.

Recommended initial set:

```text
MCP001   unused-mcp
MCP002   oversized-mcp-schema

TOOL001  oversized-tool-output
TOOL002  repeated-tool-call
TOOL003  repeated-file-read

CTX001   runaway-context-growth
```

Do not begin with semantic rules such as:

```text
stale-context
duplicated-instructions
prompt-quality optimization
```

until the deterministic core proves useful.

## Future Optimization and Verification

A finding is not the same as a verified optimization.

Future TokDoctor workflows may support:

```text
finding
  |
  v
proposed fix
  |
  v
before/after replay
  |
  v
token comparison
  |
  v
quality verification
```

Rules should preserve enough structured evidence to support future verification.

When TokDoctor eventually claims:

```text
safe to apply
```

that conclusion should come from verification, not merely from the diagnostic rule.

## Definition of Done

A rule is complete when:

- it has a stable rule ID
- it detects one clear platform-independent problem
- it operates on normalized data
- it provides concrete evidence
- severity is justified
- confidence is justified
- measured and estimated values are clearly distinguished
- token impact does not overstate recoverable waste
- recommendation is specific and actionable
- normal behavior does not produce obvious false positives
- positive tests pass
- negative tests pass
- relevant edge cases are covered
- no source-specific parsing leaks into the rule
- no sensitive content is exposed unnecessarily
- relevant test, vet, and lint checks pass
