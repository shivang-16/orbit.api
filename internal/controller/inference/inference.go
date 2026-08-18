package inference

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	sharedController "github.com/shivang-16/orbit.api/internal/controller/shared"
	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

type Controller struct {
	service *inferenceService.Service
	billing billingService.Enqueuer
	orgs    *organizationRepository.Repository
}

func NewController(service *inferenceService.Service, billing billingService.Enqueuer, orgs *organizationRepository.Repository) *Controller {
	return &Controller{service: service, billing: billing, orgs: orgs}
}

func (c *Controller) Chat(w http.ResponseWriter, r *http.Request) {
	modelID := chi.URLParam(r, "id")

	var req inferenceService.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("inference/chat: invalid body model=%s: %v", modelID, err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result, err := c.service.Chat(r.Context(), modelID, req, w)
	if err != nil {
		orgID, _ := apikeyMiddleware.OrganizationID(r.Context())
		log.Printf("inference/chat failed model=%s org=%s: %v", modelID, orgID, err)
		switch {
		case errors.Is(err, inferenceService.ErrInvalid):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "messages are required"})
		case errors.Is(err, inferenceService.ErrModelNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		case errors.Is(err, inferenceService.ErrUnsupportedProvider):
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "model provider not supported yet"})
		case errors.Is(err, inferenceService.ErrLowCredits):
			writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "low on credits"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to reach model provider"})
		}
		return
	}

	// When result.Streamed is true, the service already wrote the status
	// line, headers, and every SSE chunk directly to w — nothing left to
	// write here. Otherwise this is the buffered ("stream": false) path.
	if !result.Streamed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(result.StatusCode)
		_, _ = w.Write(result.Body)
	}

	errBody := string(result.Body)
	if result.Streamed {
		errBody = "stream interrupted"
	}
	sharedController.RecordUsage(r.Context(), c.billing, "inference/chat", result, req.Prompt(), errBody)
}

// Playground is the dashboard chat surface: Clerk session + active org,
// always streamed, billed to the org with no API key. Same Bedrock path
// as Chat; only the auth story differs.
func (c *Controller) Playground(w http.ResponseWriter, r *http.Request) {
	modelID := chi.URLParam(r, "id")

	orgID, err := c.resolvePlaygroundOrg(r)
	if err != nil {
		log.Printf("inference/playground org: %v", err)
		switch {
		case errors.Is(err, errNoOrganization):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no organization"})
		case errors.Is(err, errForbiddenOrg):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve organization"})
		}
		return
	}

	var req inferenceService.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("inference/playground: invalid body model=%s: %v", modelID, err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	stream := true
	req.Stream = &stream

	ctx := apikeyMiddleware.WithOrganization(r.Context(), orgID)
	result, err := c.service.Chat(ctx, modelID, req, w)
	if err != nil {
		log.Printf("inference/playground failed model=%s org=%s: %v", modelID, orgID, err)
		switch {
		case errors.Is(err, inferenceService.ErrInvalid):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "messages are required"})
		case errors.Is(err, inferenceService.ErrModelNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		case errors.Is(err, inferenceService.ErrUnsupportedProvider):
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "model provider not supported yet"})
		case errors.Is(err, inferenceService.ErrLowCredits):
			writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "low on credits"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to reach model provider"})
		}
		return
	}

	if !result.Streamed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(result.StatusCode)
		_, _ = w.Write(result.Body)
	}

	errBody := string(result.Body)
	if result.Streamed {
		errBody = "stream interrupted"
	}
	sharedController.RecordUsage(ctx, c.billing, "inference/playground", result, req.Prompt(), errBody)
}

var (
	errNoOrganization = errors.New("no organization")
	errForbiddenOrg   = errors.New("forbidden organization")
)

func (c *Controller) resolvePlaygroundOrg(r *http.Request) (string, error) {
	userID, ok := authMiddleware.UserID(r.Context())
	if !ok {
		return "", errors.New("missing user id")
	}

	orgID := strings.TrimSpace(r.Header.Get("X-Organization-Id"))
	if orgID != "" {
		if c.orgs == nil {
			return orgID, nil
		}
		member, err := c.orgs.IsMember(r.Context(), userID, orgID)
		if err != nil {
			return "", err
		}
		if !member {
			return "", errForbiddenOrg
		}
		return orgID, nil
	}

	if c.orgs == nil {
		return "", errNoOrganization
	}
	org, err := c.orgs.GetFirstForUser(r.Context(), userID)
	if err != nil {
		return "", err
	}
	if org == nil {
		return "", errNoOrganization
	}
	return org.ID, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
