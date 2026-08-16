ALTER TABLE credit_ledger
    DROP CONSTRAINT IF EXISTS credit_ledger_pricing_non_negative;

ALTER TABLE credit_ledger
    DROP COLUMN IF EXISTS vendor_amount_micros,
    DROP COLUMN IF EXISTS orbit_amount_micros;
