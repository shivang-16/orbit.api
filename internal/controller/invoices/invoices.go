package invoices

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	invoicesService "github.com/shivang-16/orbit.api/internal/services/invoices"
)

type Controller struct {
	service *invoicesService.Service
}

func NewController(service *invoicesService.Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resp, err := c.service.List(r.Context(), organizationID(r), q.Get("page"), q.Get("limit"))
	if err != nil {
		log.Printf("invoices/list org=%s: %v", organizationID(r), err)
		writeServiceError(w, err, "failed to load invoices")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) PDF(w http.ResponseWriter, r *http.Request) {
	paymentID := chi.URLParam(r, "paymentId")
	body, contentType, err := c.service.InvoicePDF(r.Context(), organizationID(r), paymentID)
	if err != nil {
		log.Printf("invoices/pdf org=%s payment=%s: %v", organizationID(r), paymentID, err)
		writeServiceError(w, err, "failed to download invoice")
		return
	}
	filename := fmt.Sprintf("orbit-invoice-%s.pdf", strings.TrimSpace(paymentID))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func writeServiceError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, invoicesService.ErrNoOrganization):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no organization"})
	case errors.Is(err, invoicesService.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
	case errors.Is(err, invoicesService.ErrInvalidPage):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid page"})
	case errors.Is(err, invoicesService.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invoice not found"})
	default:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fallback})
	}
}

func organizationID(r *http.Request) string {
	if value := strings.TrimSpace(r.URL.Query().Get("organization_id")); value != "" {
		return value
	}
	return strings.TrimSpace(r.Header.Get("X-Organization-Id"))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
