DROP INDEX IF EXISTS model_catalogue_modalities_idx;
DROP INDEX IF EXISTS model_catalogue_tags_idx;

ALTER TABLE model_catalogue
    DROP COLUMN IF EXISTS modalities,
    DROP COLUMN IF EXISTS tags;
