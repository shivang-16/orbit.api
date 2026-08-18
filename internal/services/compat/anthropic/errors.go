package anthropic

import (
	"encoding/json"
	"net/http"
)

// errorBody is Anthropic's error envelope shape: {"type": "error",
// "error": {"type", "message"}}.
type errorBody struct {
	Type  string      `json:"type"`
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// WriteError writes an Anthropic-shaped error response.
func WriteError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Type: "error", Error: errorDetail{Type: errType, Message: message}})
}
