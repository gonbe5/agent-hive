package wecom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/chef-guo/agents-hive/internal/errs"
	"go.uber.org/zap"
)

// Client 企业微信 API 客户端
type Client struct {
	corpID     string
	secret     string
	agentID    int
	httpClient *http.Client
	logger     *zap.Logger

	// 访问令牌缓存
	accessToken string
	tokenExpiry time.Time
	tokenMu     sync.Mutex
}

// NewClient 创建企业微信 API 客户端
func NewClient(corpID, secret string, agentID int, logger *zap.Logger) *Client {
	return &Client{
		corpID:     corpID,
		secret:     secret,
		agentID:    agentID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

// getAccessToken 获取或刷新访问令牌
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		c.corpID, c.secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", errs.Wrap(errs.CodeChannelSendFailed, "创建获取令牌请求失败", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errs.Wrap(errs.CodeChannelSendFailed, "获取访问令牌失败", err)
	}
	defer resp.Body.Close()

	var result AccessTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errs.Wrap(errs.CodeChannelSendFailed, "解析令牌响应失败", err)
	}

	if result.ErrCode != 0 {
		return "", errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("企业微信 API 返回错误: %d %s", result.ErrCode, result.ErrMsg))
	}

	c.accessToken = result.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)
	return c.accessToken, nil
}

// SendMessage 发送消息
func (c *Client) SendMessage(ctx context.Context, toUser, content string) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}

	msg := WeComReply{
		ToUser:  toUser,
		MsgType: "text",
		AgentID: c.agentID,
		Text:    &WeComText{Content: content},
	}
	body, _ := json.Marshal(msg)

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "创建发送消息请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "发送消息失败", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("企业微信 API 返回状态码 %d", resp.StatusCode))
	}
	return nil
}
