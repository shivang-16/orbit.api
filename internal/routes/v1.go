package routes

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	apikeyController "github.com/shivang-16/orbit.api/internal/controller/apikey"
	catalogueController "github.com/shivang-16/orbit.api/internal/controller/catalogue"
	checkoutController "github.com/shivang-16/orbit.api/internal/controller/checkout"
	creditsController "github.com/shivang-16/orbit.api/internal/controller/credits"
	healthController "github.com/shivang-16/orbit.api/internal/controller/health"
	inferenceController "github.com/shivang-16/orbit.api/internal/controller/inference"
	organizationController "github.com/shivang-16/orbit.api/internal/controller/organization"
	planController "github.com/shivang-16/orbit.api/internal/controller/plan"
	userController "github.com/shivang-16/orbit.api/internal/controller/user"
	webhookController "github.com/shivang-16/orbit.api/internal/controller/webhook"
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
	plans *planController.Controller,
	checkout *checkoutController.Controller,
	credits *creditsController.Controller,
	webhooks *webhookController.Controller,
	apiKeyAuth *apikeyMiddleware.Middleware,
) {
	// Every ordinary route gets a tight 30s timeout. The inference chat
	// route below is deliberately excluded and gets its own, much longer
	// one, since a streamed completion can legitimately take minutes.
	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(30 * time.Second))

		r.Get("/health", health.Check)
		r.Get("/ready", health.Ready)
		r.Get("/plans", plans.List)
		r.Post("/webhooks/dodo", webhooks.Dodo)

		r.Group(func(r chi.Router) {
			r.Use(authMiddleware.Clerk)
			r.Post("/users/sync", users.Sync)
			r.Get("/catalogue", catalogue.List)
			r.Get("/catalogue/overview", catalogue.Overview)
			r.Get("/catalogue/{id}", catalogue.Get)
			r.Get("/api-keys", apiKeys.List)
			r.Post("/api-keys", apiKeys.Create)
			r.Delete("/api-keys/{id}", apiKeys.Delete)
			r.Get("/organizations", orgs.List)
			r.Post("/organizations", orgs.Create)
			r.Post("/billing/checkout", checkout.Create)
			r.Get("/billing/credits", credits.GetOrganizationCredits)
			r.Get("/billing/credits/history", credits.ListOrganizationCreditHistory)
			r.Get("/usage", credits.GetUsage)
		})
	})

	// Authenticated with an Orbit API key (sk-orbit-...) instead of a Clerk
	// session, since this is the surface external callers hit directly.
	r.Group(func(r chi.Router) {
		r.Use(apiKeyAuth.Authenticate)
		r.Use(middleware.Timeout(5 * time.Minute))
		r.Post("/models/{id}/chat", inference.Chat)
	})
}
