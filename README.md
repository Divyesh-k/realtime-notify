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
  dependency-free `/metrics` endpoint in Prometheus text format

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
internal/
  auth/                 JWT verification (issuance lives in your auth service)
  hub/                   per-instance connection registry + client lifecycle
  pubsub/                 Redis Pub/Sub + replay buffer, behind a swappable interface
  channel/               subscribe-time authorization (user/org/broadcast namespaces)
  transport/              WebSocket handler, SSE handler, publish endpoint
  middleware/              auth + publish-key guards
  metrics/                Prometheus-format /metrics
  config/                env-driven configuration with production guardrails
docs/                   architecture notes, OpenAPI spec, load test template
docker-compose.yml       2 API instances + Redis + nginx, for the fan-out demo
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
