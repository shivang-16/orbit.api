ALTER TABLE model_catalogue
    ADD COLUMN IF NOT EXISTS slug TEXT;

-- Mirrors orbit.web's lib/slug.ts slugify(): lowercase, collapse
-- non-alphanumeric runs to '-', trim leading/trailing '-'.
UPDATE model_catalogue
SET slug = regexp_replace(regexp_replace(lower(trim(name)), '[^a-z0-9]+', '-', 'g'), '(^-+)|(-+$)', '', 'g')
WHERE slug IS NULL;

ALTER TABLE model_catalogue
    ALTER COLUMN slug SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS model_catalogue_slug_idx
    ON model_catalogue (slug);
