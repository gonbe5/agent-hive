package gateway

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// AuthToken 认证令牌
type AuthToken struct {
	Token  string   `json:"token"`
	Scopes []string `json:"scopes"` // "read", "write", "admin"
}

// AuthManager 管理 Token 认证
type AuthManager struct {
	tokens map[string]*AuthToken
	mu     sync.RWMutex
}

// NewAuthManager 创建认证管理器
func NewAuthManager(presetTokens []string) *AuthManager {
	am := &AuthManager{
		tokens: make(map[string]*AuthToken),
	}
	// 预设 token 拥有所有权限（跳过空字符串，防止环境变量未设置时注册无效 token）
	for _, t := range presetTokens {
		if t == "" {
			continue
		}
		am.tokens[t] = &AuthToken{
			Token:  t,
			Scopes: []string{"read", "write", "admin"},
		}
	}
	return am
}

// Authenticate 验证 token，支持 "Bearer <token>" 格式
func (am *AuthManager) Authenticate(tokenStr string) (*AuthToken, error) {
	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return nil, nil
	}

	am.mu.RLock()
	defer am.mu.RUnlock()

	t, ok := am.tokens[tokenStr]
	if !ok {
		return nil, nil
	}
	return t, nil
}

// HasScope 检查 token 是否有指定权限。
// 当未配置任何 token 时（本地开发模式），允许所有请求。
func (am *AuthManager) HasScope(token *AuthToken, scope string) bool {
	// 无 token 配置 = 开放模式（本地开发），跳过认证
	am.mu.RLock()
	noTokens := len(am.tokens) == 0
	am.mu.RUnlock()
	if noTokens {
		return true
	}

	if token == nil {
		return false
	}
	for _, s := range token.Scopes {
		if s == scope || s == "admin" {
			return true
		}
	}
	return false
}

// ---- IP 速率限制器（滑动窗口算法）----

// ipBucket 记录单个 IP 的滑动窗口请求时间戳
type ipBucket struct {
	mu         sync.Mutex
	timestamps []time.Time // 当前窗口内的请求时间戳（按时间顺序）
}

// IPRateLimiter 基于内存的 IP 级别速率限制器。
// 使用滑动窗口算法，按 IP 地址独立计数。
// 过期 IP 条目由后台 goroutine 定期清理，防止内存无限增长。
type IPRateLimiter struct {
	mu             sync.Mutex
	buckets        map[string]*ipBucket // key: IP 地址字符串
	limit          int                  // 窗口内允许的最大请求数
	window         time.Duration        // 滑动窗口大小
	stopCh         chan struct{}         // 停止清理 goroutine 的信号
	trustedProxies []net.IP             // 受信任的反向代理 IP 列表
}

// NewIPRateLimiter 创建速率限制器。
// limit 为每个窗口允许的最大请求数，window 为滑动窗口时长。
// 例如: NewIPRateLimiter(60, time.Minute) 表示每分钟最多 60 次请求。
// 受信任代理 IP 从环境变量 TRUSTED_PROXIES（逗号分隔）读取；
// 未配置时默认为空，此时始终使用 RemoteAddr，不信任任何代理头。
func NewIPRateLimiter(limit int, window time.Duration) *IPRateLimiter {
	rl := &IPRateLimiter{
		buckets:        make(map[string]*ipBucket),
		limit:          limit,
		window:         window,
		stopCh:         make(chan struct{}),
		trustedProxies: parseTrustedProxies(os.Getenv("TRUSTED_PROXIES")),
	}
	// 启动后台清理协程，每隔 2 个窗口周期清理过期 IP
	go rl.cleanupLoop(window * 2)
	return rl
}

// NewIPRateLimiterWithProxies 创建速率限制器（显式指定受信任代理列表，便于测试）。
// proxies 为受信任代理的 IP 字符串列表（可包含 CIDR 表示法）；
// 传入 nil 或空切片等同于不信任任何代理头，始终使用 RemoteAddr。
func NewIPRateLimiterWithProxies(limit int, window time.Duration, proxies []string) *IPRateLimiter {
	rl := &IPRateLimiter{
		buckets:        make(map[string]*ipBucket),
		limit:          limit,
		window:         window,
		stopCh:         make(chan struct{}),
		trustedProxies: parseTrustedProxies(strings.Join(proxies, ",")),
	}
	go rl.cleanupLoop(window * 2)
	return rl
}

// parseTrustedProxies 解析逗号分隔的 IP/CIDR 字符串列表，返回解析后的 net.IP 切片。
// 无效条目静默跳过（生产日志应在调用方检查）。
func parseTrustedProxies(raw string) []net.IP {
	if raw == "" {
		return nil
	}
	var result []net.IP
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		ip := net.ParseIP(part)
		if ip == nil {
			// 忽略无法解析的条目（应在启动时由配置校验报告）
			continue
		}
		result = append(result, ip)
	}
	return result
}

// isTrustedProxy 判断给定 IP 字符串是否在受信任代理列表中
func (rl *IPRateLimiter) isTrustedProxy(remoteIP string) bool {
	if len(rl.trustedProxies) == 0 {
		return false
	}
	ip := net.ParseIP(remoteIP)
	if ip == nil {
		return false
	}
	for _, trusted := range rl.trustedProxies {
		if trusted.Equal(ip) {
			return true
		}
	}
	return false
}

// ExtractClientIP 从请求中提取真实客户端 IP。
// 安全策略：只有当 RemoteAddr 属于受信任代理列表时，
// 才信任 X-Forwarded-For / X-Real-IP 头；否则直接使用 RemoteAddr，
// 防止攻击者伪造代理头绕过速率限制。
func (rl *IPRateLimiter) ExtractClientIP(r *http.Request) string {
	// 先解析 RemoteAddr（直连对端地址，不可伪造）
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}

	// 仅当直连对端在受信任代理列表中时，才读取代理注入的头
	if rl.isTrustedProxy(remoteIP) {
		// X-Forwarded-For 的第一个地址为原始客户端 IP
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := xff
			if idx := strings.Index(xff, ","); idx != -1 {
				first = xff[:idx]
			}
			if ip := strings.TrimSpace(first); ip != "" {
				return ip
			}
		}
		// X-Real-IP（Nginx 等反向代理通常设置此头）
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
	}

	// 非受信任代理，或代理头为空：直接使用 RemoteAddr
	return remoteIP
}

// Allow 检查指定 IP 是否允许本次请求。
// 返回 true 表示允许，false 表示超限（应返回 429）。
func (rl *IPRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	bucket, ok := rl.buckets[ip]
	if !ok {
		bucket = &ipBucket{}
		rl.buckets[ip] = bucket
	}
	rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	// 移除窗口外的过期时间戳
	valid := bucket.timestamps[:0]
	for _, ts := range bucket.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	bucket.timestamps = valid

	// 检查是否超限
	if len(bucket.timestamps) >= rl.limit {
		return false
	}

	// 记录本次请求时间戳
	bucket.timestamps = append(bucket.timestamps, now)
	return true
}

// cleanupLoop 定期删除长时间无请求的 IP 条目，防止内存泄漏
func (rl *IPRateLimiter) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCh:
			return
		}
	}
}

// cleanup 删除所有时间戳均已过期的 IP 条目
func (rl *IPRateLimiter) cleanup() {
	cutoff := time.Now().Add(-rl.window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	for ip, bucket := range rl.buckets {
		bucket.mu.Lock()
		allExpired := true
		for _, ts := range bucket.timestamps {
			if ts.After(cutoff) {
				allExpired = false
				break
			}
		}
		bucket.mu.Unlock()

		if allExpired {
			delete(rl.buckets, ip)
		}
	}
}

// Stop 停止后台清理协程
func (rl *IPRateLimiter) Stop() {
	close(rl.stopCh)
}

// ExtractClientIP 包级别便捷函数，不信任任何代理头（等同于 trustedProxies 为空）。
// 适用于无反向代理的直连场景，或临时调用场景。
// 如需代理感知，请使用 IPRateLimiter.ExtractClientIP。
func ExtractClientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
