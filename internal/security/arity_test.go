package security

import "testing"

func TestFormatPermissionPrompt(t *testing.T) {
	tests := []struct {
		name    string
		rawCmd  string
		want    string
	}{
		{
			"npm_install_包管理器",
			"npm install lodash",
			"npm install lodash [包管理器·低风险]",
		},
		{
			"npm_test_只读",
			"npm test",
			"npm test [包管理器·只读]",
		},
		{
			"git_status_只读",
			"git status",
			"git status [版本控制·只读]",
		},
		{
			"git_push_中风险",
			"git push origin main",
			"git push origin main [版本控制·中风险]",
		},
		{
			"git_rebase_高风险",
			"git rebase main",
			"git rebase main [版本控制·高风险]",
		},
		{
			"rm_rf_危险",
			"rm -rf /tmp/test",
			"rm -rf /tmp/test [危险操作·高风险]",
		},
		{
			"rm_普通删除",
			"rm file.txt",
			"rm file.txt [文件系统·高风险]",
		},
		{
			"ls_只读",
			"ls -la",
			"ls -la [文件系统·只读]",
		},
		{
			"cat_只读",
			"cat /etc/passwd",
			"cat /etc/passwd [文件系统·只读]",
		},
		{
			"curl_网络",
			"curl -s https://example.com",
			"curl -s https://example.com [网络·中风险]",
		},
		{
			"ssh_高风险",
			"ssh user@host",
			"ssh user@host [网络·高风险]",
		},
		{
			"go_test_只读",
			"go test ./...",
			"go test ./... [构建·只读]",
		},
		{
			"go_build_构建",
			"go build -o app .",
			"go build -o app . [构建·低风险]",
		},
		{
			"docker_run_中风险",
			"docker run -it ubuntu bash",
			"docker run -it ubuntu bash [构建·中风险]",
		},
		{
			"make_构建",
			"make all",
			"make all [构建·低风险]",
		},
		{
			"sudo_危险",
			"sudo apt install nginx",
			"sudo apt install nginx [危险操作·高风险]",
		},
		{
			"grep_只读",
			"grep -r pattern .",
			"grep -r pattern . [文件系统·只读]",
		},
		{
			"vim_编辑器",
			"vim main.go",
			"vim main.go [编辑器·低风险]",
		},
		{
			"未知命令_原样返回",
			"my-custom-tool --flag value",
			"my-custom-tool --flag value",
		},
		{
			"空命令",
			"",
			"",
		},
		{
			"仅空格",
			"   ",
			"",
		},
		{
			"pip_install",
			"pip install requests",
			"pip install requests [包管理器·低风险]",
		},
		{
			"find_只读",
			"find . -name '*.go'",
			"find . -name '*.go' [文件系统·只读]",
		},
		{
			"mv_中风险",
			"mv old.txt new.txt",
			"mv old.txt new.txt [文件系统·中风险]",
		},
		{
			"git_commit",
			"git commit -m 'initial'",
			"git commit -m 'initial' [版本控制·低风险]",
		},
		{
			"git_clone",
			"git clone https://github.com/user/repo",
			"git clone https://github.com/user/repo [版本控制·低风险]",
		},
		{
			"cargo_test_只读",
			"cargo test --release",
			"cargo test --release [构建·只读]",
		},
		{
			"chmod_中风险",
			"chmod 755 script.sh",
			"chmod 755 script.sh [文件系统·中风险]",
		},
		{
			"sed_中风险",
			"sed -i 's/old/new/g' file.txt",
			"sed -i 's/old/new/g' file.txt [文件系统·中风险]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPermissionPrompt(tt.rawCmd)
			if got != tt.want {
				t.Errorf("FormatPermissionPrompt(%q) = %q, 期望 %q", tt.rawCmd, got, tt.want)
			}
		})
	}
}

func TestArityTable_覆盖率(t *testing.T) {
	// 确保所有分类都有对应的中文标签
	categories := map[string]bool{}
	for _, arity := range arityTable {
		categories[arity.Category] = true
	}

	for cat := range categories {
		if _, ok := categoryLabels[cat]; !ok {
			t.Errorf("分类 %q 缺少中文标签", cat)
		}
	}

	// 确保所有风险等级都有对应的中文标签
	risks := map[string]bool{}
	for _, arity := range arityTable {
		risks[arity.Risk] = true
	}

	for risk := range risks {
		if _, ok := riskLabels[risk]; !ok {
			t.Errorf("风险等级 %q 缺少中文标签", risk)
		}
	}
}

func TestFormatPermissionPrompt_最长前缀匹配(t *testing.T) {
	// 确保 "git status" 匹配 "git status" 而非仅 "git"
	got := FormatPermissionPrompt("git status --short")
	want := "git status --short [版本控制·只读]"
	if got != want {
		t.Errorf("最长前缀匹配失败: got %q, 期望 %q", got, want)
	}

	// "go test" 应匹配 "go test" 而非 "go"（go 不在 arityTable 中，但 go test 在）
	got = FormatPermissionPrompt("go test -v ./internal/...")
	want = "go test -v ./internal/... [构建·只读]"
	if got != want {
		t.Errorf("go test 前缀匹配失败: got %q, 期望 %q", got, want)
	}
}
