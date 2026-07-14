// Package metrics exposes a dependency-free /metrics endpoint in
// Prometheus text format — consistent with the SaaS starter kit's
// philosophy: a handful of counters and gauges doesn't need a full SDK.
package metrics

import (
	"fmt"
	"net/http"

	"github.com/yourname/realtime-notify/internal/hub"
)

type Metrics struct {
	hub *hub.Hub
}

func New(h *hub.Hub) *Metrics {
	return &Metrics{hub: h}
}

func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		fmt.Fprintf(w, "# HELP rtn_connections_active Currently connected clients on this instance\n")
		fmt.Fprintf(w, "# TYPE rtn_connections_active gauge\n")
		fmt.Fprintf(w, "rtn_connections_active %d\n", m.hub.LocalConnectionCount())

		fmt.Fprintf(w, "# HELP rtn_channels_active Channels with at least one local subscriber\n")
		fmt.Fprintf(w, "# TYPE rtn_channels_active gauge\n")
		fmt.Fprintf(w, "rtn_channels_active %d\n", m.hub.LocalChannelCount())

		fmt.Fprintf(w, "# HELP rtn_connections_total Total connections accepted since start on this instance\n")
		fmt.Fprintf(w, "# TYPE rtn_connections_total counter\n")
		fmt.Fprintf(w, "rtn_connections_total %d\n", m.hub.ConnectionsTotal())

		fmt.Fprintf(w, "# HELP rtn_messages_delivered_total Messages successfully enqueued to a local client\n")
		fmt.Fprintf(w, "# TYPE rtn_messages_delivered_total counter\n")
		fmt.Fprintf(w, "rtn_messages_delivered_total %d\n", m.hub.DeliveredTotal())

		fmt.Fprintf(w, "# HELP rtn_messages_dropped_total Messages dropped because a client's buffer was full\n")
		fmt.Fprintf(w, "# TYPE rtn_messages_dropped_total counter\n")
		fmt.Fprintf(w, "rtn_messages_dropped_total %d\n", m.hub.DroppedTotal())
	}
}
