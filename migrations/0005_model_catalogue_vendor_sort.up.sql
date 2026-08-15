ALTER TABLE model_catalogue
    ADD COLUMN IF NOT EXISTS vendor TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS model_catalogue_vendor_sort_idx
    ON model_catalogue (vendor, sort_order);
