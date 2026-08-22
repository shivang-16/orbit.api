package routes

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/shivang-16/orbit.api/internal/config"
	apikeyController "github.com/shivang-16/orbit.api/internal/controller/apikey"
	catalogueController "github.com/shivang-16/orbit.api/internal/controller/catalogue"
	checkoutController "github.com/shivang-16/orbit.api/internal/controller/checkout"
	anthropicController "github.com/shivang-16/orbit.api/internal/controller/compat/anthropic"
	openaiController "github.com/shivang-16/orbit.api/internal/controller/compat/openai"
	creditsController "github.com/shivang-16/orbit.api/internal/controller/credits"
	healthController "github.com/shivang-16/orbit.api/internal/controller/health"
	inferenceController "github.com/shivang-16/orbit.api/internal/controller/inference"
	invoicesController "github.com/shivang-16/orbit.api/internal/controller/invoices"
	organizationController "github.com/shivang-16/orbit.api/internal/controller/organization"
	planController "github.com/shivang-16/orbit.api/internal/controller/plan"
	userController "github.com/shivang-16/orbit.api/internal/controller/user"
	webhookController "github.com/shivang-16/orbit.api/internal/controller/webhook"
	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
)

func registerV1(
	r chi.Router,
	cfg config.Config,
	health *healthController.Controller,
	users *userController.Controller,
	catalogue *catalogueController.Controller,
	apiKeys *apikeyController.Controller,
	orgs *organizationController.Controller,
	inference *inferenceController.Controller,
	openaiCompat *openaiController.Controller,
	anthropicCompat *anthropicController.Controller,
	plans *planController.Controller,
	checkout *checkoutController.Controller,
	credits *creditsController.Controller,
	invoices *invoicesController.Controller,
	webhooks *webhookController.Controller,
	apiKeyAuth *apikeyMiddleware.Middleware,
) {
	// Every ordinary route gets a tight 30s timeout. The inference chat
	// route below is deliberately excluded and gets its own, much longer
	// one, since a streamed completion can legitimately take minutes.
	// CORS is applied on the root router (corsByPath) so OPTIONS
	// preflights are not 405'd by Chi's method matcher.
	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(time.Duration(cfg.Server.DashboardTimeoutSeconds) * time.Second))

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
			r.Patch("/organizations", orgs.Update)
			r.Get("/organizations/members", orgs.ListMembers)
			r.Post("/organizations/members", orgs.AddMember)
			r.Post("/billing/checkout", checkout.Create)
			r.Get("/billing/credits", credits.GetOrganizationCredits)
			r.Get("/billing/credits/history", credits.ListOrganizationCreditHistory)
			r.Get("/billing/invoices", invoices.List)
			r.Get("/billing/invoices/{paymentId}/pdf", invoices.PDF)
			r.Get("/usage", credits.GetUsage)
		})
	})

	// Dashboard playground: Clerk session instead of an API key, billed
	// to the active organization. Same 5-minute timeout as API-key chat
	// so a streamed completion is not cut off by the 30s dashboard budget.
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware.Clerk)
		r.Use(middleware.Timeout(time.Duration(cfg.Server.InferenceTimeoutSeconds) * time.Second))
		r.Post("/playground/models/{id}/chat", inference.Playground)
	})

	// Authenticated with an Orbit API key (sk-orbit-...) instead of a Clerk
	// session, since this is the surface external callers hit directly.
	// This same group carries the native chat route plus the
	// OpenAI/Anthropic-compatible routes, so official SDKs authenticate
	// exactly like a direct Orbit call — only base_url and api_key change.
	r.Group(func(r chi.Router) {
		r.Use(apiKeyAuth.Authenticate)
		r.Use(middleware.Timeout(time.Duration(cfg.Server.InferenceTimeoutSeconds) * time.Second))
		r.Post("/models/{id}/chat", inference.Chat)

		// OpenAI SDK: base_url = ".../api/v1" (its client appends
		// "/chat/completions" and "/models" itself).
		r.Post("/chat/completions", openaiCompat.ChatCompletions)
		r.Get("/models", openaiCompat.ListModels)

		// Anthropic SDK: base_url = ".../api" (its client appends
		// "/v1/messages" itself), so this still lives under the
		// existing "/api/v1" mount.
		r.Post("/messages", anthropicCompat.Messages)
	})
}
