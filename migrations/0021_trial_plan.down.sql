DELETE FROM plans WHERE slug = 'trial';

UPDATE plans SET includes_from = '' WHERE slug = 'starter';
UPDATE plans SET sort_order = 1 WHERE slug = 'starter';
UPDATE plans SET sort_order = 2 WHERE slug = 'builder';
UPDATE plans SET sort_order = 3 WHERE slug = 'pro';
UPDATE plans SET sort_order = 4 WHERE slug = 'business';

ALTER TABLE plans DROP CONSTRAINT IF EXISTS plans_slug_check;
ALTER TABLE plans ADD CONSTRAINT plans_slug_check
    CHECK (slug IN ('starter', 'builder', 'pro', 'business'));
