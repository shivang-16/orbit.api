CREATE TABLE IF NOT EXISTS model_catalogue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    provider TEXT NOT NULL,
    model_id TEXT NOT NULL UNIQUE,
    input_context_limit INTEGER NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS model_catalogue_provider_idx
    ON model_catalogue (provider);

CREATE INDEX IF NOT EXISTS model_catalogue_is_active_idx
    ON model_catalogue (is_active);

DROP TRIGGER IF EXISTS model_catalogue_set_updated_at ON model_catalogue;
CREATE TRIGGER model_catalogue_set_updated_at
BEFORE UPDATE ON model_catalogue
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
