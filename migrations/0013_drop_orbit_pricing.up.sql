ALTER TABLE model_pricing
    DROP CONSTRAINT IF EXISTS model_pricing_amounts_positive;

ALTER TABLE model_pricing
    DROP COLUMN IF EXISTS orbit_input_per_million_micros,
    DROP COLUMN IF EXISTS orbit_output_per_million_micros;

ALTER TABLE model_pricing
    ADD CONSTRAINT model_pricing_amounts_positive CHECK (
        vendor_input_per_million_micros >= 0
        AND vendor_output_per_million_micros >= 0
    );

ALTER TABLE credit_ledger
    DROP CONSTRAINT IF EXISTS credit_ledger_pricing_non_negative;

ALTER TABLE credit_ledger
    DROP COLUMN IF EXISTS orbit_amount_micros;

ALTER TABLE credit_ledger
    ADD CONSTRAINT credit_ledger_pricing_non_negative
    CHECK (vendor_amount_micros >= 0);
