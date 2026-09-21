# TokDoctor

Local-first token profiler and context optimizer for AI coding agents.

TokDoctor helps you understand where your AI coding tokens go, detect unnecessary context usage, and get actionable recommendations to reduce token consumption and cost without sacrificing quality.

> TokDoctor is currently in early development.

## Features

Supported sources:

* Codex
* Claude Code

Supported today:

* **Source discovery** — `tok sources` reports each agent's status, and `tok source test <name>` verifies that one is usable
* **Session usage** — `tok usage` and `tok sessions` report provider-authoritative token usage
* **Cost estimation** — `tok cost` and `tok pricing` estimate API-equivalent cost from a local catalog, with no network access
* **Coverage** — `tok coverage tool-output` reports how much tool output a source actually let TokDoctor observe
* **Inspection and reconciliation** — `tok inspect` shows a session's per-turn usage, context attribution, and how gateway-captured requests compare against the session transcript
* **Gateway capture** — `tok gateway` proxies local API traffic, records request metadata, and reports per-profile usage
* **Diagnostic rules** — two rules are implemented and enabled: `TOOL001 oversized-tool-output`, which reports complete tool outputs above 64 KiB, and `TOOL002 repeated-tool-call`, which reports conservatively matched repeated tool calls
* **Terminal and JSON output** — every report command supports both, via `--format terminal|json`
* **Local Web UI** — `tok doctor --ui` or `tok ui` serves a UI on `127.0.0.1`

9Router is detected and probed, but it does not yet produce sessions or usage.

## Requirements

Core development:

* Go
* Make
* Git

Web UI development:

* Node.js and npm

Check your environment:

```bash
go version
make --version
node --version
npm --version
```

## Getting Started

### 1. Clone the repository

```bash
git clone https://github.com/thanhpham0406/tok-doctor.git
cd tok-doctor
```

### 2. Install Go dependencies

```bash
go mod tidy
```

### 3. Install Web UI dependencies

The Web UI is optional, but install its dependencies if you want to use `--ui` or work on the frontend.

```bash
npm --prefix ui install
```

### 4. Build TokDoctor

```bash
make build
```

The binary will be created at:

```text
bin/tok
```

### 5. Run TokDoctor

Check the version:

```bash
./bin/tok version
```

List the sessions TokDoctor can read, then analyze one with the deterministic
`TOOL001` oversized tool output and `TOOL002` repeated tool call rules:

```bash
./bin/tok sessions
./bin/tok doctor <session-id>
```

`TOOL001` reports complete tool outputs above 64 KiB. Byte size is observed
from the local transcript; per-output token contribution remains an estimate.

`TOOL002` reports a tool that appears to have been called more than once with
the same normalized arguments when every complete result had the same content,
and only when the source declared a distinct tool call ID for each call. It
cannot see whether a repeat was legitimate, so stateful operations and polling
may be reported; the token impact it shows is an estimate of repeated tool
output, not provider-measured waste.

`tok doctor` without a session ID does not discover a session yet. It analyzes
a placeholder and reports no findings, so pass a session ID for a real result.

Get JSON output:

```bash
./bin/tok doctor <session-id> --format json
```

Inspect pricing catalog state and API-equivalent cost estimates:

```bash
./bin/tok pricing status
./bin/tok pricing update
./bin/tok pricing list
./bin/tok pricing show gpt-5.5
./bin/tok cost <session-id>
./bin/tok cost --all
```

`tok cost` never fetches pricing data from the network. Catalog precedence is local override, downloaded last-known-good catalog, embedded fallback, then unavailable. To use custom pricing, set `override_path` under `[pricing]` in the TokDoctor config to a local catalog JSON file.

Inspect a session turn by turn, including how gateway-captured requests compare
against the session transcript:

```bash
./bin/tok inspect <session-id>
./bin/tok inspect <session-id> --all-turns --context
```

Report how much tool output a source let TokDoctor observe:

```bash
./bin/tok coverage tool-output --all
```

Open the local Web UI:

```bash
./bin/tok doctor --ui
```

Or start the UI directly:

```bash
./bin/tok ui
```

The Web UI binds to:

```text
127.0.0.1
```

by default and is not exposed to your local network.

## Run Without Building

During development, you can run TokDoctor directly with Go:

```bash
go run ./cmd/tok version
```

```bash
go run ./cmd/tok sessions
```

This is useful while developing because you do not need to rebuild `bin/tok` manually after every change.

## Development Commands

Build the CLI:

```bash
make build
```

Run tests:

```bash
make test
```

Run Go static analysis:

```bash
make vet
```

Run linters:

```bash
make lint
```

Build the Web UI:

```bash
npm --prefix ui run build
```

Run all relevant checks before submitting changes:

```bash
make test
make vet
make lint
npm --prefix ui run build
```

## Project Structure

```text
cmd/
  tok/                 CLI entry point

internal/
  model/               Canonical platform-independent model
  source/              AI agent source adapters
  analyze/             Analysis orchestration
  rule/                Token and context diagnostic rules
  match/               Observation matching
  reconcile/           Usage reconciliation across observations
  coverage/            Tool output observability metrics
  gateway/             Local HTTP capture and gateway observations
  pricing/             Pricing catalog and cost estimation
  report/              Terminal and JSON presentation
  app/                 Application use cases
  cli/                 Cobra commands
  webui/               Local Web UI server
  config/              Configuration

ui/                    Preact + TypeScript Web UI

fixtures/              Synthetic and sanitized test data

docs/
  architecture.md
  adapter-spec.md
  rule-spec.md
  reconciliation-observation.md
```

## Architecture

TokDoctor keeps platform-specific collection separate from platform-independent analysis. Two pipelines run over the same canonical model:

```text
Agent Data
    |
    v
Source Adapter
    |
    v
Canonical Model
    |
    +--> Analyzer --> Rule Engine --> Analysis Result --> Terminal
    |                                                 --> JSON
    |                                                 --> Web UI
    |
    +--> Observation Producer --> Matcher --> Reconciler --> Reconciliation
```

The session pipeline produces findings. The observation pipeline compares
independently observed usage from the gateway and from the session transcript.
`tok inspect` runs the second; `tok doctor` runs the first.

See [`docs/architecture.md`](docs/architecture.md) for details.

## Local-First

TokDoctor is designed to work locally.

By default:

* no account is required
* no cloud service is required
* no session data is uploaded
* no source code is uploaded
* no prompt data is uploaded
* original agent session data is read-only
* the Web UI is exposed only on loopback

See [`SECURITY.md`](SECURITY.md) for the security model.

## Current Status

The Codex and Claude vertical slices are complete: discovery, streaming parse,
authoritative usage, normalization, context attribution, the `TOOL001` and
`TOOL002` rules, and terminal, JSON, and Web UI rendering.

Implemented but not yet finished:

* `tok inspect` reconciles gateway capture against a session transcript, but it returns no analysis result, so it does not show findings
* `tok doctor` runs the rules but does not reconcile
* `tok doctor` without a session ID analyzes a placeholder session instead of discovering one
* two diagnostic rules are implemented (`TOOL001`, `TOOL002`); the other rule families are specified in [`docs/rule-spec.md`](docs/rule-spec.md) but not built
* 9Router is detected and probed, but produces no sessions or usage

## Contributing

Contributions are welcome.

Before contributing, read:

* [`AGENTS.md`](AGENTS.md)
* [`CONTRIBUTING.md`](CONTRIBUTING.md)
* [`docs/architecture.md`](docs/architecture.md)
* [`docs/adapter-spec.md`](docs/adapter-spec.md)
* [`docs/rule-spec.md`](docs/rule-spec.md)

## License

Licensed under the Apache License 2.0.
