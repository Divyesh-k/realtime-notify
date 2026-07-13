# Load Test Notes (template)

Run a simple concurrent-connection test before handing this to a client
as proof it scales, rather than asserting it does. Suggested approach:

```bash
# Example using a small Go script (not included) or a tool like
# `hey`/`websocket-bench` adapted for WS handshakes:
#   1. Open N concurrent WebSocket connections split across api-a/api-b
#      behind nginx.
#   2. Subscribe every connection to a shared channel.
#   3. Publish M messages via POST /api/v1/publish.
#   4. Record: connection setup time, message delivery latency
#      (publish timestamp -> client receive timestamp), memory usage
#      per instance, and CPU.
```

Record real numbers here once measured -- this file is scaffolding, not
a claim. Suggested table to fill in:

| Concurrent connections | Instances | Msg/sec published | p50 delivery latency | p99 delivery latency | Memory per instance |
|---|---|---|---|---|---|
| 1,000 | 2 | 100 | | | |
| 10,000 | 2 | 500 | | | |
