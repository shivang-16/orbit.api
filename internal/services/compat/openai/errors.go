package openai

import (
	"encoding/json"
	"net/http"
)

// errorBody is OpenAI's error envelope shape: {"error": {"message",
// "type", ...}}.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// WriteError writes an OpenAI-shaped error response.
func WriteError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: errorDetail{Message: message, Type: errType}})
}
