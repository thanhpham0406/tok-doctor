# TokDoctor

Local-first token profiler and context optimizer for AI coding agents.

TokDoctor helps you understand where your AI coding tokens go, detect unnecessary context usage, and get actionable recommendations to reduce token consumption and cost without sacrificing quality.

> TokDoctor is currently in early development.

## Features

Current scaffold includes:

* CLI application
* terminal and JSON output
* optional local Web UI
* local-first architecture
* foundation for AI agent adapters and diagnostic rules

Initial platform targets:

* Codex
* 9Router
* Antigravity
* Kiro
* Cursor
* Claude

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

Run the doctor command:

```bash
./bin/tok doctor
```

Get JSON output:

```bash
./bin/tok doctor --format json
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
go run ./cmd/tok doctor
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
  token/               Token measurement and estimation
  report/              Terminal and JSON presentation
  webui/               Local Web UI server
  config/              Configuration

ui/                    Preact + TypeScript Web UI

fixtures/              Synthetic and sanitized test data

docs/
  architecture.md
  adapter-spec.md
  rule-spec.md
```

## Architecture

TokDoctor keeps platform-specific collection separate from platform-independent analysis:

```text
Agent Data
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
    +--> Web UI
```

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

TokDoctor is currently an initialized scaffold.

The next development milestone is the first useful Codex vertical slice:

```text
discover latest Codex session
        |
        v
stream session JSONL
        |
        v
extract authoritative token usage
        |
        v
normalize relevant events
        |
        v
identify large tool outputs
        |
        v
render tok doctor findings
```

The current `tok doctor` command does not yet perform full token analysis.

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
