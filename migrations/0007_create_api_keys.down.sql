DROP TRIGGER IF EXISTS api_keys_set_updated_at ON api_keys;
DROP INDEX IF EXISTS api_keys_organization_active_idx;
DROP INDEX IF EXISTS api_keys_organization_id_idx;
DROP TABLE IF EXISTS api_keys;
