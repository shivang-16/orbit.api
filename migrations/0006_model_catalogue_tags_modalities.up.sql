ALTER TABLE model_catalogue
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS modalities TEXT[] NOT NULL DEFAULT '{text}';

CREATE INDEX IF NOT EXISTS model_catalogue_tags_idx
    ON model_catalogue USING GIN (tags);

CREATE INDEX IF NOT EXISTS model_catalogue_modalities_idx
    ON model_catalogue USING GIN (modalities);
