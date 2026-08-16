ALTER TABLE plans
    ADD COLUMN IF NOT EXISTS tagline TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS features TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS includes_from TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS highlighted BOOLEAN NOT NULL DEFAULT false;

UPDATE plans SET
    tagline = 'Try the API on a side project',
    includes_from = '',
    highlighted = false,
    features = ARRAY[
        'One API for Claude, GPT, Gemini, and more',
        'Chat completions at Bedrock list rates',
        '$30 inference credits included',
        'Usage logged per request',
        'Web dashboard and API keys',
        'Community support'
    ]
WHERE slug = 'starter';

UPDATE plans SET
    tagline = 'Ship one product',
    includes_from = 'Starter',
    highlighted = false,
    features = ARRAY[
        '$100 inference credits included',
        'Multiple API keys',
        'Organization workspace',
        'Email support',
        'Same vendor rates, no markup'
    ]
WHERE slug = 'builder';

UPDATE plans SET
    tagline = 'Production traffic, several keys',
    includes_from = 'Builder',
    highlighted = true,
    features = ARRAY[
        '$350 inference credits included',
        'Higher monthly volume',
        'Priority email support',
        'Usage history for the org',
        'Best for production apps'
    ]
WHERE slug = 'pro';

UPDATE plans SET
    tagline = 'Company-wide usage',
    includes_from = 'Pro',
    highlighted = false,
    features = ARRAY[
        '$910 inference credits included',
        'Highest plan bonus (45%)',
        'Priority support',
        'Onboarding help',
        'Invoice-friendly for finance'
    ]
WHERE slug = 'business';
