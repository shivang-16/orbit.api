ALTER TABLE credit_ledger
    ADD COLUMN IF NOT EXISTS vendor_amount_micros BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS orbit_amount_micros BIGINT NOT NULL DEFAULT 0;

UPDATE credit_ledger
SET orbit_amount_micros = amount_micros
WHERE entry_type = 'usage'
  AND orbit_amount_micros = 0;

ALTER TABLE credit_ledger
    DROP CONSTRAINT IF EXISTS credit_ledger_pricing_non_negative;
ALTER TABLE credit_ledger
    ADD CONSTRAINT credit_ledger_pricing_non_negative
    CHECK (vendor_amount_micros >= 0 AND orbit_amount_micros >= 0);
