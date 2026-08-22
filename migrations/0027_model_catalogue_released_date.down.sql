DROP INDEX IF EXISTS model_catalogue_released_date_idx;

ALTER TABLE model_catalogue
    DROP COLUMN IF EXISTS model_released_date;
