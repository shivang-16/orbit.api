package organization

const (
	freeMaxOrganizations = 1
	freeMaxMembersPerOrg = 1
)

type Entitlement struct {
	PlanSlug               string `json:"plan_slug"`
	OrganizationCount      int    `json:"organization_count"`
	MaxOrganizations       *int   `json:"max_organizations"`
	MaxMembersPerOrg       *int   `json:"max_members_per_org"`
	UnlimitedOrganizations bool   `json:"unlimited_organizations"`
	UnlimitedMembers       bool   `json:"unlimited_members"`
	CanCreateOrganization  bool   `json:"can_create_organization"`
}

type MemberEntitlement struct {
	PlanSlug         string `json:"plan_slug"`
	MemberCount      int    `json:"member_count"`
	MaxMembersPerOrg *int   `json:"max_members_per_org"`
	UnlimitedMembers bool   `json:"unlimited_members"`
	CanAddMember     bool   `json:"can_add_member"`
}

func entitlementFor(planSlug string, orgCount int, maxOrgs, maxMembers *int) Entitlement {
	if planSlug == "" {
		orgs := freeMaxOrganizations
		members := freeMaxMembersPerOrg
		return Entitlement{
			OrganizationCount:     orgCount,
			MaxOrganizations:      &orgs,
			MaxMembersPerOrg:      &members,
			CanCreateOrganization: orgCount < orgs,
		}
	}

	e := Entitlement{
		PlanSlug:               planSlug,
		OrganizationCount:      orgCount,
		MaxOrganizations:       maxOrgs,
		MaxMembersPerOrg:       maxMembers,
		UnlimitedOrganizations: maxOrgs == nil,
		UnlimitedMembers:       maxMembers == nil,
	}
	e.CanCreateOrganization = e.UnlimitedOrganizations || (maxOrgs != nil && orgCount < *maxOrgs)
	return e
}

func (e Entitlement) memberEntitlement(memberCount int) MemberEntitlement {
	return MemberEntitlement{
		PlanSlug:         e.PlanSlug,
		MemberCount:      memberCount,
		MaxMembersPerOrg: e.MaxMembersPerOrg,
		UnlimitedMembers: e.UnlimitedMembers,
		CanAddMember:     e.canAddMember(memberCount),
	}
}

func (e Entitlement) canAddMember(memberCount int) bool {
	if e.UnlimitedMembers {
		return true
	}
	return e.MaxMembersPerOrg != nil && memberCount < *e.MaxMembersPerOrg
}
