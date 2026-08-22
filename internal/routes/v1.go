package routes

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

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
	// Clerk-cookie routes: the browser sends the session cookie automatically,
	// so an open origin allowlist would let any site ride a logged-in user's
	// session (CSRF). Kept to a small, explicit allowlist with credentials on.
	dashboardCORS := cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Organization-Id"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})

	// Api-key routes (native chat + OpenAI/Anthropic compat): these
	// authenticate with an "Authorization: Bearer sk-orbit-..." or
	// "X-Api-Key" header that the caller sets explicitly, never an
	// ambient browser credential, so there's nothing here for a strict
	// per-origin allowlist to protect — it would only block the exact
	// thing this surface exists for: any of Orbit's customers calling it
	// straight from their own website with the official OpenAI/Anthropic
	// SDKs. Origin is wildcarded and credentials are off (required by the
	// CORS spec whenever Allow-Origin is "*").
	apiKeyCORS := cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	})

	// Every ordinary route gets a tight 30s timeout. The inference chat
	// route below is deliberately excluded and gets its own, much longer
	// one, since a streamed completion can legitimately take minutes.
	r.Group(func(r chi.Router) {
		r.Use(dashboardCORS)
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
		r.Use(dashboardCORS)
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
		r.Use(apiKeyCORS)
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
