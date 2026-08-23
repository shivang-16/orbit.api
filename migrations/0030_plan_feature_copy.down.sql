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

UPDATE plans SET features = ARRAY[
    'Higher monthly volume',
    'Priority email support',
    'Usage history for the org',
    'Best for production apps'
] WHERE slug = 'pro';

UPDATE plans SET features = ARRAY[
    'Highest plan bonus (45%)',
    'Priority support',
    'Onboarding help',
    'Invoice-friendly for finance'
] WHERE slug = 'business';
