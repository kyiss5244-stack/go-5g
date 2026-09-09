# LiteCore

<!-- impeccable:product-schema 1 -->

## Platform
web

## Product Purpose
Existing frontend demonstration of a lightweight Go 5G core simulation.

## Capabilities and Constraints
Preserve UE registration demonstration, scenario selection, registration records, latency analysis and the WebMCP registration tool. Go services use gRPC; the frontend has no HTTP integration. UI actions are simulations, not live network operations. Existing benchmark.csv contains 100 historical samples, 86 successful registrations. Do not present historical successes as currently active sessions or invent measured timings.

## Brand Commitments
Preserve LiteCore name. The user requested a more beautiful, less AI-like and mechanical interface.

## Evidence on Hand
../results/benchmark.csv; app/page.tsx; ../README.md.

## Open Decisions
Audience inferred: people demonstrating or studying the core-network experiment. The user has been asked to confirm frontend-only scope; no backend expansion is authorized or implemented.
