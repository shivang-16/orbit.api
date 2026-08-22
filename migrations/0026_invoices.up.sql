CREATE TABLE IF NOT EXISTS invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    payment_id TEXT NOT NULL,
    invoice_id TEXT NOT NULL DEFAULT '',
    plan_slug TEXT NOT NULL DEFAULT '',
    amount INTEGER NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL DEFAULT 'succeeded',
    refund_status TEXT NOT NULL DEFAULT '',
    subscription_id TEXT NOT NULL DEFAULT '',
    paid_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT invoices_payment_id_unique UNIQUE (payment_id)
);

CREATE INDEX IF NOT EXISTS invoices_organization_id_paid_at_idx
    ON invoices (organization_id, paid_at DESC);
