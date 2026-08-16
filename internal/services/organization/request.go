package organization

import (
	"strings"
	"unicode/utf8"
)

type CreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (r *CreateRequest) isValid() bool {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	if r.Name == "" || utf8.RuneCountInString(r.Name) > 80 {
		return false
	}
	if utf8.RuneCountInString(r.Description) > 280 {
		return false
	}
	return true
}
