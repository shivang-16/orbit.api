ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS credits_granted_micros BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS credits_used_micros BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS credits_remaining_micros BIGINT NOT NULL DEFAULT 0;

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS organizations_credits_non_negative;
ALTER TABLE organizations
    ADD CONSTRAINT organizations_credits_non_negative
    CHECK (
        credits_granted_micros >= 0
        AND credits_used_micros >= 0
        AND credits_remaining_micros >= 0
    );

CREATE TABLE IF NOT EXISTS inference_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    api_key_id UUID REFERENCES api_keys (id) ON DELETE SET NULL,
    model_catalogue_id UUID REFERENCES model_catalogue (id) ON DELETE SET NULL,
    prompt TEXT NOT NULL DEFAULT '',
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT inference_requests_status_check CHECK (status IN ('success', 'error'))
);

CREATE INDEX IF NOT EXISTS inference_requests_organization_id_created_at_idx
    ON inference_requests (organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS inference_requests_api_key_id_idx
    ON inference_requests (api_key_id);

CREATE TABLE IF NOT EXISTS credit_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    inference_request_id UUID REFERENCES inference_requests (id) ON DELETE SET NULL,
    entry_type TEXT NOT NULL,
    amount_micros BIGINT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT credit_ledger_entry_type_check
        CHECK (entry_type IN ('grant', 'usage', 'refund', 'adjustment')),
    CONSTRAINT credit_ledger_amount_positive CHECK (amount_micros > 0)
);

CREATE INDEX IF NOT EXISTS credit_ledger_organization_id_created_at_idx
    ON credit_ledger (organization_id, created_at DESC);
