package organization

import (
	"github.com/shivang-16/orbit.api/internal/model"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
)

type ListResponse struct {
	Organizations []model.Organization `json:"organizations"`
	Total         int                  `json:"total"`
	Entitlement   Entitlement          `json:"entitlement"`
}

type CreateResponse struct {
	Organization model.Organization `json:"organization"`
	Entitlement  Entitlement        `json:"entitlement"`
}

type UpdateResponse struct {
	Organization model.Organization `json:"organization"`
}

type ListMembersResponse struct {
	Members     []organizationRepository.Member `json:"members"`
	Total       int                             `json:"total"`
	Role        model.OrgRole                   `json:"role"`
	Entitlement MemberEntitlement               `json:"entitlement"`
}

type AddMemberResponse struct {
	Member      organizationRepository.Member `json:"member"`
	Entitlement MemberEntitlement             `json:"entitlement"`
}
