DROP TABLE IF EXISTS credit_ledger;
DROP TABLE IF EXISTS inference_requests;

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS organizations_credits_non_negative;

ALTER TABLE organizations
    DROP COLUMN IF EXISTS credits_granted_micros,
    DROP COLUMN IF EXISTS credits_used_micros,
    DROP COLUMN IF EXISTS credits_remaining_micros;
