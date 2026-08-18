DROP INDEX IF EXISTS model_catalogue_slug_idx;

ALTER TABLE model_catalogue
    DROP COLUMN IF EXISTS slug;
