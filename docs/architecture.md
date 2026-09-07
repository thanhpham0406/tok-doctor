# Architecture

TokDoctor is a local-first token profiler and context optimizer for AI coding agents.

Its architecture is designed around one core requirement:

> Platform-specific data collection must be isolated from platform-independent analysis.

TokDoctor should remain lightweight, fast, easy to extend, and suitable for open-source contributions.

## Goals

The architecture should support:

- local-first analysis
- single-binary distribution
- low memory overhead
- streaming large session files
- multiple AI coding-agent sources
- deterministic token and context diagnostics
- terminal, JSON, and optional Web UI output
- future community-built adapters
- future historical comparison and optimization workflows

The architecture should avoid:

- framework-heavy layering
- Java-style service/repository abstractions
- cloud dependencies for local analysis
- platform-specific logic leaking into core analysis
- duplicated analysis logic across CLI and Web UI
- unnecessary infrastructure before product needs justify it

## High-Level Architecture

```text
External Agent Data
        |
        v
+---------------------+
|    Source Adapter   |
+----------+----------+
           |
           v
+---------------------+
|   Canonical Model   |
+----------+----------+
           |
           v
+---------------------+
|      Analyzer       |
+----------+----------+
           |
           v
+---------------------+
|     Rule Engine     |
+----------+----------+
           |
           v
+---------------------+
|   Analysis Result   |
+----------+----------+
           |
     +-----+-----+
     |     |     |
     v     v     v
 Terminal JSON  Web UI
```

Initial sources:

```text
Codex
9Router
Antigravity
Kiro
Cursor
Claude
```

Additional sources should be added based on real community demand.

## Dependency Direction

The dependency direction must remain simple:

```text
Source Adapter
      |
      v
Canonical Model
      ^
      |
Analyzer
      |
      v
Rule Engine
      |
      v
Analysis Result
      |
      v
Presentation
```

The important rule is not the exact package diagram. The important rule is that platform-specific details stay at the edge.

The following dependencies are not allowed:

```text
model   -> codex
model   -> cursor
rule    -> codex parser
rule    -> web UI
analyze -> terminal renderer
web UI  -> raw source files
```

Core analysis should operate on normalized data only.

## Main Components

### 1. Source Adapters

Source adapters are the platform-specific boundary.

Examples:

```text
CodexAdapter
9RouterAdapter
AntigravityAdapter
KiroAdapter
CursorAdapter
ClaudeAdapter
```

An adapter may:

- detect whether a source exists
- discover available sessions or usage records
- open and read source data
- parse platform-specific records
- normalize source data into the canonical model
- preserve authoritative provider-reported usage

An adapter must not:

- calculate health scores
- execute generic diagnostic rules
- generate generic optimization recommendations
- render terminal output
- contain Web UI logic

Conceptual source contract:

```go
type Source interface {
	Name() string
	Detect(context.Context) bool
	Sessions(context.Context) ([]SessionRef, error)
	Read(context.Context, SessionRef) (model.Session, error)
}
```

The exact interface may evolve with implementation needs. Do not add methods speculatively.

### 2. Canonical Model

The canonical model is the platform-independent representation of AI-agent activity.

It is the most important architectural boundary in TokDoctor.

Core concepts may include:

```text
Session
Turn
Event
ContextItem
ToolCall
ToolOutput
Usage
Finding
Evidence
```

Example:

```go
type Session struct {
	ID        string
	Agent     Agent
	Model     string
	StartedAt time.Time
	EndedAt   *time.Time

	Turns []Turn
	Usage Usage
}
```

Example usage model:

```go
type Usage struct {
	Input     int64
	Cached    int64
	Output    int64
	Reasoning int64
	Total     int64
}
```

The canonical model must not contain source-specific concepts such as:

```text
Codex rollout JSONL record types
Cursor internal database tables
Claude-specific session paths
9Router HTTP endpoint shapes
Kiro-specific storage formats
```

Those details belong in adapters.

### 3. Analyzer

The analyzer orchestrates platform-independent analysis.

Responsibilities:

- receive normalized session data
- derive analysis metrics
- prepare rule input
- execute registered rules
- aggregate findings
- produce an `AnalysisResult`

The analyzer should not:

- read Codex files directly
- query 9Router directly
- parse source-specific payloads
- render UI
- mutate original session data

Conceptually:

```text
model.Session
     |
     v
Analyzer
     |
     +--> derived metrics
     +--> rule execution
     +--> finding aggregation
     |
     v
AnalysisResult
```

### 4. Rule Engine

Rules detect token and context problems.

Examples:

```text
MCP001   unused-mcp
MCP002   oversized-mcp-schema

TOOL001  oversized-tool-output
TOOL002  repeated-tool-call
TOOL003  repeated-file-read

CTX001   runaway-context-growth

INST001  oversized-instructions
```

Rules should be deterministic by default.

A rule should:

- detect one clear problem
- return evidence
- assign severity
- assign confidence
- measure or estimate token impact when possible
- provide an actionable recommendation

Conceptual rule contract:

```go
type Rule interface {
	ID() string
	Analyze(context.Context, *analyze.Context) []model.Finding
}
```

Rules should not mutate session data.

Rules must not depend on raw platform formats.

### 5. Analysis Result

`AnalysisResult` is the shared output of the core analysis pipeline.

It should contain enough structured data for all presentation layers.

Conceptually:

```text
AnalysisResult
├── source
├── session metadata
├── token usage
├── derived metrics
├── findings
├── health score
└── recoverable token estimates
```

Terminal, JSON, and Web UI output should all render this same result.

This prevents business logic from being duplicated in presentation code.

### 6. Presentation

TokDoctor has multiple presentation modes.

```text
Terminal
JSON
Local Web UI
```

Presentation layers may:

- format findings
- sort or group results for display
- render tables
- render charts
- serialize results

Presentation layers must not:

- calculate waste
- parse session files
- execute source-specific logic
- redefine rule behavior

## CLI and Web UI

TokDoctor is CLI-first.

Expected usage:

```bash
tok doctor
tok doctor --ui
tok ui
tok sessions
tok inspect <session>
```

All core features must remain usable without the Web UI.

### Terminal

The terminal renderer is the default user experience.

Example:

```text
Token Health                           64/100

Total input                           148,291
Potentially recoverable          ~32k-41k

HIGH  TOOL001
Oversized tool output

git diff produced 128 KB.
Estimated impact: ~12k-16k tokens.
```

### JSON

JSON output exists for:

- automation
- CI
- integrations
- future editor plugins
- external tooling

Example:

```bash
tok doctor --format json
```

### Web UI

The Web UI is an optional local presentation layer.

Architecture:

```text
Preact
   |
   v
HTTP API
   |
   v
Go application/core
```

The Web UI must consume the same `AnalysisResult` used by terminal and JSON output.

The local server must bind to:

```text
127.0.0.1
```

by default.

The UI may be embedded into the Go binary using:

```go
//go:embed
```

This preserves single-binary distribution.

## Package Structure

Initial package organization:

```text
tokdoctor/
|
├── cmd/
│   └── tok/
│       └── main.go
|
├── internal/
│   ├── model/
│   ├── source/
│   │   ├── codex/
│   │   ├── router9/
│   │   ├── antigravity/
│   │   ├── kiro/
│   │   ├── cursor/
│   │   └── claude/
│   │
│   ├── analyze/
│   ├── rule/
│   │   ├── mcp/
│   │   ├── tool/
│   │   ├── context/
│   │   └── instruction/
│   │
│   ├── token/
│   ├── report/
│   │   ├── terminal/
│   │   └── json/
│   │
│   ├── webui/
│   └── config/
|
├── ui/
├── fixtures/
└── docs/
```

This structure may evolve as real use cases appear.

Do not create packages purely to match this diagram.

Packages should exist because they own meaningful behavior.

## Go Design Principles

TokDoctor follows idiomatic Go rather than framework-driven Clean Architecture.

### Standard Library First

Prefer:

```text
encoding/json
bufio
net/http
log/slog
context
errors
embed
io
os
path/filepath
```

before introducing external dependencies.

### Small Interfaces

Interfaces should represent real variation or boundaries.

Do not create interfaces for every concrete type.

Prefer interfaces near their consumers.

### Explicit Dependency Injection

Use constructors and explicit wiring.

Example:

```go
source := codex.New(...)
analyzer := analyze.New(...)
app := New(source, analyzer)
```

Avoid dependency injection frameworks.

### Concrete Types by Default

Use concrete types until an abstraction provides real value.

Avoid:

```text
Service
ServiceImpl
Repository
RepositoryImpl
Manager
BaseAnalyzer
AbstractRule
```

unless the design genuinely requires them.

### Clear Package Ownership

Avoid generic packages such as:

```text
utils
common
helpers
base
misc
impl
```

Prefer:

```text
token.Estimate
codex.Parse
analyze.Analyze
terminal.Render
```

### Comments

Prefer self-explanatory code.

Comments should explain:

- non-obvious intent
- important constraints
- compatibility behavior
- protocol quirks
- deliberate workarounds

Comments should not narrate obvious code.

## Token Accuracy Model

TokDoctor must distinguish authoritative measurements from local estimates.

Suggested confidence levels:

```text
measured
high
medium
low
```

Definitions:

### measured

Directly reported by a provider or authoritative source.

Example:

```text
input_tokens = 82,410
```

from a provider usage record.

### high

Derived deterministically from reliable local data.

Example:

```text
tool output size
known event relationship
repeated call count
```

### medium

A useful inference with meaningful uncertainty.

### low

A heuristic or semantic estimate.

Provider-reported usage always takes precedence over local token estimation.

TokDoctor must never present estimated attribution as exact provider usage.

## Data Flow Example

A Codex analysis may look like:

```text
~/.codex/sessions/.../rollout.jsonl
                |
                v
          Codex Source
                |
        parse + normalize
                |
                v
          model.Session
                |
                v
            Analyzer
                |
     +----------+----------+
     |          |          |
     v          v          v
 TOOL001     MCP001      CTX001
     |          |          |
     +----------+----------+
                |
                v
        AnalysisResult
                |
        +-------+-------+
        |       |       |
        v       v       v
      CLI      JSON     UI
```

A 9Router analysis follows the same core path:

```text
9Router data
     |
     v
9Router Source
     |
     v
model.Session
     |
     v
same Analyzer
     |
     v
same Rules
```

The difference belongs only at the source boundary.

## Correlating Multiple Sources

Some environments may provide complementary data.

Example:

```text
Codex local session
        |
        +---- agent execution events
        |
        v
     Correlation
        ^
        |
9Router usage records
        |
        +---- downstream provider usage
```

This may allow TokDoctor to connect:

```text
large tool output
        |
        v
subsequent request token growth
```

When cross-source correlation is added, it should produce stronger evidence without coupling generic rules to one platform combination.

Correlation is not required for the initial MVP.

## Streaming and Large Files

AI-agent session files may become very large.

Source adapters should stream JSONL and similar formats whenever practical.

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
```

Avoid reading an entire session into memory by default.

Performance optimizations should be driven by profiling and measurements.

## Storage

### MVP

No database is required for basic analysis.

Flow:

```text
source
  |
  v
stream
  |
  v
analyze
  |
  v
report
```

### Future

SQLite may be introduced when features require persistent history, such as:

```text
tok history
tok compare
regression detection
before/after verification
trend analysis
```

Storage should remain local by default.

The analysis engine must not depend directly on SQLite.

## External Adapter Protocol

Built-in adapters may be implemented in Go.

Future community adapters should not require contributors to use Go.

A future external adapter protocol may use NDJSON over stdin/stdout:

```text
tokdoctor-adapter-example
          |
          v
Normalized NDJSON events
          |
          v
TokDoctor
```

This allows adapters to be implemented in:

```text
Go
Rust
Python
TypeScript
other languages
```

External adapters are a future extension point and should not complicate the initial MVP.

## Security Boundaries

TokDoctor processes potentially sensitive local data.

Architecture must preserve these guarantees by default:

```text
no account
no cloud dependency
no prompt upload
no source-code upload
no session upload
no telemetry without consent
read-only source analysis
loopback-only Web UI
```

Security-sensitive features should be reviewed against `SECURITY.md`.

## Testing Strategy

Architecture boundaries should be protected by tests.

### Source Adapter Tests

Use sanitized fixtures:

```text
fixture
  |
  v
parser
  |
  v
normalizer
  |
  v
canonical model
```

### Rule Tests

Each rule should include:

```text
positive case
negative case
edge cases
```

### Integration Tests

Important paths should test:

```text
fixture
  |
  v
source
  |
  v
normalize
  |
  v
analyze
  |
  v
finding
```

### Presentation Tests

Stable terminal output may use golden tests.

Avoid excessive mocking.

## Architectural Decision Rules

When making a design decision, prefer the option that:

1. keeps platform details isolated
2. reduces dependencies
3. keeps local execution simple
4. remains understandable to contributors
5. preserves single-binary distribution
6. supports streaming
7. avoids speculative abstraction
8. produces testable behavior

If two designs solve the same problem, prefer the simpler one.

## MVP Architecture Scope

The first vertical slice should remain intentionally small:

```text
Codex session discovery
        |
        v
stream latest JSONL session
        |
        v
extract authoritative usage
        |
        v
normalize relevant events
        |
        v
detect largest tool outputs
        |
        v
run first deterministic rule
        |
        v
render terminal report
```

Do not build all adapters, persistent storage, auto-fix, cloud features, or historical comparison before the core analysis proves useful.

## Architecture Invariants

The following rules should remain true as TokDoctor grows:

1. Core analysis is platform-independent.
2. Source adapters own platform-specific parsing.
3. Presentation does not contain analysis logic.
4. Rules consume normalized data.
5. Provider measurements and estimates remain distinguishable.
6. Local-first is the default.
7. Source data is read-only during analysis.
8. Interfaces stay small and purposeful.
9. Dependencies remain minimal.
10. Simple code is preferred over architectural ceremony.
