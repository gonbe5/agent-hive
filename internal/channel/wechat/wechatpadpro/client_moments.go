package wechatpadpro

import (
	"context"
	"net/http"
)

// PostMoment 发布朋友圈
func (c *HTTPClient) PostMoment(ctx context.Context, content string, images []string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/sns/SendFriendCircle", PostMomentReq{
		Content: content,
		Images:  images,
	})
	return err
}

// GetTimeline 获取朋友圈时间线
func (c *HTTPClient) GetTimeline(ctx context.Context) ([]MomentItem, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/sns/SendSnsTimeLine", nil)
	if err != nil {
		return nil, err
	}
	return parseResponseData[[]MomentItem](resp)
}

// GetUserMoments 获取指定用户的朋友圈
func (c *HTTPClient) GetUserMoments(ctx context.Context, wxID string) ([]MomentItem, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/sns/SendSnsUserPage", GetUserMomentsReq{WxID: wxID})
	if err != nil {
		return nil, err
	}
	return parseResponseData[[]MomentItem](resp)
}

// LikeMoment 给朋友圈点赞
func (c *HTTPClient) LikeMoment(ctx context.Context, snsID string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/sns/SendSnsComment", SnsCommentReq{
		SnsID: snsID,
		Type:  1, // 1=点赞
	})
	return err
}

// CommentMoment 评论朋友圈
func (c *HTTPClient) CommentMoment(ctx context.Context, snsID, content string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/sns/SendSnsComment", SnsCommentReq{
		SnsID:   snsID,
		Type:    2, // 2=评论
		Content: content,
	})
	return err
}
