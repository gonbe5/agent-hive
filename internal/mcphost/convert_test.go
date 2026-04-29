package mcphost

import (
	"encoding/json"
	"testing"
)

func TestMCPToOpenAI(t *testing.T) {
	mcpTools := []ToolDefinition{
		{
			Name:        "read_file",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	}

	result := MCPToOpenAI(mcpTools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Type != "function" {
		t.Errorf("expected type function, got %s", result[0].Type)
	}
	if result[0].Function.Name != "read_file" {
		t.Errorf("expected name read_file, got %s", result[0].Function.Name)
	}
	if result[0].Function.Description != "Read a file" {
		t.Errorf("expected description 'Read a file', got %s", result[0].Function.Description)
	}
}

func TestOpenAIToMCP(t *testing.T) {
	openAITools := []OpenAITool{
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "search",
				Description: "Search the web",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		},
	}

	result := OpenAIToMCP(openAITools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Name != "search" {
		t.Errorf("expected name search, got %s", result[0].Name)
	}
	if result[0].Description != "Search the web" {
		t.Errorf("expected description 'Search the web', got %s", result[0].Description)
	}
}

func TestConvertTools_MCPToOpenAI(t *testing.T) {
	mcpTools := []ToolDefinition{
		{
			Name:        "read_file",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	}
	toolsJSON, _ := json.Marshal(mcpTools)

	result, err := ConvertTools("mcp_to_openai", toolsJSON)
	if err != nil {
		t.Fatalf("ConvertTools returned error: %v", err)
	}

	var openAITools []OpenAITool
	if err := json.Unmarshal(result, &openAITools); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(openAITools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(openAITools))
	}
	if openAITools[0].Function.Name != "read_file" {
		t.Errorf("expected name read_file, got %s", openAITools[0].Function.Name)
	}
}

func TestConvertTools_OpenAIToMCP(t *testing.T) {
	openAITools := []OpenAITool{
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "search",
				Description: "Search the web",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		},
	}
	toolsJSON, _ := json.Marshal(openAITools)

	result, err := ConvertTools("openai_to_mcp", toolsJSON)
	if err != nil {
		t.Fatalf("ConvertTools returned error: %v", err)
	}

	var mcpTools []ToolDefinition
	if err := json.Unmarshal(result, &mcpTools); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(mcpTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(mcpTools))
	}
	if mcpTools[0].Name != "search" {
		t.Errorf("expected name search, got %s", mcpTools[0].Name)
	}
}

func TestConvertTools_InvalidDirection(t *testing.T) {
	_, err := ConvertTools("invalid", json.RawMessage(`[]`))
	if err == nil {
		t.Fatal("expected error for invalid direction")
	}
}

func TestConvertTools_InvalidInput(t *testing.T) {
	_, err := ConvertTools("mcp_to_openai", json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
