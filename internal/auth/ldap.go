package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/go-ldap/ldap/v3"
	"go.uber.org/zap"
)

// LDAPAuthConfig LDAP 认证配置
type LDAPAuthConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	BaseDN   string `json:"base_dn"`
	BindDN   string `json:"bind_dn"`
	BindPass string `json:"bind_password"`
	UseTLS   bool   `json:"use_tls"`
}

// LDAPProvider LDAP 认证 provider
type LDAPProvider struct {
	host     string
	port     int
	baseDN   string
	bindDN   string
	bindPass string
	useTLS   bool
	logger   *zap.Logger
}

// NewLDAPProvider 创建 LDAP provider
func NewLDAPProvider(cfg LDAPAuthConfig) *LDAPProvider {
	port := cfg.Port
	if port == 0 {
		port = 389
	}
	return &LDAPProvider{
		host:     cfg.Host,
		port:     port,
		baseDN:   cfg.BaseDN,
		bindDN:   cfg.BindDN,
		bindPass: cfg.BindPass,
		useTLS:   cfg.UseTLS,
		logger:   zap.NewNop(),
	}
}

// SetLogger 设置 LDAP provider 的日志记录器
func (p *LDAPProvider) SetLogger(logger *zap.Logger) {
	p.logger = logger
}

func (p *LDAPProvider) Type() string { return "ldap" }

func (p *LDAPProvider) Authenticate(ctx context.Context, username, password string) (*UserInfo, error) {
	addr := fmt.Sprintf("%s:%d", p.host, p.port)

	// 计算剩余超时：优先使用 context deadline，否则默认 10s
	const defaultTimeout = 10 * time.Second
	timeout := defaultTimeout
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dialer := &net.Dialer{Timeout: timeout}

	var conn *ldap.Conn
	var err error
	if p.useTLS {
		conn, err = ldap.DialURL("ldaps://"+addr, ldap.DialWithDialer(dialer))
	} else {
		p.logger.Warn("LDAP 认证使用明文连接，密码将以非加密方式传输，建议启用 TLS",
			zap.String("host", p.host), zap.Int("port", p.port))
		conn, err = ldap.DialURL("ldap://"+addr, ldap.DialWithDialer(dialer))
	}
	if err != nil {
		return nil, fmt.Errorf("连接 LDAP 服务器失败: %w", err)
	}
	defer conn.Close()

	// 将超时应用到连接，控制后续所有操作（Bind、Search）
	conn.SetTimeout(timeout)

	// 服务账号 bind
	if err := conn.Bind(p.bindDN, p.bindPass); err != nil {
		return nil, fmt.Errorf("LDAP 服务账号 bind 失败: %w", err)
	}

	// 搜索用户
	searchReq := ldap.NewSearchRequest(
		p.baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(uid=%s)", ldap.EscapeFilter(username)),
		[]string{"dn", "cn", "mail", "department"},
		nil,
	)
	result, err := conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("LDAP 搜索用户失败: %w", err)
	}
	if len(result.Entries) == 0 {
		return nil, fmt.Errorf("用户不存在")
	}
	entry := result.Entries[0]

	// 用用户 DN + 密码验证
	if err := conn.Bind(entry.DN, password); err != nil {
		return nil, fmt.Errorf("密码错误")
	}

	return &UserInfo{
		ExternalID:  entry.DN,
		DisplayName: entry.GetAttributeValue("cn"),
		Email:       entry.GetAttributeValue("mail"),
		Department:  entry.GetAttributeValue("department"),
	}, nil
}

// unmarshalConfig 辅助函数：将 JSON 反序列化到目标结构
func unmarshalConfig(data json.RawMessage, v interface{}) error {
	return json.Unmarshal(data, v)
}
