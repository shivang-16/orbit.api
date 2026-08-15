package routes

import (
	"github.com/go-chi/chi/v5"

	healthController "github.com/shivang-16/orbit.api/internal/controller/health"
)

func registerV1(r chi.Router, health *healthController.Controller) {
	r.Get("/health", health.Check)
	r.Get("/ready", health.Ready)
}
