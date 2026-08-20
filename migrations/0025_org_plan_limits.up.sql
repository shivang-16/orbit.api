-- Attach the paying plan to the org that checked out, and store per-plan
-- organization / member caps. NULL on a cap means unlimited.
ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS plan_slug TEXT REFERENCES plans (slug);

CREATE INDEX IF NOT EXISTS organizations_created_by_idx
    ON organizations (created_by);

CREATE INDEX IF NOT EXISTS organizations_plan_slug_idx
    ON organizations (plan_slug);

ALTER TABLE plans
    ADD COLUMN IF NOT EXISTS max_organizations INTEGER,
    ADD COLUMN IF NOT EXISTS max_members_per_org INTEGER;

ALTER TABLE plans DROP CONSTRAINT IF EXISTS plans_org_limits_positive;
ALTER TABLE plans ADD CONSTRAINT plans_org_limits_positive CHECK (
    (max_organizations IS NULL OR max_organizations > 0)
    AND (max_members_per_org IS NULL OR max_members_per_org > 0)
);

UPDATE plans SET max_organizations = 2, max_members_per_org = 10 WHERE slug = 'trial';
UPDATE plans SET max_organizations = 5, max_members_per_org = 20 WHERE slug = 'starter';
UPDATE plans SET max_organizations = NULL, max_members_per_org = NULL
WHERE slug IN ('builder', 'pro', 'business');

-- Backfill from the latest plan grant so existing paid orgs keep their tier.
UPDATE organizations o
SET plan_slug = sub.slug
FROM (
    SELECT DISTINCT ON (organization_id)
        organization_id,
        substring(note FROM 6) AS slug
    FROM credit_ledger
    WHERE entry_type = 'grant' AND note LIKE 'plan:%'
    ORDER BY organization_id, created_at DESC
) sub
WHERE o.id = sub.organization_id
  AND EXISTS (SELECT 1 FROM plans p WHERE p.slug = sub.slug);

-- 0022 rewrites feature copy every boot; this migration runs after it.
UPDATE plans SET features = ARRAY[
    'One API for Claude, GPT, Kimi, and more',
    'Chat completions at Bedrock list rates',
    'Usage logged per request',
    'Web dashboard and API keys',
    '2 organizations',
    '10 members per organization'
] WHERE slug = 'trial';

UPDATE plans SET features = ARRAY[
    'Community support',
    '5 organizations',
    '20 members per organization'
] WHERE slug = 'starter';

UPDATE plans SET features = ARRAY[
    'Multiple API keys',
    'Organization workspace',
    'Email support',
    'Unlimited organizations',
    'Unlimited members'
] WHERE slug = 'builder';
