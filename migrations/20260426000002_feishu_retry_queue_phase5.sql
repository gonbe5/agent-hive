ALTER TABLE feishu_outbound_retry_queue
    ADD COLUMN IF NOT EXISTS tenant_key TEXT NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_feishu_retry_queue_tenant
    ON feishu_outbound_retry_queue(tenant_key);
