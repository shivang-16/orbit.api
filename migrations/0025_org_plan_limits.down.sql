UPDATE plans SET features = ARRAY[
    'One API for Claude, GPT, Kimi, and more',
    'Chat completions at Bedrock list rates',
    'Usage logged per request',
    'Web dashboard and API keys'
] WHERE slug = 'trial';

UPDATE plans SET features = ARRAY[
    'Community support'
] WHERE slug = 'starter';

UPDATE plans SET features = ARRAY[
    'Multiple API keys',
    'Organization workspace',
    'Email support'
] WHERE slug = 'builder';

ALTER TABLE organizations DROP COLUMN IF EXISTS plan_slug;

ALTER TABLE plans DROP CONSTRAINT IF EXISTS plans_org_limits_positive;
ALTER TABLE plans DROP COLUMN IF EXISTS max_organizations;
ALTER TABLE plans DROP COLUMN IF EXISTS max_members_per_org;
