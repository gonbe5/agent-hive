-- migrations/20260423000000_feishu_dedup.sql
CREATE TABLE IF NOT EXISTS feishu_event_dedup (
    event_id VARCHAR(255) PRIMARY KEY,
    claimed_at TIMESTAMP WITH TIME ZONE,
    processed BOOLEAN NOT NULL DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE
);
CREATE INDEX idx_feishu_dedup_claimed_unprocessed ON feishu_event_dedup(claimed_at) WHERE processed = FALSE;
