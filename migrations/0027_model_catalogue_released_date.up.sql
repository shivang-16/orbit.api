-- Real public release date of the model (vendor announcement / model card /
-- changelog), used for the "Newest" sort in the catalogue API and web UI.
-- Values are written by 0028 (in-place UPDATE by model_id) so existing
-- catalogue UUIDs stay intact. cmd/seed also writes this column on a
-- local wipe-and-reinsert; do not run seed against prod.
ALTER TABLE model_catalogue
    ADD COLUMN IF NOT EXISTS model_released_date DATE;

CREATE INDEX IF NOT EXISTS model_catalogue_released_date_idx
    ON model_catalogue (model_released_date DESC);
