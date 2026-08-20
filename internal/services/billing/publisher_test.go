package billing

import "testing"

func TestDecodeJobRequiresStableIdentity(t *testing.T) {
	t.Parallel()

	valid := `{"idempotency_key":"abc","organization_id":"org-1"}`
	job, err := DecodeJob(valid)
	if err != nil {
		t.Fatalf("DecodeJob: %v", err)
	}
	if job.IdempotencyKey != "abc" || job.OrganizationID != "org-1" {
		t.Fatalf("decoded job = %+v", job)
	}

	withHold := `{"idempotency_key":"abc","organization_id":"org-1","hold_id":"hold-1"}`
	job, err = DecodeJob(withHold)
	if err != nil {
		t.Fatalf("DecodeJob hold: %v", err)
	}
	if job.HoldID != "hold-1" {
		t.Fatalf("hold_id = %q", job.HoldID)
	}

	if _, err := DecodeJob(`{"organization_id":"org-1"}`); err == nil {
		t.Fatal("expected missing idempotency_key to fail")
	}
	if _, err := DecodeJob(`{"idempotency_key":"abc"}`); err == nil {
		t.Fatal("expected missing organization_id to fail")
	}
	if _, err := DecodeJob(`not-json`); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}
