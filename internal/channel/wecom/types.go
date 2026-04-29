package wecom

import "encoding/xml"

// WeComCallback 企业微信回调 XML 消息结构
type WeComCallback struct {
	XMLName    xml.Name `xml:"xml"`
	ToUserName string   `xml:"ToUserName"`
	AgentID    string   `xml:"AgentID"`
	Encrypt    string   `xml:"Encrypt"`
}

// WeComMessage 企业微信解密后的消息结构
type WeComMessage struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	MsgID        int64    `xml:"MsgId"`
	AgentID      int      `xml:"AgentID"`
}

// WeComReply 企业微信回复消息格式（JSON API）
type WeComReply struct {
	ToUser  string      `json:"touser"`
	MsgType string      `json:"msgtype"`
	AgentID int         `json:"agentid"`
	Text    *WeComText  `json:"text,omitempty"`
}

// WeComText 文本消息内容
type WeComText struct {
	Content string `json:"content"`
}

// AccessTokenResp 访问令牌响应
type AccessTokenResp struct {
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}
