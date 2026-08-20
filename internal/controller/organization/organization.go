package organization

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	organizationService "github.com/shivang-16/orbit.api/internal/services/organization"
)

type Controller struct {
	service *organizationService.Service
}

func NewController(service *organizationService.Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) List(w http.ResponseWriter, r *http.Request) {
	resp, err := c.service.List(r.Context())
	if err != nil {
		log.Printf("organizations/list: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list organizations"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) Create(w http.ResponseWriter, r *http.Request) {
	var req organizationService.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	resp, err := c.service.Create(r.Context(), req)
	if err != nil {
		log.Printf("organizations/create: %v", err)
		writeServiceError(w, err, "failed to create organization")
		return
	}
	log.Printf("organizations/create ok id=%s name=%s", resp.Organization.ID, resp.Organization.Name)
	writeJSON(w, http.StatusCreated, resp)
}

func (c *Controller) Update(w http.ResponseWriter, r *http.Request) {
	var req organizationService.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.OrganizationID == "" {
		req.OrganizationID = organizationID(r)
	}

	resp, err := c.service.Update(r.Context(), req)
	if err != nil {
		log.Printf("organizations/update: %v", err)
		writeServiceError(w, err, "failed to update organization")
		return
	}
	log.Printf("organizations/update ok id=%s name=%s", resp.Organization.ID, resp.Organization.Name)
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) ListMembers(w http.ResponseWriter, r *http.Request) {
	resp, err := c.service.ListMembers(r.Context(), organizationID(r))
	if err != nil {
		log.Printf("organizations/members/list: %v", err)
		writeServiceError(w, err, "failed to list members")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) AddMember(w http.ResponseWriter, r *http.Request) {
	var req organizationService.AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.OrganizationID == "" {
		req.OrganizationID = organizationID(r)
	}

	resp, err := c.service.AddMember(r.Context(), req)
	if err != nil {
		log.Printf("organizations/members/add: %v", err)
		writeServiceError(w, err, "failed to add member")
		return
	}
	log.Printf("organizations/members/add ok org=%s user=%s", resp.Member.OrganizationID, resp.Member.UserID)
	writeJSON(w, http.StatusCreated, resp)
}

func writeServiceError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, organizationService.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
	case errors.Is(err, organizationService.ErrNoOrganization):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no organization"})
	case errors.Is(err, organizationService.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
	case errors.Is(err, organizationService.ErrOrgLimit):
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "organization limit reached",
			"code":  "org_limit",
		})
	case errors.Is(err, organizationService.ErrMemberLimit):
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "member limit reached",
			"code":  "member_limit",
		})
	case errors.Is(err, organizationService.ErrUserNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "user not found",
			"code":  "user_not_found",
		})
	case errors.Is(err, organizationService.ErrAlreadyMember):
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "already a member",
			"code":  "already_member",
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fallback})
	}
}

func organizationID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Organization-Id"))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
