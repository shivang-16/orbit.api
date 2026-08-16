package routes

import (
	"github.com/go-chi/chi/v5"

	apikeyController "github.com/shivang-16/orbit.api/internal/controller/apikey"
	catalogueController "github.com/shivang-16/orbit.api/internal/controller/catalogue"
	healthController "github.com/shivang-16/orbit.api/internal/controller/health"
	inferenceController "github.com/shivang-16/orbit.api/internal/controller/inference"
	organizationController "github.com/shivang-16/orbit.api/internal/controller/organization"
	userController "github.com/shivang-16/orbit.api/internal/controller/user"
	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
)

func registerV1(
	r chi.Router,
	health *healthController.Controller,
	users *userController.Controller,
	catalogue *catalogueController.Controller,
	apiKeys *apikeyController.Controller,
	orgs *organizationController.Controller,
	inference *inferenceController.Controller,
	apiKeyAuth *apikeyMiddleware.Middleware,
) {
	r.Get("/health", health.Check)
	r.Get("/ready", health.Ready)

	r.Group(func(r chi.Router) {
		r.Use(authMiddleware.Clerk)
		r.Post("/users/sync", users.Sync)
		r.Get("/catalogue", catalogue.List)
		r.Get("/catalogue/overview", catalogue.Overview)
		r.Get("/catalogue/{id}", catalogue.Get)
		r.Get("/api-keys", apiKeys.List)
		r.Post("/api-keys", apiKeys.Create)
		r.Get("/organizations", orgs.List)
		r.Post("/organizations", orgs.Create)
	})

	// Authenticated with an Orbit API key (sk-orbit-...) instead of a Clerk
	// session, since this is the surface external callers hit directly.
	r.Group(func(r chi.Router) {
		r.Use(apiKeyAuth.Authenticate)
		r.Post("/models/{id}/chat", inference.Chat)
	})
}
