ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';

ALTER TABLE api_keys
    DROP CONSTRAINT IF EXISTS api_keys_status_check;
ALTER TABLE api_keys
    ADD CONSTRAINT api_keys_status_check
    CHECK (status IN ('active', 'inactive'));

UPDATE api_keys
SET status = 'inactive'
WHERE revoked_at IS NOT NULL
  AND status <> 'inactive';

CREATE INDEX IF NOT EXISTS api_keys_organization_active_status_idx
    ON api_keys (organization_id)
    WHERE status = 'active';
