package push

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/chef-guo/agents-hive/internal/channel"
)

type templateSpec struct {
	msgType channel.MsgType
	body    string
}

var builtInTemplates = map[string]templateSpec{
	"task_done": {
		msgType: channel.MsgTypeMarkdown,
		body:    "## {{.title}}\n{{.summary}}",
	},
	"daily_report": {
		msgType: channel.MsgTypeMarkdown,
		body:    "## {{.title}}\n- 日期: {{.date}}\n- 摘要: {{.summary}}",
	},
}

func renderBuiltInTemplate(name string, vars map[string]any) (channel.MsgType, string, error) {
	spec, ok := builtInTemplates[name]
	if !ok {
		return "", "", fmt.Errorf("push template not found: %s", name)
	}
	tpl, err := template.New(name).Option("missingkey=error").Parse(spec.body)
	if err != nil {
		return "", "", fmt.Errorf("parse push template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, vars); err != nil {
		return "", "", fmt.Errorf("render push template %s: %w", name, err)
	}
	return spec.msgType, buf.String(), nil
}
