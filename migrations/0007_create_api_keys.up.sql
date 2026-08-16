CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    created_by TEXT NOT NULL REFERENCES users (id),
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    key_preview TEXT NOT NULL,
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS api_keys_organization_id_idx
    ON api_keys (organization_id);

CREATE INDEX IF NOT EXISTS api_keys_organization_active_idx
    ON api_keys (organization_id)
    WHERE revoked_at IS NULL;

DROP TRIGGER IF EXISTS api_keys_set_updated_at ON api_keys;
CREATE TRIGGER api_keys_set_updated_at
BEFORE UPDATE ON api_keys
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
