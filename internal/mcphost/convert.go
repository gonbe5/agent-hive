package mcphost

import (
	"encoding/json"
	"fmt"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// OpenAITool represents an OpenAI function tool definition.
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

// OpenAIFunction represents the function inside an OpenAI tool.
type OpenAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// MCPToOpenAI converts MCP tool definitions to OpenAI function tool format.
func MCPToOpenAI(mcpTools []ToolDefinition) []OpenAITool {
	openAITools := make([]OpenAITool, len(mcpTools))
	for i, mcp := range mcpTools {
		openAITools[i] = OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        mcp.Name,
				Description: mcp.Description,
				Parameters:  mcp.InputSchema,
			},
		}
	}
	return openAITools
}

// OpenAIToMCP converts OpenAI function tool definitions to MCP tool format.
func OpenAIToMCP(openAITools []OpenAITool) []ToolDefinition {
	mcpTools := make([]ToolDefinition, len(openAITools))
	for i, oai := range openAITools {
		mcpTools[i] = ToolDefinition{
			Name:        oai.Function.Name,
			Description: oai.Function.Description,
			InputSchema: oai.Function.Parameters,
		}
	}
	return mcpTools
}

// ConvertTools converts between MCP and OpenAI tool formats.
// direction must be "mcp_to_openai" or "openai_to_mcp".
func ConvertTools(direction string, tools json.RawMessage) (json.RawMessage, error) {
	switch direction {
	case "mcp_to_openai":
		var mcpTools []ToolDefinition
		if err := json.Unmarshal(tools, &mcpTools); err != nil {
			return nil, errs.Wrap(errs.CodeInvalidInput, "invalid MCP tools", err)
		}
		return json.Marshal(MCPToOpenAI(mcpTools))
	case "openai_to_mcp":
		var openAITools []OpenAITool
		if err := json.Unmarshal(tools, &openAITools); err != nil {
			return nil, errs.Wrap(errs.CodeInvalidInput, "invalid OpenAI tools", err)
		}
		return json.Marshal(OpenAIToMCP(openAITools))
	default:
		return nil, errs.New(errs.CodeInvalidInput, fmt.Sprintf("unknown direction: %s", direction))
	}
}
