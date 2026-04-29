package wechatpadpro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/chef-guo/agents-hive/internal/errs"
	"go.uber.org/zap"
)

// HTTPClient WeChatPadPro HTTP 客户端
type HTTPClient struct {
	baseURL    string       // WeChatPadPro 服务地址 (例如: http://localhost:1238)
	key        string       // API 授权 Key (必需)
	httpClient *http.Client // 底层 HTTP 客户端
	logger     *zap.Logger  // 日志
}

// HTTPClientConfig 客户端配置
type HTTPClientConfig struct {
	BaseURL string        // 必填：服务地址
	Key     string        // 必填：授权 Key
	Timeout time.Duration // 可选：超时时间，默认 30s
	Logger  *zap.Logger   // 可选：日志（nil 使用 nop logger）
}

// NewHTTPClient 创建 HTTP 客户端
func NewHTTPClient(cfg HTTPClientConfig) *HTTPClient {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	return &HTTPClient{
		baseURL: cfg.BaseURL,
		key:     cfg.Key,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: cfg.Logger,
	}
}

// doRequest 执行 HTTP 请求（通用方法）
// WeChatPadPro 所有 API 都需要在 URL 中添加 ?key= 参数
func (c *HTTPClient) doRequest(ctx context.Context, method, path string, reqBody interface{}) (*APIResponse, error) {
	// 构建完整 URL，添加 key 参数
	apiURL, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, errs.New(errs.CodeWeChatPadProConnectFailed, "无效的 URL: "+err.Error())
	}

	// 添加 key 参数
	query := apiURL.Query()
	query.Set("key", c.key)
	apiURL.RawQuery = query.Encode()

	var bodyReader io.Reader
	if reqBody != nil {
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, errs.New(errs.CodeWeChatPadProInvalidResp, "请求序列化失败: "+err.Error())
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, apiURL.String(), bodyReader)
	if err != nil {
		return nil, errs.New(errs.CodeWeChatPadProConnectFailed, "创建请求失败: "+err.Error())
	}

	// 设置请求头
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// 发送请求
	c.logger.Debug("发送 HTTP 请求",
		zap.String("method", method),
		zap.String("url", apiURL.String()),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errs.New(errs.CodeWeChatPadProConnectFailed, "请求失败: "+err.Error())
	}
	defer resp.Body.Close()

	// 读取响应
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errs.New(errs.CodeWeChatPadProInvalidResp, "读取响应失败: "+err.Error())
	}

	c.logger.Debug("收到 HTTP 响应",
		zap.Int("status", resp.StatusCode),
		zap.String("body", string(respBytes)),
	)

	// 解析响应
	var apiResp APIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		c.logger.Error("响应解析失败",
			zap.String("body", string(respBytes)),
			zap.Error(err),
		)
		return nil, errs.New(errs.CodeWeChatPadProInvalidResp, "响应解析失败: "+err.Error())
	}

	// 检查业务错误 (Code=200 表示成功)
	if apiResp.Code != 200 {
		c.logger.Warn("API 返回错误",
			zap.Int("code", apiResp.Code),
			zap.String("text", apiResp.Text),
		)
		return &apiResp, errs.New(errs.CodeWeChatPadProAPIError, fmt.Sprintf("API 错误 (code=%d): %s", apiResp.Code, apiResp.Text))
	}

	return &apiResp, nil
}

// doRequestWithKey 使用指定 Key 执行 HTTP 请求（用于管理接口）
func (c *HTTPClient) doRequestWithKey(ctx context.Context, method, path string, reqBody interface{}, key string) (*APIResponse, error) {
	// 构建完整 URL，添加 key 参数
	apiURL, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, errs.New(errs.CodeWeChatPadProConnectFailed, "无效的 URL: "+err.Error())
	}

	// 添加指定的 key 参数
	query := apiURL.Query()
	query.Set("key", key)
	apiURL.RawQuery = query.Encode()

	var bodyReader io.Reader
	if reqBody != nil {
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, errs.New(errs.CodeWeChatPadProInvalidResp, "请求序列化失败: "+err.Error())
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, apiURL.String(), bodyReader)
	if err != nil {
		return nil, errs.New(errs.CodeWeChatPadProConnectFailed, "创建请求失败: "+err.Error())
	}

	// 设置请求头
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// 发送请求
	c.logger.Debug("发送 HTTP 请求（自定义 Key）",
		zap.String("method", method),
		zap.String("url", apiURL.String()),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errs.New(errs.CodeWeChatPadProConnectFailed, "请求失败: "+err.Error())
	}
	defer resp.Body.Close()

	// 读取响应
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errs.New(errs.CodeWeChatPadProInvalidResp, "读取响应失败: "+err.Error())
	}

	c.logger.Debug("收到 HTTP 响应",
		zap.Int("status", resp.StatusCode),
		zap.String("body", string(respBytes)),
	)

	// 解析响应
	var apiResp APIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		c.logger.Error("响应解析失败",
			zap.String("body", string(respBytes)),
			zap.Error(err),
		)
		return nil, errs.New(errs.CodeWeChatPadProInvalidResp, "响应解析失败: "+err.Error())
	}

	// 检查业务错误 (Code=200 表示成功)
	if apiResp.Code != 200 {
		c.logger.Warn("API 返回错误",
			zap.Int("code", apiResp.Code),
			zap.String("text", apiResp.Text),
		)
		return &apiResp, errs.New(errs.CodeWeChatPadProAPIError, fmt.Sprintf("API 错误 (code=%d): %s", apiResp.Code, apiResp.Text))
	}

	return &apiResp, nil
}

// parseResponseData 泛型响应解析辅助
// 将 APIResponse.Data 解析为指定类型，减少重复的 marshal/unmarshal 代码
func parseResponseData[T any](resp *APIResponse) (T, error) {
	var result T
	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return result, errs.New(errs.CodeWeChatPadProInvalidResp, "响应数据序列化失败: "+err.Error())
	}
	if err := json.Unmarshal(dataBytes, &result); err != nil {
		return result, errs.New(errs.CodeWeChatPadProInvalidResp, "响应数据解析失败: "+err.Error())
	}
	return result, nil
}

// CheckLoginStatus 检查微信登录状态
func (c *HTTPClient) CheckLoginStatus(ctx context.Context) (*LoginStatusData, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/login/CheckLoginStatus", nil)
	if err != nil {
		// Code:300 "不存在状态" 表示未登录，不是真正的错误
		if resp != nil && resp.Code == 300 {
			c.logger.Info("微信未登录（不存在状态）")
			return &LoginStatusData{IsLogin: false}, nil
		}
		return nil, err
	}

	// 解析 Data 字段
	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, errs.New(errs.CodeWeChatPadProInvalidResp, "解析登录状态失败: "+err.Error())
	}

	var status LoginStatusData
	if err := json.Unmarshal(dataBytes, &status); err != nil {
		return nil, errs.New(errs.CodeWeChatPadProInvalidResp, "解析登录状态失败: "+err.Error())
	}

	c.logger.Info("查询登录状态成功",
		zap.Bool("is_login", status.IsLogin),
		zap.String("wxid", status.WxID),
	)

	return &status, nil
}

// GetQRCode 获取登录二维码
func (c *HTTPClient) GetQRCode(ctx context.Context) (*QRCodeData, error) {
	// 请求体可以为空，或者配置代理
	reqBody := GetLoginQrCodeModel{}

	resp, err := c.doRequest(ctx, http.MethodPost, "/login/GetLoginQrCodeNewX", reqBody)
	if err != nil {
		return nil, err
	}

	// 解析 Data 字段
	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, errs.New(errs.CodeWeChatPadProInvalidResp, "解析二维码数据失败: "+err.Error())
	}

	var qrCode QRCodeData
	if err := json.Unmarshal(dataBytes, &qrCode); err != nil {
		return nil, errs.New(errs.CodeWeChatPadProInvalidResp, "解析二维码数据失败: "+err.Error())
	}

	c.logger.Info("获取二维码成功",
		zap.String("qrcode_url", qrCode.QrCodeUrl),
		zap.String("qr_link", qrCode.QrLink),
		zap.Int("expired_time", qrCode.ExpiredTime),
	)

	return &qrCode, nil
}

// SendTextMessage 发送文本消息
func (c *HTTPClient) SendTextMessage(ctx context.Context, toWxID, content string) error {
	// 构建请求体
	reqBody := SendMessageModel{
		MsgItem: []MessageItem{
			{
				ToUserName:  toWxID,
				TextContent: content,
				MsgType:     1, // 1=文本消息
			},
		},
	}

	_, err := c.doRequest(ctx, http.MethodPost, "/message/SendTextMessage", reqBody)
	if err != nil {
		return err
	}

	c.logger.Info("发送消息成功",
		zap.String("to_wxid", toWxID),
		zap.Int("content_len", len(content)),
	)

	return nil
}
