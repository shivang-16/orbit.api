ALTER TABLE plans DROP CONSTRAINT IF EXISTS plans_slug_check;
ALTER TABLE plans ADD CONSTRAINT plans_slug_check
    CHECK (slug IN ('trial', 'starter', 'builder', 'pro', 'business'));

UPDATE plans SET sort_order = 2 WHERE slug = 'starter';
UPDATE plans SET sort_order = 3 WHERE slug = 'builder';
UPDATE plans SET sort_order = 4 WHERE slug = 'pro';
UPDATE plans SET sort_order = 5 WHERE slug = 'business';

INSERT INTO plans (
    slug,
    name,
    dodo_product_id,
    price_micros,
    credits_micros,
    tagline,
    features,
    includes_from,
    highlighted,
    sort_order
) VALUES (
    'trial',
    'Trial',
    'pdt_0NlkLEpnqBG9BKAMuRTRi',
    5000000,
    7000000,
    'See if Orbit fits',
    ARRAY[
        'One API for Claude, GPT, Kimi, and more',
        'Chat completions at Bedrock list rates',
        'Usage logged per request',
        'Web dashboard and API keys'
    ],
    '',
    false,
    1
)
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    price_micros = EXCLUDED.price_micros,
    credits_micros = EXCLUDED.credits_micros,
    tagline = EXCLUDED.tagline,
    features = EXCLUDED.features,
    includes_from = EXCLUDED.includes_from,
    highlighted = EXCLUDED.highlighted,
    sort_order = EXCLUDED.sort_order,
    is_active = true;

UPDATE plans SET includes_from = 'Trial' WHERE slug = 'starter';
