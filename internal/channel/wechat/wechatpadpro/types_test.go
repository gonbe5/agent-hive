package wechatpadpro

import "testing"

func TestWSMsgData_IsTextMessage(t *testing.T) {
	msg := &WSMsgData{MsgType: 1}
	if !msg.IsTextMessage() {
		t.Error("MsgType=1 应该是文本消息")
	}

	msg.MsgType = 3
	if msg.IsTextMessage() {
		t.Error("MsgType=3 不应该是文本消息")
	}
}

func TestWSMsgData_IsGroupMessage(t *testing.T) {
	msg := &WSMsgData{RoomWxID: "room123"}
	if !msg.IsGroupMessage() {
		t.Error("RoomWxID 不为空应该是群聊消息")
	}

	msg.RoomWxID = ""
	if msg.IsGroupMessage() {
		t.Error("RoomWxID 为空不应该是群聊消息")
	}
}

func TestWSMsgData_GetChatID(t *testing.T) {
	// 群聊消息
	msg := &WSMsgData{
		RoomWxID: "room123",
		FromWxID: "user456",
	}
	if msg.GetChatID() != "room123" {
		t.Errorf("群聊应该返回 room_wxid, 实际 = %s", msg.GetChatID())
	}

	// 私聊消息
	msg.RoomWxID = ""
	if msg.GetChatID() != "user456" {
		t.Errorf("私聊应该返回 from_wxid, 实际 = %s", msg.GetChatID())
	}
}
