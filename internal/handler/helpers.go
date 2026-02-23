package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// writeJSON encodes v as JSON and writes it to the response writer.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to encode response: %v"}`, err), http.StatusInternalServerError)
	}
}
