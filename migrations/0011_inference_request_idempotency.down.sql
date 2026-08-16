DROP INDEX IF EXISTS inference_requests_idempotency_key_key;

ALTER TABLE inference_requests
    DROP COLUMN IF EXISTS idempotency_key;
