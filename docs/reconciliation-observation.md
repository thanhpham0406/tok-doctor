# Canonical Observation Contract

## Purpose

An observation is a platform-independent record of token usage seen through one
source. It is the common input for gateway observation, agent telemetry, and
persisted session transcripts. Adapters produce observations; matchers and the
reconciler consume them. The contract itself contains no platform-specific
logic.

## Channel vs Scope

Channel and scope answer different questions and are independent:

- **Channel** describes *how* the data was observed.
- **Scope** describes *what level* the record represents.

Any channel may use any scope. The adapter decides which combinations its source
can actually produce; the contract does not restrict them.

## Channels

- `gateway` — the gateway observed the HTTP request/response directly.
- `agent_telemetry` — a coding agent runtime (for example Codex or Claude Code)
  recorded the event or request telemetry.
- `session_transcript` — the record was reconstructed from a persisted session
  log or transcript.

## Identity vs Correlation

`Observation.ID` is the stable internal identifier of the observation record.
It is not a correlation key. Correlation evidence lives in
`ObservationIdentity`, and each field is a separate namespace. Invariants:

- An observation ID is not a correlation identity.
- A provider request ID is never copied into a response object ID.
- A session ID is never copied into an agent request ID.
- Identity is never synthesized from a timestamp, token total, or model name.
- A field that was not observed is left empty.
- Canonical identity never contains raw platform payload.

## Identity Fields

- `exchangeId` — gateway exchange record.
- `agentRequestId` — request identifier written by the coding agent itself.
- `providerRequestId` — request identifier reported by the provider.
- `responseObjectId` — identifier of the provider response object.
- `parentResponseObjectId` — canonical, neutral name for a parent response
  reference (for example an OpenAI `previous_response_id` may map here).
- `sessionId` — session identifier.
- `turnId` — per-turn identifier within a session.
- `invocationId` — invocation-level identifier.
- `chainId` — an explicit correlation chain when the source provides one.

An entirely empty identity is valid: an observation may have no correlation key
yet.

## Missing vs Explicit Zero

Usage reuses `model.Measurement`. Each field distinguishes:

- **Missing** — `Measurement{}` or `Measurement{Kind: MeasurementUnknown}` with
  `Value == nil`. The source did not observe the value.
- **Explicit zero measured** — `NewMeasurement(0, MeasurementMeasured)`. The
  source observed a real zero.

JSON round-trips preserve this: explicit zero keeps a non-nil `Value`, missing
keeps `Value == nil`. Missing is never converted to zero, and unknown is never
converted to measured.

## Provenance Per Usage Field

Provenance is attached to each usage field, not to the usage block as a whole.
A single observation may mix kinds, for example:

- `freshInput`: measured
- `cachedInput`: measured
- `totalInput`: derived
- `output`: measured
- `total`: derived

There is no shared `kind` for the whole usage record.

## Outcome vs Completeness

Outcome and completeness answer different questions and must not be conflated:

- **Outcome** describes how the request ended — `succeeded`, `provider_error`,
  `transport_failure`, `canceled`, `truncated`, or `unknown`.
- **Completeness** describes how much of the data was observed — `complete`,
  `partial`, or `unknown`.

A request that fails can still be completely observed, because the gateway
finished observing a failure even when the provider reported no usage. For
example, an upstream HTTP error is `provider_error` + `complete`: the outcome is
a failure, but nothing about that failure is missing. Only `truncated` forces
`partial`.

Outcome and completeness are independent: one never forces the other as a
contract invariant. In the current gateway projection, an unknown gateway
outcome yields `unknown` completeness because that adapter has no other
completeness evidence. That is an adapter policy, not a global rule. A persisted
session transcript can report `outcome=unknown` together with
`completeness=complete`, because the transcript may contain full token usage
while providing no request outcome.

Completeness is reported by the adapter. The canonical model never infers it.

## Completeness

- `complete` — the adapter has evidence that the observation finished according
  to the source/protocol.
- `partial` — only part was observed, for example a truncated stream or a record
  missing expected parts.
- `unknown` — there is not enough evidence to decide between complete and
  partial.

## Authority Is Not Decided Here

The contract does not decide which source is authoritative, whether a total must
equal input plus output, or which identities must be present for a given channel
or scope. Those are adapter, matcher, and reconciler concerns.

## Adapters

An adapter detects its source, reads platform data, and converts it into one or
more observations. Platform-specific formats stay inside the adapter. Adapters
must not invent identity or completeness.

## Claude Transcript Projection

A Claude session transcript produces one session-scope observation and one
turn-scope observation per normalized turn, in source order. The session
observation is projected first.

Usage mapping:

| Claude usage field | Observation field | kind |
| --- | --- | --- |
| `input_tokens` | `freshInput` | measured |
| `cache_read_input_tokens` | `cachedInput` | measured |
| `cache_creation_input_tokens` | `cacheCreationInput` | measured |
| sum of the three input components | `totalInput` | derived |
| `output_tokens` | `output` | measured |
| `totalInput + output` | `total` | derived |
| reasoning output | missing | — |

`input_tokens` is fresh input only. Claude does not expose a separate reasoning
field, so reasoning is left missing rather than inferred. `totalInput` is
derived only when fresh input, cache read, and cache creation are all present;
`total` is derived only when `totalInput` and `output` are present. A missing
cache field is never treated as zero.

### Missing vs Explicit Zero

A JSON field that is absent produces a missing measurement with a nil value. A
field present with `0` produces a measured explicit zero with a non-nil value.
An empty usage object `{}` and a missing usage object both produce no usage at
all. Negative raw values are never published as token measurements; the
affected field becomes unknown and dependent derived fields stay missing.

### Outcome and Completeness

Transcript projection always reports `outcome=unknown`: a persisted transcript
does not prove how the request ended. Completeness is independent of outcome
and is reported as `unknown` when no usable usage is present, `partial` when
some usage is missing or non-authoritative, and `complete` when fresh input,
cached input, cache creation input, output, and the derived totals are all
present. `outcome=unknown` with `completeness=complete` is valid.

### Session Aggregates

A session aggregate is authoritative per field only when every accepted turn
reports that field with a valid non-negative value. A field missing from any
accepted turn, or reported as negative, is not published as a full session
number; it becomes unknown and dependent derived totals stay missing. Partial
turns from different calls are therefore never combined into a session that
looks complete. Explicit zeros are valid reported values: a session whose
provider-reported fields are all zero is retained and projected with non-nil
zero values.

### Identity

The adapter does not synthesize provider or agent identity. The session
observation carries only `sessionId`; each turn observation carries `sessionId`
and `turnId`. Exchange, agent request, provider request, response object,
parent response object, invocation, and chain identifiers are left empty.

### Duplicates and Conflicts

Exact duplicate records sharing a provider `message.id` with identical usage
collapse to a single turn and are not double-counted. When records share a
`message.id` but report different usage, the turn is non-authoritative: its
numeric usage is not published, its completeness is `partial`, and the session
aggregate excludes it. A conflict is never presented as provider-measured
truth.

## Matcher

A matcher groups observations that describe the same underlying activity. It
uses identity and time. Matching logic is outside this contract and does not
live in `internal/model`.

## Reconciler

The reconciler compares usage across matched observations and reports deltas.
It distinguishes measured from estimated values and never presents estimated
attribution as provider-reported usage.

## Scope Safety

Do not compare a request-scope observation directly against a session-scope
aggregate. Their usage describes different levels and must be reconciled only
after matching compatible scopes.

## Privacy

Observations never store raw prompts, raw payloads, raw responses, credentials,
API keys, or absolute private session paths. Evidence holds audit-safe
references, not source content.

## Schema Version

The current contract is `ObservationSchemaVersion = 1`, emitted as
`schemaVersion` on every observation.
