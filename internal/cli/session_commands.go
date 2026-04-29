package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/master"
)

// handleSessionCommand 处理 /session 系列命令
func (a *App) handleSessionCommand(ctx context.Context, cmd string) error {
	// 移除 "session " 前缀
	args := strings.TrimPrefix(cmd, "session ")
	if args == cmd {
		// 没有参数，显示帮助
		return a.showSessionHelp()
	}

	// 解析子命令
	parts := strings.SplitN(args, " ", 2)
	subCmd := strings.TrimSpace(parts[0])
	var cmdArgs []string
	if len(parts) > 1 {
		cmdArgs = strings.Fields(parts[1])
	}

	switch subCmd {
	case "new":
		return a.sessionNew(ctx, cmdArgs)
	case "list", "ls":
		return a.sessionList(ctx)
	case "switch", "sw":
		return a.sessionSwitch(ctx, cmdArgs)
	case "delete", "del", "rm":
		return a.sessionDelete(ctx, cmdArgs)
	case "rename", "mv":
		return a.sessionRename(ctx, cmdArgs)
	case "info":
		return a.sessionInfo(ctx)
	case "export":
		return a.sessionExport(ctx, cmdArgs)
	case "fork":
		return a.sessionFork(ctx, cmdArgs)
	case "revert":
		return a.sessionRevert(ctx, cmdArgs)
	default:
		return errs.New(errs.CodeInvalidInput, fmt.Sprintf("未知的会话子命令: %s (使用 /session 查看帮助)", subCmd))
	}
}

// showSessionHelp 显示会话命令帮助
func (a *App) showSessionHelp() error {
	fmt.Println("会话管理命令:")
	fmt.Println()
	fmt.Println("  /session new [name]       - 创建新会话")
	fmt.Println("  /session list             - 列出所有会话")
	fmt.Println("  /session switch <id>      - 切换到指定会话")
	fmt.Println("  /session delete <id>      - 删除指定会话")
	fmt.Println("  /session rename <name>    - 重命名当前会话")
	fmt.Println("  /session info             - 显示当前会话信息")
	fmt.Println("  /session export <path>    - 导出会话历史到文件")
	fmt.Println("  /session fork [name] [point] - 从当前会话创建分支")
	fmt.Println("  /session revert <index>   - 回滚会话到指定消息索引")
	fmt.Println()
	fmt.Println("简写:")
	fmt.Println("  list -> ls, switch -> sw, delete -> del/rm, rename -> mv")
	return nil
}

// sessionNew 创建新会话
func (a *App) sessionNew(ctx context.Context, args []string) error {
	sessionName := "新会话"
	if len(args) > 0 {
		sessionName = args[0]
	}

	// 发送创建会话命令
	select {
	case a.master.RequestCh() <- master.SessionRequest{
		Command: master.SessionCommandNew,
		Args:    []string{sessionName},
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// 等待响应
	select {
	case resp := <-a.master.ResponseCh():
		if resp.Error != "" {
			return errs.New(errs.CodeInternal, resp.Error)
		}
		fmt.Println(resp.Message)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sessionList 列出所有会话
func (a *App) sessionList(ctx context.Context) error {
	// 发送列表命令
	select {
	case a.master.RequestCh() <- master.SessionRequest{
		Command: master.SessionCommandList,
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// 等待响应
	select {
	case resp := <-a.master.ResponseCh():
		if resp.Error != "" {
			return errs.New(errs.CodeInternal, resp.Error)
		}
		fmt.Print(resp.Message)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sessionSwitch 切换会话
func (a *App) sessionSwitch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errs.New(errs.CodeInvalidInput, "需要指定会话 ID (使用 /session list 查看)")
	}

	targetID := args[0]

	// 发送切换命令
	select {
	case a.master.RequestCh() <- master.SessionRequest{
		Command: master.SessionCommandSwitch,
		Args:    []string{targetID},
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// 等待响应
	select {
	case resp := <-a.master.ResponseCh():
		if resp.Error != "" {
			return errs.New(errs.CodeInternal, resp.Error)
		}
		fmt.Println(resp.Message)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sessionDelete 删除会话
func (a *App) sessionDelete(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errs.New(errs.CodeInvalidInput, "需要指定会话 ID (使用 /session list 查看)")
	}

	targetID := args[0]

	// 发送删除命令
	select {
	case a.master.RequestCh() <- master.SessionRequest{
		Command: master.SessionCommandDelete,
		Args:    []string{targetID},
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// 等待响应
	select {
	case resp := <-a.master.ResponseCh():
		if resp.Error != "" {
			return errs.New(errs.CodeInternal, resp.Error)
		}
		fmt.Println(resp.Message)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sessionRename 重命名当前会话
func (a *App) sessionRename(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errs.New(errs.CodeInvalidInput, "需要指定新名称")
	}

	newName := strings.Join(args, " ")

	// 发送重命名命令
	select {
	case a.master.RequestCh() <- master.SessionRequest{
		Command: master.SessionCommandRename,
		Args:    []string{newName},
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// 等待响应
	select {
	case resp := <-a.master.ResponseCh():
		if resp.Error != "" {
			return errs.New(errs.CodeInternal, resp.Error)
		}
		fmt.Println(resp.Message)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sessionInfo 显示当前会话信息
func (a *App) sessionInfo(ctx context.Context) error {
	// 获取当前会话 ID
	sessionID, sessionName := a.master.GetCurrentSessionInfo()
	if sessionID == "" {
		fmt.Println("当前没有活跃会话")
		return nil
	}

	// 从 Master 获取完整会话详情
	session, err := a.master.GetSessionByID(ctx, sessionID)
	if err != nil {
		// 回退：显示基本信息
		fmt.Printf("会话 ID: %s\n", sessionID)
		fmt.Printf("会话名称: %s\n", sessionName)
		fmt.Printf("(无法加载完整详情: %v)\n", err)
		return nil
	}

	// 格式化输出完整的会话元数据
	fmt.Println("=== 会话信息 ===")
	fmt.Printf("ID:           %s\n", session.ID)
	fmt.Printf("名称:         %s\n", session.Name)
	fmt.Printf("消息数:       %d\n", session.MessageCount)
	fmt.Printf("总 Token:     %d\n", session.TotalTokens)
	fmt.Printf("创建时间:     %s\n", session.CreatedAt)
	fmt.Printf("更新时间:     %s\n", session.UpdatedAt)
	fmt.Printf("最后访问:     %s\n", session.LastAccessedAt)

	if len(session.Tags) > 0 {
		fmt.Printf("标签:         %v\n", session.Tags)
	}

	return nil
}

// sessionExport 导出会话历史
func (a *App) sessionExport(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errs.New(errs.CodeInvalidInput, "需要指定导出文件路径")
	}

	exportPath := args[0]

	// 获取当前会话信息
	info, err := a.getCurrentSessionInfo(ctx)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "获取会话信息失败", err)
	}

	// 根据文件扩展名选择格式
	var content []byte
	if strings.HasSuffix(exportPath, ".json") {
		// JSON 格式
		content, err = json.MarshalIndent(info, "", "  ")
		if err != nil {
			return errs.Wrap(errs.CodeInternal, "序列化 JSON 失败", err)
		}
	} else {
		// Markdown 格式（默认）
		content = []byte(a.formatSessionAsMarkdown(info))
	}

	// 写入文件
	if err := os.WriteFile(exportPath, content, 0644); err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "写入文件失败", err)
	}

	fmt.Printf("会话已导出到: %s\n", exportPath)
	return nil
}

// getCurrentSessionInfo 获取当前会话的详细信息
func (a *App) getCurrentSessionInfo(ctx context.Context) (*master.SessionInfo, error) {
	sessionID, sessionName := a.master.GetCurrentSessionInfo()
	if sessionID == "" {
		return nil, errs.New(errs.CodeTaskNotFound, "当前没有活跃会话")
	}

	// 从 Master 获取完整会话记录
	record, err := a.master.GetSessionByID(ctx, sessionID)
	if err != nil {
		// 降级：返回基本信息
		return &master.SessionInfo{
			ID:       sessionID,
			Name:     sessionName,
			IsActive: true,
		}, nil
	}

	lastAccessed, _ := time.Parse(time.RFC3339, record.LastAccessedAt)
	return &master.SessionInfo{
		ID:           record.ID,
		Name:         record.Name,
		MessageCount: record.MessageCount,
		LastAccessed: lastAccessed,
		Tags:         record.Tags,
		IsActive:     true,
	}, nil
}

// formatSessionAsMarkdown 将会话格式化为 Markdown
func (a *App) formatSessionAsMarkdown(info *master.SessionInfo) string {
	var sb strings.Builder

	sb.WriteString("# 会话导出\n\n")
	sb.WriteString(fmt.Sprintf("**会话 ID**: %s\n", info.ID))
	sb.WriteString(fmt.Sprintf("**会话名称**: %s\n", info.Name))
	sb.WriteString(fmt.Sprintf("**消息数量**: %d\n", info.MessageCount))
	sb.WriteString(fmt.Sprintf("**最后访问**: %s\n\n", info.LastAccessed.Format(time.RFC3339)))

	sb.WriteString("## 标签\n\n")
	if len(info.Tags) > 0 {
		for _, tag := range info.Tags {
			sb.WriteString(fmt.Sprintf("- %s\n", tag))
		}
	} else {
		sb.WriteString("（无标签）\n")
	}

	sb.WriteString("\n---\n")
	sb.WriteString(fmt.Sprintf("*导出时间: %s*\n", time.Now().Format(time.RFC3339)))

	return sb.String()
}

// sessionFork 创建会话分支
func (a *App) sessionFork(ctx context.Context, args []string) error {
	// 发送 fork 命令
	select {
	case a.master.RequestCh() <- master.SessionRequest{
		Command: master.SessionCommandFork,
		Args:    args, // args[0]=fork_name (可选), args[1]=fork_point (可选)
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// 等待响应
	select {
	case resp := <-a.master.ResponseCh():
		if resp.Error != "" {
			return errs.New(errs.CodeInternal, resp.Error)
		}
		fmt.Println(resp.Message)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sessionRevert 回滚会话到指定消息索引
func (a *App) sessionRevert(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errs.New(errs.CodeInvalidInput, "需要指定目标消息索引")
	}

	// 发送 revert 命令
	select {
	case a.master.RequestCh() <- master.SessionRequest{
		Command: master.SessionCommandRevert,
		Args:    args, // args[0]=revert_to
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// 等待响应
	select {
	case resp := <-a.master.ResponseCh():
		if resp.Error != "" {
			return errs.New(errs.CodeInternal, resp.Error)
		}
		fmt.Println(resp.Message)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
