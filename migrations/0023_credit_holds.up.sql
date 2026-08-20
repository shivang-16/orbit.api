-- Credit holds freeze a request's cost ceiling on remaining before Bedrock
-- is called. Remaining is reduced by the hold up front and restored (minus
-- actual usage) when the hold is settled or expired. Expired unsettled
-- holds are reclaimed so a crash cannot lock a wallet forever.
CREATE TABLE IF NOT EXISTS credit_holds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    amount_micros BIGINT NOT NULL CHECK (amount_micros > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    settled_at TIMESTAMPTZ,
    CONSTRAINT credit_holds_expires_after_create CHECK (expires_at > created_at)
);

CREATE UNIQUE INDEX IF NOT EXISTS credit_holds_open_id_idx
    ON credit_holds (id)
    WHERE settled_at IS NULL;

CREATE INDEX IF NOT EXISTS credit_holds_org_open_expires_idx
    ON credit_holds (organization_id, expires_at)
    WHERE settled_at IS NULL;
