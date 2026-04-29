package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/chef-guo/agents-hive/internal/errs"
	"go.uber.org/zap"
)

// Client 钉钉 API 客户端
type Client struct {
	httpClient *http.Client
	logger     *zap.Logger
}

func NewClient(logger *zap.Logger) *Client {
	return &Client{
		httpClient: &http.Client{},
		logger:     logger,
	}
}

// SendByWebhook 通过 sessionWebhook 发送消息（钉钉机器人回复）
func (c *Client) SendByWebhook(ctx context.Context, webhookURL, content string) error {
	msg := DingTalkResponse{
		MsgType: "text",
		Text:    &TextMsg{Content: content},
	}
	body, _ := json.Marshal(msg)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "创建请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "发送消息失败", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return errs.New(errs.CodeChannelSendFailed, fmt.Sprintf("钉钉 API 返回状态码 %d", resp.StatusCode))
	}
	return nil
}
