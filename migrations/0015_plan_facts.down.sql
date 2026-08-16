ALTER TABLE plans
    DROP COLUMN IF EXISTS tagline,
    DROP COLUMN IF EXISTS features,
    DROP COLUMN IF EXISTS includes_from,
    DROP COLUMN IF EXISTS highlighted;
