# Security Policy

TokDoctor is a local-first developer tool that analyzes AI coding-agent sessions, usage data, prompts, tool output, and related context.

Because this data may contain source code, credentials, proprietary information, or other sensitive content, security and privacy are core product requirements.

## Supported Versions

Security fixes are provided for the latest released version of TokDoctor.

When possible, users should reproduce issues against the latest release before reporting them.

## Security Principles

TokDoctor should follow these defaults:

- process agent data locally
- avoid uploading prompts, source code, session history, or tool output
- require no account
- require no cloud service
- require no external API key for local analysis
- avoid telemetry unless explicitly enabled by the user
- never modify original agent session data during analysis
- bind the local Web UI to loopback only
- avoid logging secrets or sensitive prompt contents
- keep dependencies minimal

These are product constraints, not optional implementation details.

## Local Web UI

The embedded Web UI must bind to loopback by default:

```text
127.0.0.1
```

It must not bind to:

```text
0.0.0.0
```

unless the user explicitly requests a different network exposure mode.

Any future feature that makes the Web UI remotely accessible must include an explicit security review and suitable authentication or access controls.

## Sensitive Data

Agent session data may contain:

- source code
- prompts
- tool output
- shell commands
- filesystem paths
- environment information
- API keys
- access tokens
- credentials
- repository metadata
- proprietary business information

TokDoctor should avoid persisting or exposing this data unless it is required for the requested analysis.

When diagnostic output can be produced from metadata instead of raw content, prefer metadata.

For example, prefer:

```text
tool_output_bytes: 84321
estimated_tokens: 18200
```

over printing the full tool output.

## Logging

Logs must not include secrets unnecessarily.

Avoid logging:

- API keys
- bearer tokens
- passwords
- session cookies
- private prompt contents
- full tool output
- proprietary source code

When errors need source context, prefer identifiers, paths, event types, offsets, or sanitized excerpts.

## Original Session Data

TokDoctor analysis must be read-only by default.

Do not:

- rewrite agent session logs
- modify provider usage records
- alter local coding-agent configuration
- delete source data

Any future optimization or auto-fix feature that modifies configuration must require an explicit user action and should support a preview or dry-run mode first.

## Fixtures and Test Data

Never commit real user session data unless it has been intentionally created for public testing.

Fixtures must be synthetic or carefully sanitized.

Before committing fixture data, remove or replace:

- names
- emails
- usernames
- access tokens
- API keys
- credentials
- internal hostnames
- repository URLs
- proprietary code
- real prompts containing private information
- machine-specific sensitive paths

Use the smallest fixture necessary to reproduce the behavior.

## External Sources and Adapters

Adapters may read data from local files, local databases, local HTTP services, or other platform-specific sources.

Adapters must:

- request only the data required for analysis
- avoid mutating the source
- handle malformed input safely
- avoid exposing raw sensitive data in errors
- preserve platform security boundaries
- respect local-only access when interacting with local services

An adapter must not silently enable remote access, install system services, or modify external agent configuration.

## External Adapter Protocol

If TokDoctor supports external adapter executables, treat adapter processes as separate trust boundaries.

The core should:

- pass only required information to the adapter
- validate adapter output
- tolerate malformed or unknown events safely
- avoid executing arbitrary shell content from adapter output
- clearly document how external adapters are discovered and invoked

Users should only install external adapters they trust.

## Dependencies

Dependencies increase the attack surface.

Before adding a dependency:

- prefer the Go standard library when practical
- verify that the dependency is actively maintained
- avoid unnecessary transitive dependency trees
- consider whether it introduces CGO or native code
- review security-sensitive behavior

Run:

```bash
govulncheck ./...
```

for dependency updates and security-sensitive changes when appropriate.

## Reporting a Vulnerability

Do not open a public GitHub issue for vulnerabilities that could expose sensitive data, allow unintended code execution, bypass local-only boundaries, or otherwise put users at risk.

Report security issues privately to the project maintainer using the repository's private security reporting mechanism when available.

Include:

1. affected TokDoctor version
2. operating system
3. affected source or adapter
4. clear reproduction steps
5. expected behavior
6. actual behavior
7. impact
8. proof-of-concept or sanitized logs if useful

Do not include real credentials, proprietary source code, or private user data.

If private GitHub security reporting is not available, use the maintainer contact method listed in the repository without publishing exploit details.

## Security-Relevant Issues

Examples of issues that should be reported privately include:

- Web UI unintentionally exposed outside loopback
- arbitrary command execution
- unsafe external adapter execution
- path traversal
- reading files outside the intended source scope
- leakage of prompts or source code
- leakage of credentials or tokens
- telemetry occurring without explicit consent
- mutation or corruption of original session data
- unsafe handling of local databases or sockets
- malicious session data causing unintended code execution
- secret exposure through logs, reports, or error messages

Normal parser errors, unsupported platform versions, incorrect token estimates, or UI bugs are usually not security vulnerabilities unless they create a security impact.

## Disclosure Process

After receiving a valid security report, maintainers should:

1. confirm receipt
2. reproduce and assess the issue
3. determine affected versions
4. prepare a fix
5. add regression tests where appropriate
6. release the fix
7. publish a security advisory when appropriate

Do not disclose reporter identity without permission.

## Security Reviews for New Features

Features that require explicit security consideration include:

- remote Web UI access
- cloud synchronization
- telemetry
- account systems
- authentication
- auto-fix functionality
- configuration mutation
- external adapter execution
- plugin systems
- remote APIs
- shared team data
- session upload
- LLM-based analysis using remote providers

These features should not weaken TokDoctor's local-first defaults.

## Responsible Use

TokDoctor should analyze only data the user is authorized to access.

Do not design features that intentionally bypass operating-system permissions, agent security controls, provider restrictions, or repository access controls.
