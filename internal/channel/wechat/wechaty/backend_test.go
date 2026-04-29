package wechaty

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chef-guo/agents-hive/internal/channel/wechat"
	pb "github.com/chef-guo/agents-hive/internal/channel/wechat/wechaty/proto"
	"github.com/chef-guo/agents-hive/internal/config"
)

func newTestLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	return l
}

func TestBackendName(t *testing.T) {
	b := New(config.WechatyInstanceConfig{}, newTestLogger())
	assert.Equal(t, "wechaty", b.Name())
}

func TestBackendIsLoggedIn(t *testing.T) {
	b := New(config.WechatyInstanceConfig{}, newTestLogger())
	assert.False(t, b.IsLoggedIn())
}

func TestBackendSetMessageHandler(t *testing.T) {
	b := New(config.WechatyInstanceConfig{}, newTestLogger())
	called := false
	b.SetMessageHandler(func(msg wechat.IncomingMessage) {
		called = true
	})
	require.NotNil(t, b.handler)
	b.handler(wechat.IncomingMessage{})
	assert.True(t, called)
}

func TestBackendSendText_NotLoggedIn(t *testing.T) {
	b := New(config.WechatyInstanceConfig{}, newTestLogger())
	err := b.SendText(context.Background(), "wxid_xxx", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未连接")
}

func TestBackendStop_NoConnection(t *testing.T) {
	b := New(config.WechatyInstanceConfig{}, newTestLogger())
	err := b.Stop()
	require.NoError(t, err)
}

// mockPuppetServer 模拟 Wechaty gRPC 服务器
type mockPuppetServer struct {
	pb.UnimplementedPuppetServer
	loginPayload  string
	msgPayloads   map[string]*pb.MessagePayloadResponse
	sendTextCalls []sendTextCall
}

type sendTextCall struct {
	ConversationID string
	Text           string
}

func (s *mockPuppetServer) Start(_ context.Context, _ *pb.StartRequest) (*pb.StartResponse, error) {
	return &pb.StartResponse{}, nil
}

func (s *mockPuppetServer) Stop(_ context.Context, _ *pb.StopRequest) (*pb.StopResponse, error) {
	return &pb.StopResponse{}, nil
}

func (s *mockPuppetServer) Logout(_ context.Context, _ *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	return &pb.LogoutResponse{}, nil
}

func (s *mockPuppetServer) Event(_ *pb.EventRequest, stream grpc.ServerStreamingServer[pb.EventResponse]) error {
	// 发送登录事件
	if err := stream.Send(&pb.EventResponse{
		Type:    pb.EventType_EVENT_TYPE_LOGIN,
		Payload: s.loginPayload,
	}); err != nil {
		return err
	}

	// 发送一条消息事件
	msgPayload, _ := json.Marshal(eventPayload{MessageID: "msg_001"})
	if err := stream.Send(&pb.EventResponse{
		Type:    pb.EventType_EVENT_TYPE_MESSAGE,
		Payload: string(msgPayload),
	}); err != nil {
		return err
	}

	// 保持流打开直到客户端关闭
	<-stream.Context().Done()
	return nil
}

func (s *mockPuppetServer) MessageSendText(_ context.Context, req *pb.MessageSendTextRequest) (*pb.MessageSendTextResponse, error) {
	s.sendTextCalls = append(s.sendTextCalls, sendTextCall{
		ConversationID: req.ConversationId,
		Text:           req.Text,
	})
	return &pb.MessageSendTextResponse{Id: "sent_001"}, nil
}

func (s *mockPuppetServer) MessagePayload(_ context.Context, req *pb.MessagePayloadRequest) (*pb.MessagePayloadResponse, error) {
	if resp, ok := s.msgPayloads[req.Id]; ok {
		return resp, nil
	}
	return &pb.MessagePayloadResponse{
		Id:        req.Id,
		ContactId: "wxid_sender",
		Type:      pb.MessageType_MESSAGE_TYPE_TEXT,
		Text:      "测试消息",
		Timestamp: uint64(time.Now().Unix()),
	}, nil
}

func (s *mockPuppetServer) ContactPayload(_ context.Context, req *pb.ContactPayloadRequest) (*pb.ContactPayloadResponse, error) {
	return &pb.ContactPayloadResponse{
		Id:   req.Id,
		Name: "测试用户",
	}, nil
}

func (s *mockPuppetServer) RoomPayload(_ context.Context, req *pb.RoomPayloadRequest) (*pb.RoomPayloadResponse, error) {
	return &pb.RoomPayloadResponse{
		Id:    req.Id,
		Topic: "测试群",
	}, nil
}

// startMockServer 启动模拟 gRPC 服务器
func startMockServer(t *testing.T, srv *mockPuppetServer) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	pb.RegisterPuppetServer(grpcServer, srv)

	go grpcServer.Serve(lis)

	return lis.Addr().String(), func() {
		grpcServer.Stop()
	}
}

func TestBackendStartAndStop(t *testing.T) {
	srv := &mockPuppetServer{
		loginPayload: `{"contactId":"wxid_self"}`,
	}
	addr, cleanup := startMockServer(t, srv)
	defer cleanup()

	b := New(config.WechatyInstanceConfig{
		Endpoint: addr,
	}, newTestLogger())

	err := b.Start(context.Background())
	require.NoError(t, err)

	// 等待事件流中的登录事件
	time.Sleep(200 * time.Millisecond)
	assert.True(t, b.IsLoggedIn())

	err = b.Stop()
	require.NoError(t, err)
	assert.False(t, b.IsLoggedIn())
}

func TestBackendSendText_Success(t *testing.T) {
	srv := &mockPuppetServer{
		loginPayload: `{"contactId":"wxid_self"}`,
	}
	addr, cleanup := startMockServer(t, srv)
	defer cleanup()

	b := New(config.WechatyInstanceConfig{
		Endpoint: addr,
	}, newTestLogger())

	err := b.Start(context.Background())
	require.NoError(t, err)
	defer b.Stop()

	// 等待登录
	time.Sleep(200 * time.Millisecond)

	err = b.SendText(context.Background(), "wxid_friend", "你好")
	require.NoError(t, err)

	require.Len(t, srv.sendTextCalls, 1)
	assert.Equal(t, "wxid_friend", srv.sendTextCalls[0].ConversationID)
	assert.Equal(t, "你好", srv.sendTextCalls[0].Text)
}

func TestBackendReceiveMessage(t *testing.T) {
	srv := &mockPuppetServer{
		loginPayload: `{"contactId":"wxid_self"}`,
	}
	addr, cleanup := startMockServer(t, srv)
	defer cleanup()

	received := make(chan wechat.IncomingMessage, 1)

	b := New(config.WechatyInstanceConfig{
		Endpoint: addr,
	}, newTestLogger())
	b.SetMessageHandler(func(msg wechat.IncomingMessage) {
		received <- msg
	})

	err := b.Start(context.Background())
	require.NoError(t, err)
	defer b.Stop()

	select {
	case msg := <-received:
		assert.Equal(t, "wxid_sender", msg.FromUser)
		assert.Equal(t, "测试消息", msg.Content)
		assert.Equal(t, wechat.MsgText, msg.MsgType)
		assert.Equal(t, "测试用户", msg.SenderName)
	case <-time.After(5 * time.Second):
		t.Fatal("超时等待消息接收")
	}
}

func TestBackendStart_ConnectFailed(t *testing.T) {
	b := New(config.WechatyInstanceConfig{
		Endpoint: "127.0.0.1:1", // 不存在的端口
	}, newTestLogger())

	// grpc.NewClient 不会立即失败，但 Start RPC 会失败
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := b.Start(ctx)
	require.Error(t, err)
}

func TestBackendSendText_ClientNil(t *testing.T) {
	b := New(config.WechatyInstanceConfig{}, newTestLogger())
	b.loggedIn.Store(true) // 模拟已登录但 client 为空

	err := b.SendText(context.Background(), "wxid_xxx", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "客户端为空")
}

func TestBackendReceiveGroupMessage(t *testing.T) {
	srv := &mockPuppetServer{
		loginPayload: `{"contactId":"wxid_self"}`,
		msgPayloads: map[string]*pb.MessagePayloadResponse{
			"msg_001": {
				Id:        "msg_001",
				ContactId: "wxid_group_member",
				RoomId:    "room_001@chatroom",
				Type:      pb.MessageType_MESSAGE_TYPE_TEXT,
				Text:      "群消息",
				Timestamp: uint64(time.Now().Unix()),
			},
		},
	}
	addr, cleanup := startMockServer(t, srv)
	defer cleanup()

	received := make(chan wechat.IncomingMessage, 1)

	b := New(config.WechatyInstanceConfig{
		Endpoint: addr,
	}, newTestLogger())
	b.SetMessageHandler(func(msg wechat.IncomingMessage) {
		received <- msg
	})

	err := b.Start(context.Background())
	require.NoError(t, err)
	defer b.Stop()

	select {
	case msg := <-received:
		assert.Equal(t, "wxid_group_member", msg.FromUser)
		assert.Equal(t, "room_001@chatroom", msg.FromGroup)
		assert.Equal(t, "群消息", msg.Content)
		assert.True(t, msg.IsGroup())
	case <-time.After(5 * time.Second):
		t.Fatal("超时等待群消息")
	}
}

func TestEventPayload_Unmarshal(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantID  string
	}{
		{"消息事件", `{"messageId":"msg_123"}`, "msg_123"},
		{"空 payload", `{}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ep eventPayload
			err := json.Unmarshal([]byte(tt.payload), &ep)
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, ep.MessageID)
		})
	}
}

func TestBackendSendText_WithMockGRPC(t *testing.T) {
	// 测试通过 mock gRPC 直接调用 SendText
	srv := &mockPuppetServer{
		loginPayload: `{"contactId":"wxid_self"}`,
	}
	addr, cleanup := startMockServer(t, srv)
	defer cleanup()

	// 直接构建连接
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	b := &Backend{
		cfg:    config.WechatyInstanceConfig{Endpoint: addr},
		conn:   conn,
		client: pb.NewPuppetClient(conn),
		logger: newTestLogger(),
	}
	b.loggedIn.Store(true)

	err = b.SendText(context.Background(), "wxid_test", "直接测试")
	require.NoError(t, err)

	require.Len(t, srv.sendTextCalls, 1)
	assert.Equal(t, "wxid_test", srv.sendTextCalls[0].ConversationID)
	assert.Equal(t, "直接测试", srv.sendTextCalls[0].Text)
}
