package organization

import (
	"testing"
)

func TestEntitlementFreeCannotCreateSecondOrg(t *testing.T) {
	t.Parallel()

	got := entitlementFor("", 1, nil, nil)
	if got.CanCreateOrganization {
		t.Fatal("free plan should not create a second org")
	}
	if got.UnlimitedOrganizations || got.UnlimitedMembers {
		t.Fatal("free plan is not unlimited")
	}
	if got.MaxOrganizations == nil || *got.MaxOrganizations != 1 {
		t.Fatalf("free max orgs = %v", got.MaxOrganizations)
	}
	if got.MaxMembersPerOrg == nil || *got.MaxMembersPerOrg != 1 {
		t.Fatalf("free max members = %v", got.MaxMembersPerOrg)
	}
	if got.canAddMember(1) {
		t.Fatal("free org already has the owner; cannot add members")
	}
}

func TestEntitlementTrialAllowsSecondOrg(t *testing.T) {
	t.Parallel()

	maxOrgs, maxMembers := 2, 10
	got := entitlementFor("trial", 1, &maxOrgs, &maxMembers)
	if !got.CanCreateOrganization {
		t.Fatal("trial with 1 org should allow a second")
	}
	if got.canAddMember(10) {
		t.Fatal("trial at 10 members is full")
	}
	if !got.canAddMember(9) {
		t.Fatal("trial with 9 members should allow one more")
	}

	full := entitlementFor("trial", 2, &maxOrgs, &maxMembers)
	if full.CanCreateOrganization {
		t.Fatal("trial at 2 orgs should be full")
	}
}

func TestEntitlementStarterCaps(t *testing.T) {
	t.Parallel()

	maxOrgs, maxMembers := 5, 20
	got := entitlementFor("starter", 5, &maxOrgs, &maxMembers)
	if got.CanCreateOrganization {
		t.Fatal("starter at 5 orgs should be full")
	}
	if !entitlementFor("starter", 4, &maxOrgs, &maxMembers).CanCreateOrganization {
		t.Fatal("starter with 4 orgs should allow one more")
	}
	if got.canAddMember(20) {
		t.Fatal("starter at 20 members is full")
	}
}

func TestEntitlementHigherPlansUnlimited(t *testing.T) {
	t.Parallel()

	got := entitlementFor("builder", 40, nil, nil)
	if !got.UnlimitedOrganizations || !got.UnlimitedMembers {
		t.Fatal("builder should be unlimited")
	}
	if !got.CanCreateOrganization {
		t.Fatal("unlimited orgs should always allow create")
	}
	if !got.canAddMember(10_000) {
		t.Fatal("unlimited members should always allow add")
	}
}
