package main

import (
	"net/http"
	"time"
)

func (a *api) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "ts": time.Now().UTC().Format(time.RFC3339)})
}
