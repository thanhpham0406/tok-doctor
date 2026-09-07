# Contributing to TokDoctor

Thanks for contributing to TokDoctor.

TokDoctor is a local-first token profiler and context optimizer for AI coding agents. Contributions should keep the project lightweight, fast, privacy-friendly, and easy to extend.

## Development Principles

Keep these principles in mind before making changes:

- Prefer simple, idiomatic Go.
- Keep dependencies minimal.
- Use the Go standard library when it is sufficient.
- Keep platform-specific logic inside source adapters.
- Keep analysis logic independent from Codex, Claude Code, 9Router, or any other source format.
- Prefer deterministic analysis over LLM-based analysis.
- Preserve local-first behavior and privacy guarantees.
- Avoid speculative abstractions.
- Keep pull requests focused.
- Prefer clear naming and structure over comments.
- Comment only when intent, constraints, workarounds, or non-obvious behavior cannot be expressed clearly in code.

See `AGENTS.md` for the full engineering rules.

## Development Setup

### Requirements

For core development:

- Go
- Git

For Web UI development:

- Node.js or Bun

Recommended development tools:

- `golangci-lint`
- `govulncheck`

### Clone the Repository

```bash
git clone https://github.com/<owner>/tokdoctor.git
cd tokdoctor
```

### Run Tests

```bash
go test ./...
```

### Run Static Checks

```bash
go vet ./...
golangci-lint run
```

For security-sensitive changes or dependency updates:

```bash
govulncheck ./...
```

## Project Structure

The repository is organized around clear responsibilities.

```text
cmd/
  tok/                 CLI entry point

internal/
  model/               Canonical platform-independent model
  source/              Source adapters and source discovery
  analyze/             Analysis orchestration
  rule/                Diagnostic rules
  token/               Token counting and estimation
  report/              Terminal and JSON output
  webui/               Local Web UI server
  config/              Configuration

ui/                    Optional embedded Web UI

fixtures/              Synthetic or sanitized test data

docs/
  architecture.md      Architecture boundaries
  adapter-spec.md      Source adapter contract
  rule-spec.md         Diagnostic rule contract
```

Core analysis must never depend on platform-specific storage formats.

The expected dependency flow is:

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

## Making Changes

Before coding:

1. Understand the existing package boundary.
2. Keep the change as small as possible.
3. Add or update tests with the behavior change.
4. Avoid unrelated refactoring.
5. Avoid adding dependencies unless they provide clear value.

Do not create generic packages such as:

```text
utils
common
helpers
base
misc
manager
impl
```

Place code close to the responsibility it belongs to.

## Adding a Source Adapter

TokDoctor initially targets:

- Codex
- Claude Code
- 9Router

Additional platforms should be added based on real community demand.

A source adapter is responsible for:

- detecting whether the source is available
- discovering sessions or usage records
- reading source-specific data
- parsing the source format
- normalizing data into the canonical model

A source adapter must not:

- calculate health scores
- contain generic optimization logic
- generate generic recommendations
- render terminal or Web UI output

A new adapter should include:

1. source detection
2. session or usage discovery
3. safe data reading
4. normalization
5. synthetic or sanitized fixtures
6. parser tests
7. normalization tests

See `docs/adapter-spec.md` for the adapter contract.

## Adding a Diagnostic Rule

Rules should detect one clear problem and return actionable findings.

Example rule families:

```text
MCP001
MCP002

TOOL001
TOOL002

CTX001

INST001
```

A rule should provide:

- stable rule ID
- severity
- confidence
- evidence
- measured or estimated token impact when possible
- actionable recommendation

Rules should be deterministic whenever possible.

Do not make a rule depend directly on raw Codex JSONL, Claude session files, 9Router APIs, or other source-specific formats.

See `docs/rule-spec.md` for the rule contract.

## Token Accuracy

TokDoctor must distinguish measured data from estimates.

Use the following confidence model:

- `measured` — reported directly by the provider or authoritative source
- `high` — derived deterministically from reliable local data
- `medium` — inferred with meaningful uncertainty
- `low` — heuristic or semantic estimate

Provider-reported usage takes precedence over local token estimates.

Never present an estimated attribution as exact provider usage.

## Testing

Behavior changes require tests.

Prefer:

- table-driven unit tests
- fixture-based adapter tests
- parser tests
- normalization tests
- rule positive and negative cases
- relevant edge cases
- integration tests across source -> normalize -> analyze -> findings
- golden tests for stable CLI output when useful

Avoid excessive mocking.

### Fixtures

Fixtures must be:

- synthetic, or
- carefully sanitized

Never commit:

- real user prompts
- proprietary source code
- API keys
- access tokens
- credentials
- private session logs
- sensitive tool output

When reporting a parser bug, create the smallest sanitized fixture that reproduces it.

## Code Style

Follow idiomatic Go.

Prefer:

- small functions
- explicit dependencies
- concrete types
- small interfaces
- interfaces defined near consumers
- descriptive package names
- contextual error messages
- streaming parsers for large logs

Avoid:

- Java-style `Service` / `ServiceImpl` pairs
- unnecessary repositories
- dependency injection frameworks
- global mutable state
- generic base types
- unnecessary factories
- premature optimization
- excessive comments

### Comments

Prefer self-explanatory code.

Do not write comments that simply restate the code.

Avoid:

```go
// Initialize the analyzer.
analyzer := analyze.New()
```

Use comments for non-obvious intent or constraints:

```go
// Codex reports cumulative usage at the session level.
// Subtract the previous snapshot to derive per-turn usage.
delta := current.Total - previous.Total
```

## Error Handling

Errors should provide enough context to diagnose failures.

Prefer:

```go
return fmt.Errorf("read codex session %s: %w", ref.ID, err)
```

Avoid vague errors such as:

```text
something went wrong
```

Use `errors.Is` and `errors.As` when callers need to inspect error types.

## Performance

Agent session files may be large.

Prefer streaming parsers for JSONL and similar formats.

Do not load entire session files into memory unless required by the format and known to be safe.

Optimize after measurement, not speculation.

## Local-First and Privacy

TokDoctor is local-first.

By default, changes must preserve these guarantees:

- no account required
- no cloud dependency
- no prompt or source-code upload
- no session upload
- no telemetry unless explicitly enabled
- no mutation of original agent session data
- Web UI binds to loopback only

The local Web UI should bind to:

```text
127.0.0.1
```

Do not change the default to `0.0.0.0`.

## Web UI Contributions

The Web UI is optional.

Everything important must remain usable without it.

The Web UI must consume the same analysis result used by terminal and JSON output.

Do not duplicate token-analysis logic in:

- frontend code
- HTTP handlers
- UI state management

When changing the Web UI, run the frontend build and tests used by the repository.

## Dependencies

Before adding a dependency, ask:

1. Can the Go standard library solve this clearly?
2. Is the dependency actively maintained?
3. Is the dependency small enough for the problem?
4. Does it affect single-binary distribution?
5. Does it introduce CGO or platform-specific build problems?
6. Is the maintenance cost justified?

Avoid large frameworks for small problems.

## Commit and Pull Request Scope

Keep commits and pull requests focused.

Do not include unrelated:

- formatting changes
- package renames
- refactoring
- dependency upgrades
- generated files
- feature additions

A good pull request should be reviewable independently.

## Pull Request Description

Include:

1. What problem does this solve?
2. Why is this change needed?
3. What behavior changed?
4. How was it tested?
5. Are there compatibility or privacy implications?

For adapter changes, mention which platform/version or data format was tested.

For rule changes, include example evidence and expected findings.

## Before Submitting

Run:

```bash
go test ./...
go vet ./...
golangci-lint run
```

When appropriate:

```bash
govulncheck ./...
```

If Web UI code changed, also run the frontend build and tests used by the repository.

## Definition of Done

A contribution is ready when:

- the requested behavior works
- architecture boundaries remain intact
- platform-specific logic is isolated
- tests cover the behavior
- errors contain useful context
- no unnecessary dependency was added
- comments are limited to genuinely non-obvious behavior
- local-first and privacy guarantees remain intact
- relevant tests, vet, lint, and build checks pass

## Security Issues

Do not open a public issue containing credentials, private prompts, proprietary source code, or sensitive session data.

Follow the process described in `SECURITY.md` for security-related reports.

## License

By contributing to TokDoctor, you agree that your contributions will be licensed under the repository's Apache License 2.0.
