# Realtime Notify

A Go backend for real-time notifications, live dashboards, and
presence/chat features: WebSockets with an SSE fallback, fanned out
correctly across multiple server instances via Redis Pub/Sub, with
reconnect-safe message replay.

Built to demonstrate the patterns I use on client real-time infra
projects — see [`docs/architecture.md`](docs/architecture.md) for the
reasoning behind each one.

## Problem

Most SaaS products eventually need "notify the user when X happens" —
live notifications, chat, presence, collaborative editing, monitoring
dashboards. The naive version (a map of open sockets in memory) works
on localhost and breaks the moment you run more than one server
instance, because a message published on instance B never reaches a
client connected to instance A. This service solves that correctly,
so client engagements start from a working foundation instead of
rebuilding this from scratch under deadline pressure.

## What's built

- **WebSocket transport** — JWT-authenticated connections, one writer
  goroutine per connection (required for WS safety), heartbeat/liveness
  checks
- **SSE fallback** — for environments that block WebSocket upgrades
  (corporate proxies, some mobile carriers)
- **Cross-instance fan-out** — Redis Pub/Sub bridges instances so a
  message published anywhere reaches every subscriber everywhere;
  proven with a 2-instance + nginx Docker Compose setup, not just
  asserted
- **Reconnect-safe replay** — a capped Redis Stream per channel lets a
  client that drops for a few seconds catch up on what it missed,
  instead of silently losing messages
- **Subscribe-time channel authorization** — `user:*`, `org:*`, and
  `broadcast:*` namespaces, checked on every subscribe call, not just
  at connect time
- **Server-to-server publish API** — your existing backend pushes
  events in via `POST /api/v1/publish` with a shared key; no
  client-to-client messaging, keeping the trust boundary simple
- **Non-blocking backpressure** — a slow/dead client gets dropped
  rather than stalling delivery for everyone else on a channel
- **Observability** — structured JSON logs, `/healthz` + `/readyz`, a
  dependency-free `/metrics` endpoint in Prometheus text format backed
  by the hub's own delivery/drop/connection counters
- **Test suite** — unit tests for hub delivery semantics, client
  backpressure and heartbeat liveness, JWT verification, channel
  authorization, the in-memory pubsub driver, and config validation;
  runs in CI against a real Redis on every push
- **Load test tool** (`cmd/loadtest`) — measures real connection setup
  time and p50/p90/p99 delivery latency instead of asserting numbers
- **Browser demo client** (`demo/index.html`) — zero-build way to
  visually prove fan-out works, good for a portfolio screen recording

## Stack

Go 1.22 · chi router · nhooyr.io/websocket · Redis (Pub/Sub + Streams)
· Docker · nginx (for the multi-instance demo)

Deliberately minimal-dependency, same philosophy as the SaaS starter
kit this pairs with: no ORM needed here since there's no persistent
data model beyond Redis, no logging framework (`log/slog`), no metrics
SDK.

## Running it locally (single instance)

```bash
cp .env.example .env
go run ./cmd/api
```

Requires a local Redis (`redis-server` or `docker run -p 6379:6379 redis:7-alpine`).
For zero-dependency local hacking, set `PUBSUB_DRIVER=memory` — no Redis
needed, but this is single-instance only and has no reconnect replay
(disallowed in production; see `internal/config`).

## Trying it out

```bash
# 1. mint a test token (no auth service required for local testing)
go run ./cmd/mint-token -user=u1 -org=o1

# 2. run the API (in another terminal)
go run ./cmd/api

# 3. open the browser demo
open demo/index.html   # or just open the file directly in a browser
```

Paste the token in, connect, subscribe to `broadcast:demo`, then hit
"Publish event" — the message round-trips through the server and back
to the same tab. Open the file in a second tab to see true fan-out
between two independent connections.

## Tests

```bash
make test          # go test ./... -race
make test-cover    # with a coverage report
```

Covers: hub registration/subscribe/unsubscribe/delivery semantics and
concurrency safety, client backpressure (a full outbox drops rather
than blocks) and heartbeat liveness, JWT verification (valid/expired/
wrong-secret/missing-claim), channel authorization per namespace, the
in-memory PubSub driver, and config validation (production guardrails
for secrets, origins, and the pubsub driver). Runs in CI on every push
(`.github/workflows/ci.yml`), against a real Redis service container.

## Load testing

```bash
make loadtest CONNS=1000 MESSAGES=50 TOKEN=$(go run ./cmd/mint-token)
```

`cmd/loadtest` opens N concurrent WebSocket connections, subscribes
them all to one channel, publishes M events through the public API,
and reports connection setup time plus p50/p90/p99 delivery latency.
Copy the output into `docs/load-test-results.md` — real measured
numbers, not asserted ones, are what make this credible to a client.

## Proving cross-instance fan-out (the actual point of this repo)

```bash
docker compose up --build
```

This starts Redis, two API instances, and nginx load-balancing between
them on `localhost:8080`. Open two WebSocket connections through nginx
(they'll likely land on different instances) and publish an event —
both receive it regardless of which instance they're connected to.

## Project layout

```
cmd/api/               entrypoint — wires config, Redis, hub, routes, graceful shutdown
cmd/loadtest/            connection + delivery-latency load test tool
cmd/mint-token/          local JWT minting for testing, without a real auth service
demo/                   zero-build browser client for manually proving fan-out
internal/
  auth/                  JWT verification (issuance lives in your auth service; IssueDevToken is test-only)
  hub/                    per-instance connection registry + client lifecycle (unit tested)
  pubsub/                 Redis Pub/Sub + in-memory driver + replay buffer, behind a shared interface
  channel/                subscribe-time authorization (user/org/broadcast namespaces)
  transport/               WebSocket handler, SSE handler, publish endpoint
  middleware/               auth + publish-key guards
  metrics/                 Prometheus-format /metrics, backed by the hub's own delivery counters
  config/                 env-driven configuration with production guardrails
docs/                    architecture notes, OpenAPI spec, load test results template
.github/workflows/ci.yml  build, vet, race-tested unit tests, Docker build
docker-compose.yml        2 API instances + Redis + nginx, for the fan-out demo
```

## Deploying

The `Dockerfile` builds a static binary into a distroless image. Point
`REDIS_URL` at a managed Redis (Upstash, ElastiCache, Redis Cloud) and
run behind any load balancer that supports WebSocket upgrades and long
read timeouts — see `nginx.conf` for the config that matters
(`proxy_read_timeout`, `Upgrade`/`Connection` headers).

## About

Built as a companion piece to a multi-tenant SaaS backend starter kit —
together they cover the two things almost every SaaS MVP needs and
usually gets wrong under deadline: accounts/billing/permissions, and
"tell the user something happened" in real time.
