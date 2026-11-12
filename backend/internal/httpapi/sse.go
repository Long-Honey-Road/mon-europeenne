package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/auth"
)

const defaultHeartbeat = 15 * time.Second

// streamThread relays live turn events of the caller's thread as SSE. Clients open it before sending a message.
func streamThread(d Deps) http.Handler {
	heartbeat := d.Heartbeat
	if heartbeat <= 0 {
		heartbeat = defaultHeartbeat
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			writeStatus(w, http.StatusUnauthorized, codes.Unauthenticated, "authentication required")
			return
		}
		thread, err := d.Threads.Thread(r.Context(), p.UserID, r.PathValue("id"))
		if err != nil {
			writeStatus(w, http.StatusNotFound, codes.NotFound, "not found")
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeStatus(w, http.StatusInternalServerError, codes.Internal, "streaming unsupported")
			return
		}
		events, stop, err := d.Bus.Subscribe(r.Context(), thread.ID)
		if err != nil {
			writeStatus(w, http.StatusServiceUnavailable, codes.Unavailable, "event stream unavailable")
			return
		}
		defer stop()

		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		tick := time.NewTicker(heartbeat)
		defer tick.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-tick.C:
				fmt.Fprint(w, ": ping\n\n")
				flusher.Flush()
			case e, ok := <-events:
				if !ok {
					return
				}
				data, err := json.Marshal(e.Data)
				if err != nil {
					slog.Error("encode stream event", "type", e.Type, "err", err)
					continue
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
				flusher.Flush()
			}
		}
	})
}
