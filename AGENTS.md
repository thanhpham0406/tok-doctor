# TokDoctor Agent Guide

## Project Goal

TokDoctor is a local-first token profiler and context optimizer for AI coding agents.

The project analyzes local agent data, attributes token usage, detects waste, and provides actionable recommendations to reduce token consumption and cost without sacrificing quality.

## Core Principles

- Local-first by default.
- Single-binary distribution.
- Standard library first.
- Keep dependencies minimal.
- Prefer simple, explicit code over clever abstractions.
- Follow idiomatic Go.
- Do not introduce abstractions before they are needed.
- Keep platform-specific logic inside source adapters.
- Core analysis must not depend on Codex, 9Router, Antigravity, Kiro, Cursor, Claude, or any other platform-specific format.
- Prefer deterministic analysis over LLM-based analysis.
- Provider-reported token usage is authoritative when available.
- Estimated values must be clearly marked as estimates.
- Never treat estimated attribution as exact provider-reported usage.
- Privacy and local execution are default product constraints, not optional features.

## Initial Supported Sources

The initial supported platforms are:

- Codex
- 9Router
- Antigravity
- Kiro
- Cursor
- Claude

Additional sources should be added based on real community demand.

## Architecture

Dependency flow:

```text
External Agent Data
        |
        v
Source Adapter
        |
        v
Canonical Model
        |
        v
Analyzer
        |
        v
Rule Engine
        |
        v
Analysis Result
        |
        +--> Terminal
        +--> JSON
        +--> Local Web UI
```

### Source Adapters

Adapters are responsible for platform-specific data access and normalization.

Adapters may:

- detect whether a source is available
- discover sessions
- read raw session or usage data
- parse platform-specific formats
- normalize data into the canonical model

Adapters must not:

- calculate health scores
- contain optimization rules
- generate recommendations
- render terminal or Web UI output

### Canonical Model

The canonical model must remain platform-independent.

Core concepts include:

- Session
- Turn
- ContextItem
- ToolCall
- ToolOutput
- Usage
- Finding
- Evidence

The canonical model must never depend on raw Codex JSONL, Cursor storage, Claude session formats, 9Router APIs, or any other platform-specific representation.

### Analyzer

The analyzer operates only on the canonical model.

Its responsibilities include:

- deriving analysis metrics
- preparing rule input
- executing rules
- aggregating findings
- producing an analysis result

### Rule Engine

Rules should be deterministic by default.

A rule consumes normalized analysis data and returns findings.

Rules must not:

- read platform-specific source data directly
- mutate source data
- render output
- silently convert estimates into measured values

### Presentation

Presentation layers include:

- terminal output
- JSON output
- local Web UI

Presentation code must not contain business logic or token-analysis logic.

## Go Engineering Guidelines

### Prefer Simple Go

Do not copy Java-style Clean Architecture patterns into Go.

Avoid unnecessary layers such as:

- Service / ServiceImpl pairs
- Repository abstractions without multiple real implementations
- DI containers
- factories for trivial construction
- generic base types
- global interface packages

Use constructors and explicit dependency injection.

Prefer small packages with clear responsibilities.

### Interfaces

- Keep interfaces small.
- Define interfaces close to the consumer when practical.
- Accept interfaces and return concrete types where appropriate.
- Do not create an interface only to make a concrete type look abstract.
- Introduce interfaces when there is a real boundary or multiple implementations.

### Package Naming

Prefer short, specific package names such as:

- `model`
- `source`
- `analyze`
- `rule`
- `token`
- `report`
- `config`

Avoid generic packages such as:

- `utils`
- `common`
- `helpers`
- `base`
- `misc`
- `manager`
- `impl`

Keep code close to the domain it belongs to.

### Errors

Wrap errors with useful context.

Prefer:

```go
return fmt.Errorf("read codex session %s: %w", ref.ID, err)
```

Avoid vague errors such as:

```text
something went wrong
```

Use `errors.Is` and `errors.As` when callers need to inspect error types.

### Context

Use `context.Context` only for cancellation, deadlines, and request lifecycle.

Do not:

- store `context.Context` inside long-lived structs
- use context as a generic data bag

### Comments

Prefer self-explanatory code over comments.

Comment only when intent, constraints, workarounds, protocol details, or non-obvious behavior cannot be expressed clearly through naming and code structure.

Do not add comments that merely repeat what the code already says.

Avoid AI-style explanatory comments such as:

```go
// Initialize the analyzer.
// Loop through the findings.
// Print the result.
```

A useful comment explains why something exists, not what an obvious statement does.

Example:

```go
// Codex reports cumulative usage at the session level.
// Subtract the previous snapshot to derive per-turn usage.
delta := current.Total - previous.Total
```

## Data and Token Accuracy

TokDoctor must clearly distinguish between measured and estimated data.

Suggested confidence levels:

- `measured` — directly reported by the provider or authoritative source
- `high` — derived deterministically from reliable local data
- `medium` — inferred with meaningful uncertainty
- `low` — heuristic or semantic estimate

Provider-reported token usage takes precedence over locally estimated token counts.

Never present estimated token attribution as exact provider usage.

## Performance

Agent session files can be large.

Prefer streaming parsers for JSONL and similar log formats.

Do not load an entire session file into memory unless the format or use case requires it and the size is known to be safe.

Optimize only after measurement. Do not introduce low-level complexity without evidence that it is needed.

## Local-First and Security

By default TokDoctor must:

- operate locally
- avoid uploading prompts, source code, tool output, or session history
- require no account
- require no cloud service
- avoid telemetry unless explicitly enabled
- keep the Web UI bound to loopback only
- never modify original agent session data during analysis

The local Web UI should bind to `127.0.0.1`, not `0.0.0.0`, unless the user explicitly requests otherwise.

Never log secrets, credentials, API keys, tokens, or sensitive prompt contents unnecessarily.

## Testing

Every parser change should include fixture-based tests.

Every rule should include:

- a positive case
- a negative case
- relevant edge cases

Prefer:

- table-driven tests
- fixture tests for source adapters
- golden tests for stable terminal output when useful
- integration tests across source -> normalize -> analyze -> findings

Avoid excessive mocking.

Do not add real user prompts, proprietary source code, credentials, API keys, or private session data to fixtures.

Fixtures must be synthetic or carefully sanitized.

## Adding a New Source

A new platform integration should:

1. detect the source
2. discover sessions or usage records
3. read data safely
4. normalize into the canonical model
5. include sanitized fixtures
6. include parser and normalization tests

Platform-specific logic must remain inside its source adapter.

Do not add special cases for a platform inside the analyzer or generic rules unless the behavior is genuinely universal.

## Adding a New Rule

A rule should:

- have a stable rule ID
- detect one clear problem
- provide evidence
- report severity
- report confidence
- estimate or measure token impact where possible
- provide an actionable recommendation

Example rule families:

```text
MCP001
MCP002

TOOL001
TOOL002

CTX001

INST001
```

Prefer narrow rules over large rules that diagnose many unrelated problems.

## Web UI

The Web UI is an optional presentation layer.

All core functionality must remain usable without the UI.

Expected commands may include:

```bash
tok doctor
tok doctor --ui
tok ui
tok sessions
tok inspect <session>
```

The Web UI must consume the same analysis result used by terminal and JSON output.

Do not duplicate analysis logic in HTTP handlers or frontend code.

## Dependencies

Before adding a dependency, check whether the Go standard library is sufficient.

A new dependency should provide clear value that would be costly or error-prone to implement internally.

Avoid large frameworks for small problems.

Do not add:

- web frameworks when `net/http` is sufficient
- ORM layers for simple local storage
- dependency injection frameworks
- logging frameworks when `log/slog` is sufficient

## Scope Discipline

Keep changes focused on the requested task.

Do not:

- perform unrelated refactors
- rename unrelated packages
- change public behavior without need
- add speculative abstractions for future features
- add support for new platforms unless part of the task
- introduce cloud infrastructure into local-first features

## Before Completing Changes

Run the relevant checks:

```bash
go test ./...
go vet ./...
golangci-lint run
```

When security-sensitive dependencies or code paths change, also run:

```bash
govulncheck ./...
```

If Web UI code changes, also run the frontend build and tests used by the repository.

## Definition of Done

A change is complete when:

- the requested behavior works
- architecture boundaries remain intact
- platform-specific logic stays isolated
- tests cover the behavior
- errors contain useful context
- no unnecessary dependency was introduced
- comments are limited to genuinely non-obvious intent or constraints
- local-first and privacy guarantees remain intact
- formatter, tests, vet, and lint checks pass
