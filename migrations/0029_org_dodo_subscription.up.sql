ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS dodo_subscription_id TEXT NOT NULL DEFAULT '';
