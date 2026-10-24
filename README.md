# Streaming Availability Service

Go HTTP service between Stan's transcoder and CDN origins: ingests segment
availability events and serves ordered per-asset manifests.

Live: `https://<TODO>.onrender.com`

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

`Ctrl+C` (SIGINT) or `kill -TERM <pid>` triggers a graceful shutdown that drains in-flight requests for up to `SHUTDOWN_TIMEOUT`.

## Tests

```sh
go test ./...                                  # or: make test
go test -race ./...                            # or: make race
go test -run='^$' -bench=. -benchmem ./...     # or: make bench
```

## API

TODO: short examples for `POST /ingest`, `GET /manifest/{asset_id}`, `GET /health`.

## Design decision

TODO (5–10 sentences, required).

## Assumptions

- State is in memory only and is lost on restart or Render free-tier spin-down.
- TODO: duplicate ingest policy (update vs no-op).
- TODO: meaning of `last_updated`.
- TODO: validation beyond required fields.

## Scenario 7 — high-volume burst

- **Where it breaks first:** TODO
- **What I would change:** TODO
- **Observability before production:** TODO

## Known limitations / TODOs

- TODO
