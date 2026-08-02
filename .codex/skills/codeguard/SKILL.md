---
name: codeguard
description: Enforce an evidence-based, enterprise production-readiness and anti-slop quality gate for code changes, implementations, refactors, pull requests, and agent-generated software. Use when planning, reviewing, completing, or approving software whose defects, outages, data loss, security failures, poor operability, or maintenance burden could cause material business harm. Detect incomplete shortcuts, speculative code, unnecessary complexity, weak error handling, missing failure-path tests, unsafe migrations, brittle integrations, and claims unsupported by validation; return an explicit PASS, PASS_WITH_RISKS, or BLOCK decision with concrete remediation.
---

# CodeGuard

Hold software to the standard required for dependable business use. Prevent slop without turning every change into a platform rewrite.

## Establish the real contract

1. Read all applicable project instructions and authoritative architecture, security, data, API, and operational documents.
2. Define the requested scope, users, trusted boundaries, data at risk, failure cost, availability expectations, and compatibility constraints from evidence. Ask only when a missing decision materially changes the implementation.
3. Inspect the surrounding implementation, tests, configuration, migrations, deployment path, and established patterns before judging or writing code.
4. Distinguish prototype, internal utility, and production-critical paths, but never use a prototype label to excuse security, destructive behavior, secret leakage, or silent data corruption.
5. Preserve analysis-only scope unless implementation or fixes were requested.

## Prevent agent slop

Reject or correct these patterns:

- Placeholders, TODO implementations, fake success, hard-coded production values, swallowed errors, empty catches, panics as ordinary control flow, and comments that promise behavior the code does not provide.
- Happy-path-only implementations that omit timeouts, cancellation, cleanup, partial failure, retries, concurrency, malformed inputs, permission denial, dependency failure, or safe shutdown where relevant.
- Invented APIs, fields, configuration, commands, library behavior, or architectural assumptions not verified against the repository or authoritative documentation.
- Copy-pasted logic, parallel domain models, duplicated validation, inconsistent terminology, dead branches, unused abstractions, and broad unrelated rewrites.
- Generic helpers, interfaces, factories, configuration, dependencies, or extension points added for hypothetical future needs without a current consumer.
- Overcompressed code, misleading names, magic behavior, excessive indirection, giant mixed-responsibility units, and cleverness that makes failure behavior hard to reason about.
- Tests that merely mirror implementation, mock away the risk, assert only status codes, depend on timing, or fail to exercise the regression and important negative paths.
- Claims such as “secure,” “atomic,” “backward compatible,” or “tested” without direct evidence.

Do not confuse brevity with slop. Prefer the smallest complete solution that satisfies the real contract and repository architecture.

## Apply production gates

Evaluate every relevant gate in `references/production-gates.md`. Scale depth by credible impact, reachability, reversibility, and blast radius.

At minimum verify:

1. **Correctness:** invariants, boundaries, state transitions, concurrency, determinism, and negative paths are explicit and tested.
2. **Failure behavior:** errors preserve useful identity, partial work is handled, retries are safe and bounded, timeouts and cancellation propagate, and cleanup cannot silently corrupt state.
3. **Security and privacy:** authentication, authorization, tenant or ownership binding, input validation, output encoding, secrets, least privilege, audit, and data minimization hold at trusted boundaries.
4. **Data integrity:** transactions, migrations, idempotency, uniqueness, compatibility, rollback or forward recovery, and durable event publication match the system's source of truth.
5. **Operability:** startup, shutdown, configuration validation, health, observability, rate and resource bounds, degraded dependencies, recovery, and actionable diagnostics are adequate.
6. **Maintainability:** the change follows local patterns, keeps responsibilities coherent, avoids duplication and speculative abstraction, and documents durable public contracts.
7. **Delivery safety:** tests and analysis cover the actual risk; deployment ordering, compatibility windows, feature flags, rollback limits, and dependency changes are understood.

Invoke language-specific analysis skills when available and relevant, such as `$go-static-analysis` or `$nodejs-static-analysis`. CodeGuard remains responsible for the cross-cutting production decision and must verify their findings in project context.

## Demand evidence

For each concern, cite the exact file and line, the violated contract, a realistic trigger, and the consequence. Separate confirmed defects from questions and residual risks. Do not inflate severity merely because a pattern looks suspicious.

Accept evidence from source inspection, focused tests, compiler and analyzer output, migration tests, integration tests, reproducible commands, and authoritative contracts. Record checks not run and why. Never report a command as passed unless it completed successfully.

When implementing, remove only slop within scope. Do not perform opportunistic rewrites. Add focused regression coverage and run narrow checks before broad checks.

## Issue a gate decision

End with exactly one decision:

- `PASS`: no material unresolved defect; evidence is sufficient for the stated scope.
- `PASS_WITH_RISKS`: usable for the stated scope, with explicit non-blocking residual risks, owners or follow-ups, and validation gaps.
- `BLOCK`: a confirmed material defect, missing essential evidence, unsafe failure mode, architecture violation, or unrecoverable delivery risk prevents responsible use.

Report blockers first, then other findings, evidence run, skipped checks, and the decision. Do not lower the result because of style preferences. Do not raise it because the code is large, polished, or test-heavy when the critical failure paths remain unproven.
