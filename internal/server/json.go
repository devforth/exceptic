package server

import (
	"encoding/json"
	"net/http"
)

// WriteJSON sends body as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	// Responses carry error data: no cache may keep them.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) // the client may be gone; nothing useful to do then
}

// JSONError sends {"error": message}, the error shape of the REST API.
func JSONError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}
