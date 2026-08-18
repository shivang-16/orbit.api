ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS organizations_credits_non_negative;

ALTER TABLE organizations
    ADD CONSTRAINT organizations_credits_non_negative
    CHECK (
        credits_granted_micros >= 0
        AND credits_used_micros >= 0
        AND credits_remaining_micros >= 0
    );
