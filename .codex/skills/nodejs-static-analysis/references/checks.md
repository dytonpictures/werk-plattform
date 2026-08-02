# Node.js analysis reference

## Check selection

| Signal | Preferred check | Qualification |
|---|---|---|
| Types | Repository typecheck script | `tsc --noEmit` is not equivalent in every build setup. |
| Lint rules | Repository lint script | Use the project's local ESLint or configured alternative. |
| Syntax | `node --check <file>` | JavaScript only; module/runtime resolution is not exercised. |
| Tests | Focused repository test command | Dynamic evidence, not a complete static proof. |
| Dependency advisories | Declared package-manager audit | May require network and contains non-reachable findings. |
| Dead exports/files | Configured Knip or equivalent | Framework conventions can create false positives. |

## High-value review paths

- Trace every promise to `await`, return, aggregation, or intentional documented detachment with error handling.
- Trace `AbortSignal` and timeouts through fetches, streams, subprocesses, and database calls.
- Match streams, sockets, file handles, timers, listeners, and subprocesses with cleanup on all exits.
- Inspect trust-boundary parsing even when TypeScript types look precise; types do not validate runtime input.
- Check shell, SQL, HTML, URL, header, filesystem path, and dynamic import construction at their actual sinks.
- Confirm server-only secrets and modules cannot enter client bundles.
- Verify authorization and tenant context on the trusted server for every data operation.
- Check synchronous CPU or I/O work on request paths and attacker-controlled size amplification.
- Validate package exports, file extensions, resolution mode, and CJS/ESM interop against the supported Node runtime.

## Error-handling review paths

- Follow creation, `cause`, wrapping, classification, translation, logging, and final presentation of important error classes.
- Inspect `Promise.all`, `allSettled`, races, event emitters, streams, callbacks, async iterators, and detached tasks for lost or delayed failures.
- Distinguish aborts, timeouts, validation errors, not-found, conflicts, forbidden access, rate limits, and infrastructure failures.
- Check whether `finally`, cleanup, response writing, or stream destruction can mask the original error or produce double responses.
- Verify process-level `uncaughtException` and `unhandledRejection` handlers perform controlled diagnostics and shutdown rather than pretending recovery.
- Ensure public responses are stable and sanitized while internal telemetry retains correlation and useful causal context.

## False-positive controls

- Do not report an unhandled promise when a framework intentionally consumes the returned promise; verify the contract.
- Do not call all `any` usage a defect; show a lost invariant at a trust or API boundary.
- Do not report prototype pollution without an attacker-controlled key reaching a vulnerable merge or assignment.
- Do not report ReDoS solely from a complex regex; establish attacker-controlled input and problematic growth.
- Do not equate an audit advisory with exploitability; inspect imported symbols, deployment scope, and mitigations.
- Do not assume browser globals, DOM behavior, or edge-runtime APIs in a Node process, or vice versa.
