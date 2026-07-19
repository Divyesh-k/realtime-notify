# Architecture Notes

## Why fan-out needs Pub/Sub, not just in-memory broadcast

A single process holding WebSocket connections in a map works fine for a
demo and breaks the moment you run more than one instance. If user A is
connected to instance 1 and an event is published from a request that
landed on instance 2, A never receives it unless the two instances have
a shared way to talk to each other.

This service solves that with Redis Pub/Sub as the coordination layer:

```
Backend calls POST /api/v1/publish
        |
        v
   hub.Publish() --> Redis PUBLISH rtn:<channel>
        |
        +--> instance A (subscribed) --> local clients on channel
        +--> instance B (subscribed) --> local clients on channel
```

Every instance's `Hub` only ever tracks its own local connections. It
never asks "is this client on another server" -- it just subscribes to
the same Redis channel every other instance subscribes to, and Redis
handles delivery to all of them. This is what `docker-compose.yml`'s
two-instance setup exists to prove.

## Why publish always goes through Redis, even for local delivery

`Hub.Publish` never delivers directly to local clients as a fast path.
It always calls the same `pubsub.Publish`, which is also how remote
instances receive it. One code path for "local" and "remote" delivery
means there's no special case that can silently drift out of sync --
the behavior you see with one instance running locally is exactly the
behavior you get with ten instances in production.

## Backpressure: what happens to a slow client

`Client.Send` is non-blocking. If a client's outbound buffer (64
messages) is full, the message is dropped and the send returns `false`.
The hub does not block waiting for a slow reader, because one frozen
tab or bad connection would otherwise stall message delivery for every
other client on that channel. A client that's actually still alive but
briefly behind will catch up in one of two ways: it reconnects and asks
for the replay buffer, or the connection error surfaces to it and it
naturally reconnects.

## Reconnect and replay

Every published message is also appended to a Redis Stream capped at
500 entries / 5 minutes, keyed per channel. On reconnect, a client
sends the ID of the last message it saw; the server replays anything
published since. This is deliberately a short window sized for network
blips (phone locks, wifi drops, laptop sleeps) -- not a durable event
log for offline clients. A client gone for an hour should refetch state
through a normal REST call, not expect a stream replay to have kept
everything.

## Channel authorization happens at subscribe time, not connect time

A client authenticates once to open the socket, but authorization to
see a specific channel's data is checked on every `subscribe` frame
(`internal/channel/auth.go`). This matters because a single connection
can subscribe to multiple channels over its lifetime -- the JWT proves
who you are, but it doesn't imply you're allowed to see everything you
might ask to subscribe to.

## Server-to-server publish, not client-to-client

Clients can only *receive* messages -- there is no client-initiated
"publish to channel" path. Events are pushed in by your backend calling
`POST /api/v1/publish` with a shared `X-Publish-Key`. This keeps the
trust boundary simple: anything a connected client sees was authored by
a trusted service, not by another end user.
