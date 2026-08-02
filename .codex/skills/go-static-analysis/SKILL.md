---
name: go-static-analysis
description: Perform context-aware, read-only static analysis of Go source, modules, packages, diffs, and pull-request changes. Use for Go code reviews, defect and vulnerability hunts, concurrency and resource-lifetime checks, API and error-handling audits, architecture-aware repository analysis, or interpreting go vet, staticcheck, govulncheck, test, and custom linter output. Optimize findings for coding agents by verifying each issue against source and project instructions, prioritizing actionable defects, suppressing unsupported speculation, and returning precise evidence and validation commands.
---

# Go Static Analysis

Analyze before proposing edits. Treat tool output as evidence to verify, not as the final answer.

## Establish scope and context

1. Identify the requested scope: repository, module, package, files, or diff. Do not silently expand a diff review into unrelated legacy code.
2. Read every applicable `AGENTS.md` and equivalent local instruction file from the repository root through the target path.
3. Inspect `go.work`, `go.mod`, relevant build scripts, CI configuration, generated-code markers, and changed tests. Run `scripts/collect-context.sh <root>` for a compact inventory.
4. Infer architecture only from repository evidence. Read referenced design documents when a local instruction makes them authoritative.
5. Preserve analysis-only scope. Do not edit code, install tools, update modules, or contact external services unless the user requests it.

## Select checks

Prefer repository-defined commands over invented invocations. Start with narrow checks and widen only when useful.

- Always consider formatting and compilation: `gofmt -d`, `go test`, and `go vet` on the smallest relevant package set.
- Use `staticcheck` when already available or explicitly authorized. Match its package scope to the review.
- Use `govulncheck` when dependency vulnerability analysis is requested and the tool/data access is available. Distinguish reachable findings from module-presence findings.
- Consider `go test -race` only when executable tests exercise relevant concurrency paths; explain its dynamic nature and cost.
- Respect build tags, generated files, CGO, platform constraints, workspace replacements, and repository-specific environment requirements.
- Never install a missing tool or run an untrusted project script without authorization. Report skipped checks and the exact reason.

See `references/checks.md` for selection guidance and high-value Go defect patterns.

## Verify findings

For every candidate:

1. Open the exact source and enough callers, callees, tests, and configuration to establish behavior.
2. State the violated invariant and a realistic execution path. Reject style preferences unless the repository makes them requirements.
3. Check language and library semantics against the repository's Go version. Do not rely on obsolete loop-variable, timer, range, or standard-library assumptions.
4. Check whether validation, ownership, cancellation, locking, cleanup, or authorization occurs at another layer.
5. Deduplicate symptoms that share one root cause.
6. Assign severity from demonstrated impact and reachability, not tool labels alone.

Pay particular attention to nil and zero-value behavior, ignored errors, partial I/O, resource closure, context propagation, goroutine lifetime, channel ownership, lock ordering, races, aliasing, integer and slice boundaries, SQL transaction boundaries, injection, path handling, authentication and authorization, tenant isolation, secret exposure, and unsafe or reflection-heavy code.

## Audit error handling

Trace representative failures end to end instead of checking only whether `err != nil` exists.

1. Verify every returned error is handled, deliberately propagated, or explicitly safe to ignore. Treat cleanup, flush, close, rollback, and commit errors according to their durability impact.
2. Preserve error identity where callers depend on `errors.Is`, `errors.As`, sentinel values, typed errors, or status mapping. Flag wrapping with `%v`, string matching, and accidental replacement when they break the contract.
3. Check that partial results and partial writes are not mistaken for success and that state remains consistent after failure.
4. Verify retries are limited to safe, transient, and idempotent operations; require bounded backoff, cancellation, and no duplicated side effects.
5. Check boundary translation for HTTP, RPC, CLI, worker, and persistence layers. Prevent internal details, secrets, and existence or authorization oracles from leaking.
6. Check logging and metrics for sufficient correlation without duplicate logging, sensitive values, or misleading success signals.
7. Require tests for important negative paths, including dependency failure, cancellation, timeout, rollback, and ambiguous commit outcomes.

## Report for agents

Lead with findings ordered by severity. Use this shape for each confirmed issue:

```text
[severity] concise defect title
Location: path/file.go:line
Evidence: what the code does and the concrete path that triggers it
Impact: observable correctness, security, availability, or maintainability consequence
Recommendation: smallest safe correction, including constraints to preserve
Validation: focused command or test that would prove the correction
Confidence: high | medium | low
```

Use `critical`, `high`, `medium`, or `low`; omit informational style notes unless requested. Cite exact lines. Separate confirmed findings from open questions. If no defects are confirmed, say so and list residual risks and checks not run. Never claim a check passed unless its command completed successfully.

## Handle fixes

When the user asks for fixes, make the smallest coherent change, preserve public contracts unless explicitly changing them, add a regression test where practical, format touched Go files, and run focused checks before broader repository checks. Report both completed and skipped validation.
