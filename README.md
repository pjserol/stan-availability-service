# Streaming Availability Service

Go HTTP service between Stan's transcoder and CDN origins: ingests segment
availability events and serves ordered per-asset manifests.

**Live:** https://stan-availability-service.onrender.com (Render free tier, Singapore).
The free tier sleeps after ~15 minutes idle: the first request can take up to a minute, and in-memory state resets on wake or redeploy.

## Getting started

### Prerequisites

- Go 1.27+ (`go version`)
- Optional: Docker, `make`, `curl`, `jq`

No database or external services are needed; state is held in memory.

### Clone

```sh
git clone https://github.com/pjserol/stan-availability-service.git
cd stan-availability-service
```

### Run with Go

```sh
go run ./cmd/server            # or: make run
```

The service listens on `http://localhost:8080`. Override settings with env vars:

```sh
PORT=9000 SHUTDOWN_TIMEOUT=30s go run ./cmd/server
```

| Env var | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | Listen port (set by Render) |
| `SHUTDOWN_TIMEOUT` | `15s` | Drain timeout on SIGTERM (Go duration, e.g. `500ms`, `30s`) |

### Run with Docker

```sh
docker build -t stan-availability-service .
docker run --rm -p 8080:8080 stan-availability-service
```

### Try it

```sh
curl -s localhost:8080/health | jq

curl -s -X POST localhost:8080/ingest \
  -H 'Content-Type: application/json' \
  -d '{"asset_id":"show/thunderbirds/season/1/ep3","segment_index":0,"cdn_url":"https://cdn.example.com/thunderbirds/s1e3/seg0.ts","received_at":"2024-05-01T10:00:00Z"}' | jq

curl -s localhost:8080/manifest/show/thunderbirds/season/1/ep3 | jq
```

### Stop

`Ctrl+C` (SIGINT) or `kill -TERM <pid>` triggers a graceful shutdown: the server stops accepting connections and waits up to `SHUTDOWN_TIMEOUT` for in-flight requests to finish. If the drain times out, remaining connections are closed and the process exits with status 1. A second signal during the drain force-quits.

Keep `SHUTDOWN_TIMEOUT` below the platform's shutdown grace period (about 30s on Render), otherwise the platform kills the process mid-drain.

## Tests

```sh
go test ./...                                  # or: make test
go test -race ./...                            # or: make race
go test -run='^$' -bench=. -benchmem ./...     # or: make bench
```

CI runs `go vet`, `go test` and `go test -race` on every pull request.

**Smoke test** against any running instance, replaying the brief's scenarios 1–6 (unique asset IDs per run, so it is safe against the live service):

```sh
scripts/smoke.sh http://localhost:8080
scripts/smoke.sh https://stan-availability-service.onrender.com
```

**Benchmarks** (Apple M3 Pro):

| Benchmark | Result |
|---|---|
| `GET` manifest of 10 / 1,000 / 10,000 segments | 0.27 µs / 58 µs / 0.74 ms, 1 allocation each |
| Parallel `Put` across 1,000 assets, 11 cores | 39 ns/op (~25M writes/s) |

## Project layout

```
cmd/server/       main: config, listener, signal handling, graceful shutdown
internal/config/  PORT and SHUTDOWN_TIMEOUT from the environment
internal/store/   sharded in-memory segment store
internal/api/     HTTP handlers, JSON helpers, request logging middleware
scripts/smoke.sh  end-to-end check against a running instance
```

## Architecture

```mermaid
flowchart TB
    T["Transcoder"] -- "POST /ingest" --> MW
    C["CDN origin"] -- "GET /manifest/asset_id" --> MW
    P["Health check"] -- "GET /health" --> MW

    subgraph svc["stan-availability-service"]
        MW["Logging middleware<br/>one log line per request"] --> H["Handlers<br/>ingest · manifest · health"]
        H -- "Put / Get" --> R
        subgraph store["In-memory store: 64 shards"]
            R["Pick the asset's shard<br/>FNV-1a hash of asset_id, mod 64"]
            R --> S0["Shard 0<br/>its own lock<br/>+ its assets' segments"]
            R --> S1["Shard 1<br/>its own lock<br/>+ its assets' segments"]
            R --> SN["… Shard 63<br/>its own lock<br/>+ its assets' segments"]
        end
    end
```

Each shard maps `asset_id → segment_index → {segment, received_at}`. Requests for different assets usually land on different shards and never wait for each other.

```mermaid
sequenceDiagram
    participant T as Transcoder
    participant C as CDN origin
    participant H as Handler
    participant S as Asset's shard

    T->>H: POST /ingest
    H->>H: decode (1 MB cap) and validate, else 400 / 413
    H->>S: take the shard's write lock
    S->>S: store segment if new, or newer received_at
    H->>S: release the lock
    H-->>T: 202 accepted

    C->>H: GET /manifest/asset_id
    H->>S: take the shard's read lock (shared)
    S->>S: copy segments and latest received_at, else 404
    H->>S: release the lock
    H->>H: sort the copy by index, outside the lock
    H-->>C: 200 manifest
```

On SIGTERM the server stops accepting connections, drains in-flight requests for up to `SHUTDOWN_TIMEOUT`, then exits.

## API

### `POST /ingest`

Records that a segment is available.

```json
{
  "asset_id": "show/thunderbirds/season/1/ep3",
  "segment_index": 4,
  "cdn_url": "https://cdn.example.com/thunderbirds/s1e3/seg4.ts",
  "received_at": "2024-05-01T10:00:00Z"
}
```

`202 Accepted` → `{"status":"accepted"}`

### `GET /manifest/{asset_id}`

Returns the asset's segments sorted by index. Asset IDs may contain slashes.

```json
{
  "asset_id": "show/thunderbirds/season/1/ep3",
  "segments": [
    { "index": 0, "cdn_url": "https://cdn.example.com/thunderbirds/s1e3/seg0.ts" },
    { "index": 1, "cdn_url": "https://cdn.example.com/thunderbirds/s1e3/seg1.ts" }
  ],
  "last_updated": "2024-05-01T10:00:05Z"
}
```

### `GET /health`

`200 OK` → `{"status":"ok","uptime_seconds":142}`

### Errors

All errors are JSON: `{"error":"<message>"}`.

| Status | Message | When |
|---|---|---|
| 400 | `Could not decode request: JSON parsing failed` | Invalid JSON, wrong field type, empty body, trailing data |
| 400 | `missing required field: <name>` | A field is missing, `null` or an empty string |
| 400 | `invalid field: <name>` | Negative `segment_index`, non-http(s) `cdn_url`, non-canonical `asset_id` |
| 404 | `asset not found` | No segment ingested for that asset |
| 413 | `request body too large` | Body over 1 MB |

## Design decision: a sharded store instead of one lock

The decision I'd most want to discuss is how the in-memory store handles concurrency. The brief rules out a single global mutex, so the store is split into 64 shards chosen by an FNV-1a hash of the asset ID, each guarded by its own `sync.RWMutex`; requests for unrelated assets usually touch different shards and never wait on each other. I considered adding a second lock per asset, which would remove the remaining contention between assets sharing a shard, but it means two lock levels to keep in order and a race when two requests create the same asset, while the critical sections are already tiny. To keep them tiny, `Get` copies an asset's segments under the shard's read lock and sorts the copy after releasing it, so the O(n log n) sort never blocks writers. The shard count started at 16 on reasoning alone; a parallel `Put` benchmark then showed contention at 11 cores, and raising it to 64 made writes 1.6× faster (63 → 39 ns/op). Duplicates are resolved by `received_at` rather than arrival order, so the outcome doesn't depend on which goroutine takes the lock first. A test with 8 concurrent writers and 4 readers under the race detector checks that every manifest stays sorted and duplicate-free, and I confirmed it fails when a lock is removed. The main limit is a single very hot asset, which still lands on one shard; there I'd cache the sorted manifest on write so reads become almost free.

## Assumptions

- **In-memory, single instance.** State is lost on restart, redeploy or free-tier sleep, as the brief allows. No persistence or replication.
- **Duplicates: newest `received_at` wins.** A segment with the same asset and index replaces the stored one only if its `received_at` is strictly newer; equal or older copies are ignored. Retries are therefore idempotent, a late stale retry can't roll back a correction, and the result doesn't depend on arrival order.
- **`last_updated` is the latest `received_at`** among the asset's stored segments (event time, reported in UTC), not the server clock. It is deterministic and survives replays. `received_at` is trusted as set by the transcoder.
- **Gaps are returned as-is.** If segment 3 hasn't arrived yet, the manifest is `[0, 1, 2, 4]`: sorted, but not necessarily contiguous.
- **Validation beyond the required fields**, checked in a fixed order (`asset_id`, `segment_index`, `cdn_url`, `received_at`) so a bad request always gets the same message:
  - unknown JSON fields are ignored, so the transcoder can add fields without breaking ingest;
  - trailing data after the JSON object is a decode error;
  - empty strings count as missing;
  - `segment_index` must be ≥ 0, and `cdn_url` an absolute `http`/`https` URL;
  - `asset_id` must be canonical (no leading or trailing `/`, `//`, `.` or `..` segments): the HTTP router normalises those paths, so such an asset could be stored but never fetched;
  - bodies over 1 MB return 413. Real events are about 200 bytes.
- **The store takes no `context.Context`.** Its operations are in-memory and take microseconds, so there is nothing to cancel. Contexts carry shutdown (signal → drain deadline) and request-scoped logging.
- **`/ingest` is trusted.** It is called by the internal transcoder, so there is no authentication or rate limiting.

## Notes on the spec

Error messages are returned exactly as specified, since clients and tests may match them verbatim. The spec mixes styles (*"Could not decode request: …"* vs *"missing required field: …"* and *"asset not found"*). In a production API I'd standardise on one convention and add a stable machine-readable code, for example RFC 9457 Problem Details with a `type` field, so clients never depend on human-readable text. All errors go through a single `writeError` function, so that change would be made in one place.

## Scenario 7: high-volume burst

*200 goroutines posting segments for 500 assets while 50 goroutines poll random manifests.*

### Where it breaks first

The store is not the first bottleneck. Benchmarks put a `Put` at about 39 ns with 11 cores (around 25M writes/s), and 500 assets spread over 64 shards is about 8 assets per shard, with locks held only for a map write or a copy. In order of what fails first:

1. **CPU in the HTTP layer.** JSON decoding and encoding, TLS and connection handling cost far more per request than the store. On a small instance (Render's free tier has a fraction of a CPU), latency climbs here first.
2. **Manifest reads on long assets.** Each `GET` copies and sorts every segment of the asset: about 0.74 ms at 10,000 segments, repeated on every poll. Hot or long live assets make this, and its allocations and GC pressure, the main cost.
3. **Memory.** Nothing is ever evicted, so a long-running instance grows with every asset and segment until it runs out of memory.
4. **A single hot asset.** All of its traffic lands on one shard; sharding spreads different assets, not one.

### What I would change

- **Cache the manifest on write.** Rebuild an immutable, sorted snapshot (even pre-encoded JSON) when a segment is actually stored, and publish it through an atomic pointer, so reads stop sorting and locking. Reads vastly outnumber writes, so writes should pay.
- **`ETag` + `If-None-Match` → `304 Not Modified`** from a per-asset version bumped on every write: cheap polls, and correct change detection (unlike `last_updated`).
- **Bound memory:** a TTL or eviction for finished assets, and a cap on segments per asset.
- **Protect the service under overload:** limit concurrent requests and shed load with `503` + `Retry-After` instead of degrading for everyone.
- **Scale out** beyond one instance with state outside the process: a shared store (for example Redis sorted sets keyed by asset), or routing each asset to one instance by consistent hashing. This also fixes losing state on every deploy.
- **Put a CDN or cache in front** of `GET /manifest` with a short TTL, so most polls never reach the service.

### Observability before production

- **RED metrics per route:** request rate, error rate by status and message, and latency histograms (p50/p99).
- **Store metrics:** number of assets and segments, manifest size distribution, and memory/GC from `runtime/metrics`.
- **Contention:** mutex and block profiles (`pprof`) to see whether any shard is hot.
- **Pipeline freshness:** ingest lag (`now − received_at`) and an alert when a live asset's `last_updated` stops moving.
- **Tracing and correlation:** a request ID on every log line and span, plus an instance ID to tell replicas apart.
- **Alerts:** p99 latency, 5xx rate, memory headroom and restarts.

## Known limitations

- **`last_updated` is not a change detector.** It is the newest `received_at`, so a late segment with an older timestamp changes the manifest without advancing it (for example, segment 3 arriving after segment 4). Clients shouldn't skip reprocessing just because `last_updated` is unchanged. Production fix: an `ETag` or per-asset version that changes on every write, with `If-None-Match` → `304 Not Modified` for pollers.
- **In-memory, single instance.** Every restart, redeploy or free-tier sleep starts empty, and there is no replication. A shared store would fix both (see Scenario 7).
- **No eviction.** Memory grows with every asset and segment; finished assets should expire.
- **Gaps are not flagged.** A manifest can be `[0, 1, 2, 4]` while segment 3 is in flight, and a player may stall at the gap. Options: return only the contiguous prefix, or report missing indexes.
- **Reads sort on every request.** Fine for normal assets (58 µs at 1,000 segments); a hot or very long asset would benefit from the cached snapshot described in Scenario 7.
- **`/ingest` has no authentication or rate limiting**, on the assumption that only the internal transcoder calls it.
- **Panics are recovered by `net/http`, not by the service.** A panicking handler drops the connection, and its stack trace goes to stderr unstructured. A recovery middleware returning `500` was considered and left out to keep the middleware simple.
- **Limited observability.** One structured log line per request, but no metrics, tracing or request IDs, and logs don't identify the instance.
- **`received_at` is trusted.** A transcoder with a skewed clock would skew `last_updated` and the newest-wins rule.
- **Free-tier hosting.** The first request after ~15 minutes idle can take up to a minute.

## Production readiness: what I'd add next

Beyond scaling (Scenario 7) and the gaps above, these are the engineering practices I'd put in place before running this in production with a team.

**Contract**
- **API spec first.** An OpenAPI 3.1 file becomes the source of truth: server interfaces and types generated from it (e.g. `oapi-codegen`), clients generated for the transcoder and CDN teams, docs published from the same file, and CI failing if the code drifts from the contract. Errors move to RFC 9457 Problem Details.

**Delivery**
- **Per-environment configuration.** Staging and production values live in the deployment platform (one `render.yaml` or Helm values file per environment), secrets in a secret manager, and a committed `.env.example` documents the variables for local development.
- **A CD pipeline with a promotion gate.** Images tagged by commit SHA deploy to staging first; the end-to-end suite (`scripts/smoke.sh`) runs against it, and only a green run promotes to production. Rollback is a redeploy of the previous image.
- **Supply-chain checks in CI:** `golangci-lint`, `govulncheck`, automated dependency updates and container image scanning.

**Quality**
- **Libraries where they pay off.** Standard library only was a constraint of this challenge, not a production goal: well-maintained libraries for configuration, metrics and tracing (OpenTelemetry) save time and bugs.
- **Scheduled load tests** reproducing the Scenario 7 traffic, so performance regressions are caught before release.

**Team and operations**
- **AI skills for conventions and onboarding.** Commit agent instructions and skills that encode how this codebase is written (table-driven tests with sentinel errors, logging, API design, PR descriptions), so new developers and AI assistants follow the same patterns from day one.
- **Architecture Decision Records.** The decisions behind this service (sharding, newest-wins duplicates, event-time `last_updated`, drain semantics) recorded next to the code, so the reasoning survives team changes.
- **SLOs, alerts and runbooks.** Targets such as manifest p99 latency and availability, alerts tied to them, and a short on-call runbook.
- **Service-to-service authentication and rate limiting on `/ingest`** (mTLS or signed tokens), instead of assuming an internal caller.
