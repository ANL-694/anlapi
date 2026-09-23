-- Keep the effective pricing snapshot alongside the durable billing idempotency key.
-- Existing rows remain valid and intentionally receive NULL.

ALTER TABLE usage_billing_dedup
    ADD COLUMN IF NOT EXISTS pricing_version VARCHAR(128);

ALTER TABLE usage_billing_dedup_archive
    ADD COLUMN IF NOT EXISTS pricing_version VARCHAR(128);
