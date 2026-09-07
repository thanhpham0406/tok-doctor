# Adapter Specification

TokDoctor adapters convert platform-specific AI coding-agent data into TokDoctor's canonical model.

Adapters are the boundary between external source formats and platform-independent analysis.

Their job is simple:

> Read source-specific data safely, preserve authoritative usage, and normalize it into canonical TokDoctor data.

Adapters must not contain generic analysis or optimization logic.

## Scope

The initial built-in adapters are:

- Codex
- Claude Code
- 9Router

Additional adapters should be added based on real community demand.

This document defines the expected behavior for both built-in adapters and future external adapters.

## Adapter Responsibilities

An adapter may:

- detect whether its source is available
- discover sessions or usage records
- read source-specific files, databases, APIs, or local services
- parse platform-specific formats
- normalize records into TokDoctor's canonical model
- preserve authoritative provider-reported usage
- expose source capabilities and limitations
- tolerate unknown or newly introduced event types when safe

An adapter must not:

- calculate token health scores
- implement generic waste-detection rules
- generate generic optimization recommendations
- render terminal output
- render Web UI output
- mutate original source data
- silently modify source configuration
- upload source data to remote services

## Adapter Boundary

The expected flow is:

```text
Platform-specific data
        |
        v
Source Adapter
        |
        v
Canonical TokDoctor Model
        |
        v
Analyzer
        |
        v
Rules
```

The analyzer and rule engine must never depend on platform-specific storage formats.

Examples of platform-specific details that must remain inside adapters:

```text
Codex rollout JSONL event names
Claude session file paths
9Router API response shapes
```

## Conceptual Interface

The exact Go interface may evolve as implementation needs become clear.

A minimal source contract may look like:

```go
type Source interface {
	Name() string
	Detect(context.Context) bool
	Sessions(context.Context) ([]SessionRef, error)
	Read(context.Context, SessionRef) (model.Session, error)
}
```

Do not add methods speculatively.

Add interface surface only when a real adapter use case requires it.

## Source Detection

`Detect` determines whether the adapter appears usable in the current environment.

Detection should be:

- fast
- read-only
- side-effect free
- safe when the source is not installed
- safe when the source format has changed

Examples:

```text
Codex:
check expected session directory

9Router:
check local configuration or expected local service

Claude:
check known local session storage
```

Detection should not:

- start external services
- modify source configuration
- authenticate remotely
- block for long periods
- fail the entire TokDoctor process when the source is simply unavailable

A missing source is normally not an error.

## Session Discovery

Adapters should expose the smallest useful unit of analysis.

For most coding agents, this is expected to be a session.

A session reference may contain:

```go
type SessionRef struct {
	ID        string
	StartedAt time.Time
	Path      string
}
```

This is illustrative, not a required final shape.

A session reference should contain only enough information to identify and read the session.

Avoid putting full parsed session data inside session discovery results.

### Ordering

When practical, adapters should return sessions in a deterministic order.

For example:

```text
newest first
```

This makes commands such as:

```bash
tok doctor
```

predictable when analyzing the latest session.

## Reading Source Data

Adapters must treat source data as read-only.

Do not:

- rewrite logs
- migrate source databases
- delete old sessions
- update agent configuration
- alter provider usage history

Use safe read-only access patterns whenever possible.

If a source requires opening a local database, prefer read-only access if the database engine supports it.

## Streaming

AI-agent logs may become very large.

Adapters should stream JSONL and similar append-only formats whenever practical.

Preferred pattern:

```text
file
 |
 v
buffered reader
 |
 v
record parser
 |
 v
normalizer
 |
 v
canonical events/session
```

Avoid:

```go
data, _ := os.ReadFile(path)
```

for potentially large session files.

Do not load an entire file into memory unless:

- the source format requires it
- the file size is known to be safe
- the implementation has a clear reason

Performance changes should be driven by measurement.

## Parsing

Parsers should be resilient to source evolution.

When safe, unknown record types should be ignored rather than causing the entire session to fail.

Prefer:

```text
known record -> parse
unknown harmless record -> skip
malformed required record -> contextual error
```

Do not silently ignore malformed records when doing so would corrupt usage or analysis.

Errors should include useful source context.

Prefer:

```text
parse codex session abc123 at line 184: invalid token usage
```

Avoid:

```text
invalid data
```

## Normalization

Normalization converts source-specific records into canonical TokDoctor concepts.

Typical mappings may include:

```text
source session          -> model.Session
source turn             -> model.Turn
prompt/message          -> model.ContextItem
tool call               -> model.ToolCall
tool result             -> model.ToolOutput
provider usage          -> model.Usage
compaction              -> canonical context event
MCP definition          -> canonical tool/context item
```

Normalization must preserve useful evidence without leaking platform-specific implementation details into the core model.

## Canonical Model Rules

Adapters should normalize toward shared concepts such as:

```text
Session
Turn
ContextItem
ToolCall
ToolOutput
Usage
Evidence
```

Do not add a source-specific field to the canonical model simply because one adapter exposes it.

Before extending the canonical model, ask:

1. Is this concept meaningful across multiple platforms?
2. Will generic analysis use it?
3. Can it be represented as metadata instead?
4. Does adding it create platform coupling?

If the answer is source-specific, keep it inside the adapter.

## Usage and Token Data

Provider-reported usage is authoritative when available.

Example:

```text
input_tokens
cached_input_tokens
output_tokens
reasoning_tokens
total_tokens
```

Adapters should preserve these values without re-estimating them.

If an adapter derives token usage locally, it must mark the result as estimated.

TokDoctor distinguishes:

```text
measured
high
medium
low
```

### measured

Directly reported by an authoritative source or provider.

### high

Derived deterministically from reliable local data.

### medium

Inferred with meaningful uncertainty.

### low

Heuristic or semantic estimate.

Never convert estimated values into `measured`.

## Raw Content

Agent data may contain sensitive material.

Adapters should avoid retaining raw content when metadata is sufficient.

Prefer:

```text
tool output size
event type
timestamp
tool name
token usage
content hash
```

over preserving full raw tool output when generic analysis does not require it.

If raw content is required for a rule or future semantic analysis, the data model and privacy implications should be considered explicitly.

Do not log raw prompts, credentials, source code, or tool output unnecessarily.

## Source Capabilities

Different platforms expose different levels of detail.

Adapters should not fabricate missing data to make all sources look identical.

For example:

```text
Codex may expose detailed local execution events.
Claude Code may expose local session usage.
9Router may expose downstream provider usage.
```

If data is unavailable:

```text
leave it absent
```

rather than inventing values.

The analyzer should handle partial canonical data safely.

## Capability Metadata

As adapters become more varied, TokDoctor may expose adapter capabilities.

Conceptually:

```go
type Capabilities struct {
	TokenUsage     bool
	ToolCalls      bool
	ToolOutput     bool
	MCPDefinitions bool
	Reasoning      bool
	ContextEvents  bool
}
```

Do not introduce this abstraction until the implementation actually needs capability-aware behavior.

## Multi-Source Correlation

Some environments may expose complementary sources.

Example:

```text
Codex
  |
  +--> agent execution events
  |
  v
Correlation
  ^
  |
  +--> actual downstream usage
9Router
```

A future correlation layer may connect:

```text
large tool output
        |
        v
subsequent provider input growth
```

This can strengthen evidence and confidence.

Cross-source correlation must not be implemented as platform-specific special cases inside generic rules.

Correlation is not required for the initial MVP.

## Error Handling

Adapters should distinguish between:

```text
source unavailable
session unavailable
unsupported format
malformed data
permission error
temporary local service failure
```

Where useful, expose sentinel or typed errors that callers can inspect with:

```go
errors.Is
errors.As
```

Wrap errors with context:

```go
return fmt.Errorf("read claude session %s: %w", ref.ID, err)
```

Do not expose secrets through error messages.

## Security

Adapters operate at a sensitive boundary.

They may access:

- local session files
- local databases
- local HTTP services
- source code references
- prompts
- tool output
- usage records

Adapters must:

- read only the data required
- remain local by default
- respect operating-system permissions
- avoid mutating source data
- avoid exposing secrets in logs or errors
- avoid silently enabling network access
- avoid executing commands derived from untrusted source content

See `SECURITY.md` for project-wide security requirements.

## Built-In Adapter Layout

A built-in adapter should keep parsing and normalization close to the source.

Example:

```text
internal/source/codex/
├── source.go
├── parser.go
├── normalize.go
├── types.go
└── source_test.go
```

Do not create files only to match this example.

Use the smallest structure that keeps responsibilities clear.

### Suggested Responsibilities

`source.go`

```text
detection
session discovery
source orchestration
```

`parser.go`

```text
source-format parsing
```

`normalize.go`

```text
source records -> canonical model
```

`types.go`

```text
source-private record types
```

Source-private types should remain inside the adapter package when possible.

## Testing Requirements

Every adapter should include fixture-based tests.

Minimum expected coverage:

```text
source detection
basic session discovery
valid session parsing
normalization
unknown event handling
malformed required data
relevant edge cases
```

### Fixtures

Fixtures must be:

- synthetic, or
- carefully sanitized

Never commit:

- real credentials
- API keys
- access tokens
- proprietary code
- private prompts
- private repository URLs
- sensitive local paths
- internal hostnames
- unsanitized real session logs

Use the smallest fixture that reproduces the behavior.

Example:

```text
fixtures/
└── codex/
    ├── basic-session.jsonl
    ├── unknown-event.jsonl
    ├── malformed-usage.jsonl
    └── oversized-tool-output.jsonl
```

## Fixture Test Example

Conceptually:

```go
func TestReadSession(t *testing.T) {
	src := newTestSource("fixtures/codex/basic-session.jsonl")

	session, err := src.Read(context.Background(), SessionRef{
		ID: "basic",
	})
	if err != nil {
		t.Fatal(err)
	}

	if session.Agent != model.AgentCodex {
		t.Fatalf("unexpected agent: %s", session.Agent)
	}
}
```

Prefer behavior-focused assertions.

Avoid testing private implementation details unless needed.

## Adapter Conformance

A built-in adapter is considered usable when it can:

1. detect its source safely
2. discover at least one supported analysis unit
3. read source data without mutation
4. normalize core events
5. preserve authoritative usage
6. tolerate harmless unknown events
7. report malformed critical data clearly
8. pass fixture-based tests

## External Adapter Protocol

Future community adapters should not require contributors to write Go.

A future external adapter may communicate with TokDoctor using NDJSON over stdin/stdout.

Conceptual output:

```json
{"type":"session","id":"abc","agent":"example"}
{"type":"turn","id":"turn-1"}
{"type":"usage","input":48291,"output":3811,"confidence":"measured"}
{"type":"tool_call","tool":"shell"}
{"type":"tool_output","bytes":81922}
```

The protocol should remain:

- language-neutral
- streaming
- versioned
- easy to validate
- safe to parse
- backward-compatible when practical

External adapter support is a future extension point.

Do not complicate the initial built-in adapter implementation for a protocol that does not yet exist.

## Adapter Naming

Use clear source names.

Preferred package names:

```text
codex
router9
claude
```

Avoid names such as:

```text
codexadapter
codex_impl
claude_manager
```

The package already provides context.

## Comments

Adapter code may require comments for protocol quirks or undocumented source behavior.

Use comments when explaining:

- a source-format quirk
- cumulative versus per-turn usage
- compatibility handling
- why a field must be ignored
- why a fallback exists

Do not comment obvious parsing steps.

Prefer:

```go
// Codex reports cumulative session usage here.
// Keep the previous snapshot so normalization can derive turn deltas.
```

Avoid:

```go
// Parse the JSON.
```

## Dependencies

Adapters should prefer the Go standard library.

Before adding a source-specific dependency, consider:

- whether the source can be parsed with stdlib
- binary-size impact
- transitive dependencies
- CGO requirements
- cross-platform support
- maintenance quality
- security implications

Do not add a large SDK merely to read a small local format.

## Compatibility

Source formats may change without notice.

Adapters should isolate compatibility code locally.

If supporting multiple source versions:

```text
detect version
   |
   +--> parser A
   |
   +--> parser B
```

Do not spread source-version checks throughout the analyzer or rules.

When a source format change breaks parsing:

1. add a sanitized fixture for the new format
2. update the adapter
3. preserve old-format support when practical
4. add regression tests

## Adapter Pull Request Checklist

A new adapter pull request should answer:

- Which platform does this support?
- Which platform versions or formats were tested?
- Where does the source data come from?
- Is the integration read-only?
- Which canonical fields are available?
- Which fields are measured versus estimated?
- What source capabilities are unavailable?
- Are fixtures synthetic or sanitized?
- Are parser and normalization tests included?
- Does the adapter introduce new dependencies?
- Does it preserve local-first behavior?

## Definition of Done

An adapter is complete when:

- source detection is safe and read-only
- supported sessions or usage records can be discovered
- source data can be parsed reliably
- platform-specific records normalize into the canonical model
- authoritative usage is preserved
- estimates are clearly identified
- malformed input produces useful errors
- harmless unknown events are handled safely
- no original source data is modified
- no sensitive content is logged unnecessarily
- fixture-based tests cover expected behavior
- generic analyzer and rules require no source-specific changes
- relevant test, vet, and lint checks pass
