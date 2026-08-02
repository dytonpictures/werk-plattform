# Production readiness gates

Use only gates relevant to the change. A non-applicable gate is not a failure; an unexamined relevant gate is a validation gap.

## Correctness and contracts

- Identify inputs, outputs, invariants, ownership, state transitions, and side effects.
- Check malformed, empty, duplicate, stale, boundary-size, reordered, and concurrent inputs.
- Verify public API, event, schema, file-format, and configuration compatibility.
- Check time, locale, ordering, randomness, retries, and process restart assumptions.

## Error and failure handling

- Trace representative failures from origin through classification, wrapping, logging, protocol translation, and caller behavior.
- Preserve machine-readable error identity and sanitize public details.
- Handle partial reads, writes, batches, commits, external side effects, and ambiguous outcomes.
- Bound retries and require idempotency, backoff, cancellation, and duplicate protection.
- Verify resource cleanup, shutdown, and recovery when a secondary failure occurs.

## Security and privacy

- Enforce authentication and authorization at the trusted server boundary.
- Bind tenant, organization, ownership, audience, and account type explicitly where applicable.
- Validate at trust boundaries; encode or parameterize at sinks.
- Apply least privilege to processes, tokens, database roles, files, and integrations.
- Prevent secret, credential, personal-data, stack-trace, and existence-oracle leakage.
- Audit consequential actions without logging sensitive payloads.

## Data and migrations

- Keep the authoritative store and cache or projection roles explicit.
- Make related durable state and outbox or audit changes atomic when the contract requires it.
- Test constraints, isolation, tenant enforcement, concurrent writers, and retry behavior.
- Treat applied migrations as immutable; make new migrations safe for real data and deployment ordering.
- Define backup, restore, rollback, or forward-recovery behavior for destructive or irreversible changes.

## Operations and resilience

- Validate configuration at startup and fail clearly on unsafe or missing values.
- Bound memory, CPU, goroutines or tasks, queues, payload sizes, connections, and fan-out.
- Set intentional deadlines and degraded behavior for dependencies.
- Provide meaningful health signals, structured diagnostics, metrics, and correlation without alert noise.
- Verify graceful shutdown, in-flight work handling, restart safety, and repeatable deployment.

## Maintainability and anti-slop

- Follow repository vocabulary and existing ownership boundaries.
- Keep one source of truth for rules and domain concepts.
- Prefer explicit, local code until an abstraction has multiple real consumers or enforces a critical invariant.
- Remove unused scaffolding, generated noise, debug paths, stale flags, and misleading comments.
- Avoid new dependencies when a clear, maintained existing facility already solves the problem.
- Keep diffs coherent and reviewable; split unrelated changes.

## Verification depth

- Test the reported regression and at least the credible high-impact negative paths.
- Use integration tests where mocks cannot prove database, network, process, migration, or authorization behavior.
- Check static analysis and formatting through repository-defined tools.
- Treat flaky, skipped, or environment-blocked tests as explicit evidence gaps.
- Match validation depth to expected loss, blast radius, irreversibility, and exposure.

## Severity guide

- `critical`: credible compromise, cross-tenant exposure, irreversible broad data loss, or systemic unsafe control.
- `high`: likely material outage, unauthorized action, durable corruption, or major financial harm.
- `medium`: bounded correctness, reliability, or maintainability failure with meaningful operational cost.
- `low`: limited defect or debt with a concrete impact and straightforward containment.

Do not report purely aesthetic preferences as defects.
