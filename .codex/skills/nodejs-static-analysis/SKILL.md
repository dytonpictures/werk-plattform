---
name: nodejs-static-analysis
description: Perform context-aware, read-only static analysis of Node.js JavaScript and TypeScript source, packages, workspaces, diffs, and pull-request changes. Use for Node.js code reviews, defect and vulnerability hunts, async, error-handling, and resource-lifetime checks, type-safety and module-boundary audits, dependency analysis, or interpreting ESLint, TypeScript, package-manager audit, test, and framework-specific output. Optimize findings for coding agents by detecting the project's runtime, package manager, module system, workspace, scripts, and local instructions; verifying every issue against source; suppressing unsupported speculation; and returning precise evidence and validation commands.
---

# Node.js Static Analysis

Analyze before proposing edits. Treat linter, compiler, and audit output as evidence to verify.

## Establish scope and context

1. Identify the requested scope: repository, workspace, package, files, or diff. Keep change reviews focused on changed behavior and directly affected code.
2. Read every applicable `AGENTS.md` and equivalent local instruction file from root through the target path.
3. Run `scripts/collect-context.sh <root>` to inventory the runtime, lockfiles, workspaces, manifests, module mode, TypeScript and lint configuration, and available package managers.
4. Inspect the root and target `package.json`, the authoritative lockfile, `engines`, `packageManager`, scripts, workspace configuration, `tsconfig` inheritance, ESLint configuration, test setup, and generated-code markers.
5. Infer framework and architecture from dependencies and source, not directory names alone. Read authoritative project design documents when local instructions require them.
6. Preserve analysis-only scope. Do not edit code, execute lifecycle scripts, install packages, mutate lockfiles, or access a registry unless requested or authorized.

## Select checks

Use the declared package manager and repository-defined scripts. Prefer offline, non-mutating checks and the smallest relevant workspace scope.

- TypeScript: run the repository's typecheck script, or `tsc --noEmit` only when configuration and local binaries make that equivalent.
- Linting: use the repository's ESLint script/configuration and existing local binary. Do not substitute a globally different major version.
- Tests: run focused tests when needed to validate a static finding; distinguish static analysis from dynamic test results.
- Dependencies: use the declared package manager's audit only when requested and network/cache conditions permit. Do not change the lockfile. Verify exploitability, runtime versus development scope, and framework mitigations.
- Consider Node's built-in checks and project-specific tools such as Biome, oxlint, Knip, or framework compilers only when configured or requested.
- Report missing tools and skipped checks. Never install dependencies or run `npm install`, `pnpm install`, or `yarn` implicitly.

See `references/checks.md` for tool choices and high-value Node.js defect patterns.

## Verify findings

For every candidate:

1. Open the exact source plus relevant callers, consumers, tests, types, configuration, and runtime boundary.
2. State a realistic input and execution path. Reject stylistic disagreement unless it violates an explicit project rule.
3. Check semantics against the declared Node.js, TypeScript, framework, and module versions.
4. Account for ESM versus CommonJS, conditional exports, transpilation, server versus browser bundles, edge runtimes, and workspace package boundaries.
5. Check whether validation, escaping, cleanup, authorization, or error translation occurs in another layer.
6. Deduplicate shared root causes and assign severity from demonstrated impact and reachability.

Prioritize unhandled promise failures, floating promises, callback/promise double completion, stream and listener leaks, missing abort propagation, race-prone shared state, event-loop blocking, unsafe parsing or allocation, prototype and object-key hazards, injection, path traversal, SSRF, XSS, secret leakage, authorization gaps, tenant confusion, type erasure at trust boundaries, CJS/ESM incompatibility, and dependency/runtime mismatches.

## Audit error handling

Trace representative failures from their origin to the process, protocol, job, or UI boundary.

1. Verify every promise is awaited, returned, aggregated, or intentionally detached with explicit rejection handling. Check callbacks for thrown errors, double completion, and lost asynchronous exceptions.
2. Preserve error causes, codes, types, and framework contracts. Flag blanket catches, empty catches, string-only matching, and wrapping that destroys machine-readable identity.
3. Check `try`, `catch`, `finally`, stream events, and resource-disposal paths for partial success, leaked resources, or a second error masking the primary failure.
4. Verify retries are restricted to safe, transient, idempotent operations and include bounds, backoff, jitter where appropriate, timeout, cancellation, and duplicate-side-effect protection.
5. Check boundary translation for HTTP, queues, subprocesses, server actions, CLI commands, and persistence. Avoid leaking stack traces, secrets, internal paths, or existence and authorization oracles.
6. Check logging and telemetry for correlation and causal context without duplicate logs, sensitive payloads, or false success metrics.
7. Require focused negative-path tests for rejection, timeout, abort, stream failure, dependency failure, partial writes, and shutdown behavior.

## Report for agents

Lead with confirmed findings ordered by severity:

```text
[severity] concise defect title
Location: path/file.ts:line
Evidence: what the code does and the concrete path that triggers it
Impact: observable correctness, security, availability, or maintainability consequence
Recommendation: smallest safe correction, including constraints to preserve
Validation: focused command or test that would prove the correction
Confidence: high | medium | low
```

Use `critical`, `high`, `medium`, or `low`; omit informational style notes unless requested. Cite exact lines. Separate confirmed findings, open questions, and tool-only diagnostics. If no defects are confirmed, say so and list residual risks and checks not run. Never claim a check passed unless its command completed successfully.

## Handle fixes

When fixes are requested, preserve the existing package manager and lockfile format, make the smallest coherent change, add a regression test where practical, format only touched files through repository tooling, and run focused checks before broad workspace checks. Report all completed and skipped validation.
