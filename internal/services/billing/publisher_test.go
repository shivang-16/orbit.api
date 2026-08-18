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
