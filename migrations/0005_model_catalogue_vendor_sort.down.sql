DROP INDEX IF EXISTS model_catalogue_vendor_sort_idx;

ALTER TABLE model_catalogue
    DROP COLUMN IF EXISTS sort_order,
    DROP COLUMN IF EXISTS vendor;
