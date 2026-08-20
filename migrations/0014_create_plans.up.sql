CREATE TABLE IF NOT EXISTS plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    dodo_product_id TEXT NOT NULL UNIQUE,
    price_micros BIGINT NOT NULL,
    credits_micros BIGINT NOT NULL,
    sort_order INTEGER NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT plans_slug_check CHECK (slug IN ('starter', 'builder', 'pro', 'business')),
    CONSTRAINT plans_amounts_positive CHECK (
        price_micros > 0
        AND credits_micros > 0
        AND credits_micros >= price_micros
    )
);

CREATE INDEX IF NOT EXISTS plans_sort_order_idx ON plans (sort_order);

DROP TRIGGER IF EXISTS plans_set_updated_at ON plans;
CREATE TRIGGER plans_set_updated_at
BEFORE UPDATE ON plans
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

INSERT INTO plans (slug, name, dodo_product_id, price_micros, credits_micros, sort_order)
VALUES
    ('starter',  'Starter',  'pdt_0NleIavFwkwJCJy2c9cKe',  20000000,  30000000,  1),
    ('builder',  'Builder',  'pdt_0NleIejHQ3mnyg1HuKmrY',  60000000, 100000000,  2),
    ('pro',      'Pro',      'pdt_0NleInH0ijd3pI3iEwIkV', 200000000, 350000000,  3),
    ('business', 'Business', 'pdt_0NleIyEdsxP1cshL9Vo8g', 500000000, 910000000,  4)
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    dodo_product_id = EXCLUDED.dodo_product_id,
    price_micros = EXCLUDED.price_micros,
    credits_micros = EXCLUDED.credits_micros,
    sort_order = EXCLUDED.sort_order,
    is_active = true;
