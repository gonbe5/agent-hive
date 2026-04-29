package wechatpadpro

import (
	"context"
	"net/http"
)

// SendImageMessage 发送图片消息
func (c *HTTPClient) SendImageMessage(ctx context.Context, toWxID, imageBase64 string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/message/SendImageMessage", SendImageReq{
		ToWxID:      toWxID,
		ImageBase64: imageBase64,
	})
	return err
}

// SendFileMessage 发送文件消息
func (c *HTTPClient) SendFileMessage(ctx context.Context, toWxID string, fileBase64, fileName string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/message/SendFileMessage", SendFileReq{
		ToWxID:   toWxID,
		Content:  fileBase64,
		FileName: fileName,
	})
	return err
}

// SendEmojiMessage 发送表情消息
func (c *HTTPClient) SendEmojiMessage(ctx context.Context, toWxID, emojiMD5 string, emojiLen int) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/message/SendEmojiMessage", SendEmojiReq{
		ToWxID:   toWxID,
		EmojiMD5: emojiMD5,
		EmojiLen: emojiLen,
	})
	return err
}

// SendCardMessage 发送名片消息
func (c *HTTPClient) SendCardMessage(ctx context.Context, toWxID, cardWxID string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/message/ShareCardMessage", ShareCardReq{
		ToWxID:   toWxID,
		CardWxID: cardWxID,
	})
	return err
}

// RevokeMessage 撤回消息
func (c *HTTPClient) RevokeMessage(ctx context.Context, msgID, toWxID string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/message/RevokeMsg", RevokeMsgReq{
		MsgID:  msgID,
		ToWxID: toWxID,
	})
	return err
}

// ForwardImage 转发图片消息
func (c *HTTPClient) ForwardImage(ctx context.Context, toWxID, xml string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/message/ForwardImageMessage", ForwardMessageReq{
		ToWxID: toWxID,
		XML:    xml,
	})
	return err
}

// ForwardVideo 转发视频消息
func (c *HTTPClient) ForwardVideo(ctx context.Context, toWxID, xml string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/message/ForwardVideoMessage", ForwardMessageReq{
		ToWxID: toWxID,
		XML:    xml,
	})
	return err
}
