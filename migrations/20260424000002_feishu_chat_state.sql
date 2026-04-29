-- migrations/20260424000002_feishu_chat_state.sql
CREATE TABLE feishu_chat_state (
    platform VARCHAR(50) NOT NULL,
    tenant_key VARCHAR(255) NOT NULL,
    chat_id VARCHAR(255) NOT NULL,
    session_id VARCHAR(255) NOT NULL DEFAULT '',
    state VARCHAR(32) NOT NULL DEFAULT 'active',
    mute_until TIMESTAMP WITH TIME ZONE,
    rollout_mode VARCHAR(32) NOT NULL DEFAULT 'allow',
    suppress_outbound BOOLEAN NOT NULL DEFAULT FALSE,
    last_lifecycle_event_id VARCHAR(255) NOT NULL DEFAULT '',
    last_lifecycle_event_time BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by VARCHAR(255) NOT NULL DEFAULT '',
    PRIMARY KEY (platform, tenant_key, chat_id),
    CONSTRAINT chk_feishu_chat_state_state CHECK (state IN ('active', 'evicted')),
    CONSTRAINT chk_feishu_chat_state_rollout_mode CHECK (rollout_mode IN ('allow', 'deny'))
);

CREATE INDEX idx_feishu_chat_state_session_id_nonempty
    ON feishu_chat_state (session_id)
    WHERE session_id <> '';

CREATE INDEX idx_feishu_chat_state_suppressed_lookup
    ON feishu_chat_state (platform, tenant_key, chat_id)
    WHERE suppress_outbound = TRUE OR mute_until IS NOT NULL;
