ALTER TABLE inference_requests
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

UPDATE inference_requests
SET idempotency_key = id::text
WHERE idempotency_key IS NULL OR idempotency_key = '';

ALTER TABLE inference_requests
    ALTER COLUMN idempotency_key SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS inference_requests_idempotency_key_key
    ON inference_requests (idempotency_key);
