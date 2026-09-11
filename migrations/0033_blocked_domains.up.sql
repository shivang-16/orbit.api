CREATE TABLE IF NOT EXISTS blocked_domains (
    domain TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO blocked_domains (domain)
VALUES
    ('shit.ralsei.lol'),
    ('beetleai.dev'),
    ('uberip.com')
ON CONFLICT (domain) DO NOTHING;
