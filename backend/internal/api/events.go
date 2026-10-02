package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// EventStream streams real-time device events to the UI as Server-Sent Events.
// Authentication uses the usual bearer token, or ?token= since EventSource
// cannot set headers. Optional ?projectId= filters to one project.
func (h *Handlers) EventStream(c *gin.Context) {
	if h.Hub == nil {
		fail(c, http.StatusServiceUnavailable, "event stream unavailable")
		return
	}
	projectID := queryUint(c, "projectId")

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	id, ch := h.Hub.Subscribe()
	defer h.Hub.Unsubscribe(id)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		fail(c, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	fmt.Fprint(c.Writer, ": connected\n\n")
	flusher.Flush()

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case e, open := <-ch:
			if !open {
				return
			}
			if projectID != 0 && e.ProjectID != projectID {
				continue
			}
			b, err := json.Marshal(e)
			if err != nil {
				continue
			}
			fmt.Fprintf(c.Writer, "data: %s\n\n", b)
			flusher.Flush()
		case <-keepAlive.C:
			fmt.Fprint(c.Writer, ": ping\n\n")
			flusher.Flush()
		}
	}
}
