-- migrations/20260423000001_feishu_retry_queue.sql
CREATE TABLE IF NOT EXISTS feishu_outbound_retry_queue (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(255) NOT NULL,
    platform VARCHAR(50) NOT NULL,
    chat_id VARCHAR(255) NOT NULL,
    sender_id VARCHAR(255) NOT NULL,
    reason VARCHAR(100) NOT NULL,
    error_msg TEXT,
    payload JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    next_retry_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    retry_count INT DEFAULT 0
);
CREATE INDEX idx_feishu_retry_queue_next ON feishu_outbound_retry_queue(next_retry_at) WHERE retry_count < 5;
