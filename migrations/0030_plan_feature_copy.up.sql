UPDATE plans SET features = ARRAY[
    'One API for Claude, GPT, Kimi, and 50+ models',
    '40% more credits than you pay',
    'Usage history',
    'Create up to 2 organizations',
    'Up to 10 members per organization'
] WHERE slug = 'trial';

UPDATE plans SET features = ARRAY[
    '50% more credits than you pay',
    'Standard rate limits',
    'Create up to 5 organizations',
    'Up to 20 members per organization'
] WHERE slug = 'starter';

UPDATE plans SET features = ARRAY[
    '67% more credits than you pay',
    'Higher rate limits',
    'Unlimited organizations',
    'Unlimited members'
] WHERE slug = 'builder';

UPDATE plans SET features = ARRAY[
    '75% more credits than you pay',
    'Higher model rate limits',
    'Unlimited organizations',
    'Unlimited members'
] WHERE slug = 'pro';

UPDATE plans SET features = ARRAY[
    '82% more credits than you pay',
    'Highest model rate limits',
    'Unlimited organizations',
    'Unlimited members'
] WHERE slug = 'business';
