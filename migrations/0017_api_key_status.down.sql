DROP INDEX IF EXISTS api_keys_organization_active_status_idx;

ALTER TABLE api_keys
    DROP CONSTRAINT IF EXISTS api_keys_status_check;

ALTER TABLE api_keys
    DROP COLUMN IF EXISTS status;
