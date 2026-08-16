CREATE TABLE IF NOT EXISTS model_pricing (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_catalogue_id UUID NOT NULL UNIQUE REFERENCES model_catalogue (id) ON DELETE CASCADE,
    vendor_input_per_million_micros BIGINT NOT NULL,
    vendor_output_per_million_micros BIGINT NOT NULL,
    orbit_input_per_million_micros BIGINT NOT NULL,
    orbit_output_per_million_micros BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT model_pricing_amounts_positive CHECK (
        vendor_input_per_million_micros >= 0
        AND vendor_output_per_million_micros >= 0
        AND orbit_input_per_million_micros >= 0
        AND orbit_output_per_million_micros >= 0
    )
);

DROP TRIGGER IF EXISTS model_pricing_set_updated_at ON model_pricing;
CREATE TRIGGER model_pricing_set_updated_at
BEFORE UPDATE ON model_pricing
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
