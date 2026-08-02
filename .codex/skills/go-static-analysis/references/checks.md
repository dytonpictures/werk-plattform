# Go analysis reference

## Check selection

| Signal | Preferred check | Qualification |
|---|---|---|
| Formatting | `gofmt -d <files>` | Generated files may be excluded by repository policy. |
| Compile and behavior | `go test <packages>` | Compilation can depend on build tags, services, or platform. |
| Standard suspicious constructs | `go vet <packages>` | Verify every diagnostic in its real call path. |
| Broader static rules | `staticcheck <packages>` | Use repository configuration and installed version. |
| Known Go vulnerabilities | `govulncheck <packages>` | Record database freshness and reachable-symbol evidence. |
| Data races | `go test -race <packages>` | Dynamic, platform-dependent, and incomplete without coverage. |
| Dependency integrity | `go mod verify` | Does not prove dependencies are safe or current. |

## High-value review paths

- Trace errors across wrapping, classification, retries, and HTTP/RPC translation.
- Trace `context.Context` cancellation from entry point to blocking operation.
- Match every acquired resource with ownership and cleanup on all exits.
- Establish goroutine termination, channel closer ownership, and synchronization.
- Examine transaction scope, rollback behavior, commit ambiguity, and atomic outbox requirements.
- Verify authorization at the trusted server boundary and tenant binding at every data access.
- Treat map iteration order, time, randomness, and environment as nondeterministic unless normalized.
- Check parsing and size limits before allocation, decompression, recursion, or persistence.

## Error-handling review paths

- Follow creation, wrapping, inspection, translation, logging, and final presentation of each important error class.
- Check `defer` paths for close, flush, unlock, rollback, and panic interactions; named returns can accidentally hide deferred errors.
- Distinguish `io.EOF`, short reads and writes, cancellation, deadlines, optimistic conflicts, not-found, invalid input, forbidden access, and infrastructure failure.
- Verify transaction helpers do not discard rollback errors or report success after an ambiguous commit.
- Ensure goroutine errors reach an owner through a bounded channel, `errgroup`, cancellation, or another explicit contract.
- Ensure public responses are stable and sanitized while internal logs retain a correlation ID and useful causal context.

## False-positive controls

- Do not report a nil dereference without showing how nil reaches the operation.
- Do not report a leak when ownership is intentionally transferred and documented by use.
- Do not report a race based only on shared data; establish concurrent access and missing synchronization.
- Do not treat all ignored cleanup errors equally; explain the consistency or durability impact.
- Do not apply pre-Go-1.22 loop capture advice to newer semantics without checking module language version.
- Do not call a dependency vulnerable solely because its version appears in `go.mod`; distinguish reachability and replacement directives.
