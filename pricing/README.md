# TokDoctor Pricing Catalog

`pricing/catalog.json` is TokDoctor's public maintained pricing dataset. It is data, not a Go package; the runtime implementation consumes it through `internal/pricing`.

The catalog is intentionally small and versioned. Each profile identifies a provider, canonical SKU, exact known aliases, currency, per-1M-token rates, optional context-length tiers, optional effective date, and source verification metadata.

Tiered profiles keep model-specific thresholds in data under `tiers`. A tier may set `up_to_input_tokens`; the first tier without that field is the fallback for input above the prior tier.

Only add prices that are verified from authoritative provider sources. Do not guess rates, effective dates, aliases, or provider metadata. If a model's public pricing cannot be confirmed, leave it unresolved until it can be verified.

To add or update a profile:

1. Verify the pricing from an authoritative provider source.
2. Add or update one profile in `pricing/catalog.json`.
3. Include the source URL and verification date.
4. Run `make pricing-sync` so `internal/pricing/embedded_catalog.json` stays in sync as the generated embedded fallback.
5. Commit both `pricing/catalog.json` and `internal/pricing/embedded_catalog.json`.
6. Run the pricing tests.
