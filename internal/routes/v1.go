package routes

import (
	"github.com/go-chi/chi/v5"

	catalogueController "github.com/shivang-16/orbit.api/internal/controller/catalogue"
	healthController "github.com/shivang-16/orbit.api/internal/controller/health"
	userController "github.com/shivang-16/orbit.api/internal/controller/user"
	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
)

func registerV1(
	r chi.Router,
	health *healthController.Controller,
	users *userController.Controller,
	catalogue *catalogueController.Controller,
) {
	r.Get("/health", health.Check)
	r.Get("/ready", health.Ready)

	r.Group(func(r chi.Router) {
		r.Use(authMiddleware.Clerk)
		r.Post("/users/sync", users.Sync)
		r.Get("/catalogue", catalogue.List)
		r.Get("/catalogue/overview", catalogue.Overview)
		r.Get("/catalogue/{id}", catalogue.Get)
	})
}
