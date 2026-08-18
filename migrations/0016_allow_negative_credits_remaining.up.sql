-- Usage deduction is allowed to take an organization's remaining balance
-- negative for the single request that crosses the low-credits threshold
-- (enforced in application code before the request is made). Only granted
-- and used totals must stay non-negative.
ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS organizations_credits_non_negative;

ALTER TABLE organizations
    ADD CONSTRAINT organizations_credits_non_negative
    CHECK (
        credits_granted_micros >= 0
        AND credits_used_micros >= 0
    );
