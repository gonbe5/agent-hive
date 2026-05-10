package store

import (
	"strings"
	"testing"
)

func TestPGInitSQLIncludesUserExternalIDsAndWechatConversations(t *testing.T) {
	sql := strings.Join(strings.Fields(pgInitSQL), " ")
	required := []string{
		"CREATE TABLE IF NOT EXISTS user_external_ids",
		"user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE",
		"provider_type TEXT NOT NULL",
		"external_id TEXT NOT NULL",
		"UNIQUE (provider_type, external_id)",
		"UNIQUE (user_id, provider_type)",
		"CREATE INDEX IF NOT EXISTS idx_user_external_ids_user_provider",
		"ON user_external_ids(user_id, provider_type)",
		"CREATE TABLE IF NOT EXISTS wechat_conversations",
		"owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE",
		"owner_account_id TEXT NOT NULL",
		"peer_wxid TEXT NOT NULL",
		"session_id TEXT NOT NULL",
		"chat_type TEXT NOT NULL DEFAULT 'direct'",
		"can_send BOOLEAN NOT NULL DEFAULT FALSE",
		"send_state TEXT NOT NULL DEFAULT 'unknown'",
		"UNIQUE (owner_user_id, peer_wxid)",
		"UNIQUE (session_id)",
		"CREATE INDEX IF NOT EXISTS idx_wechat_conversations_owner_last",
		"ON wechat_conversations(owner_user_id, last_message_at DESC NULLS LAST)",
	}

	for _, needle := range required {
		if !strings.Contains(sql, needle) {
			t.Fatalf("pgInitSQL missing %q", needle)
		}
	}
}
