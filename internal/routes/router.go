package routes

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/shivang-16/orbit.api/internal/config"
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
)

func New(
	cfg config.Config,
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
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Organization-Id"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", health.Check)
	r.Get("/ready", health.Ready)

	r.Route("/api/v1", func(r chi.Router) {
		registerV1(r, health, users, catalogue, apiKeys, orgs, inference, plans, checkout, credits, webhooks, apiKeyAuth)
	})

	return r
}
