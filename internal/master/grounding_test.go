package master

import (
	"context"
	"errors"
	"testing"

	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildToolEvidence_ExtractsURLsFromToolMessages(t *testing.T) {
	messages := []llm.MessageWithTools{
		{
			Role:     "tool",
			ToolName: "websearch",
			Content:  llm.NewTextContent(`结果: https://example.com/docs 和 https://example.com/docs#section`),
		},
		{
			Role:     "tool",
			ToolName: "read_file",
			Content:  llm.NewTextContent("本地文件没有 URL"),
		},
	}

	evidence := BuildToolEvidence(messages)

	require.Len(t, evidence.Sources, 2)
	assert.Equal(t, "https://example.com/docs", evidence.Sources[0].URL)
	assert.Equal(t, "websearch", evidence.Sources[0].Tool)
}

func TestBuildToolEvidence_ExtractsStructuredSourcesFromToolJSON(t *testing.T) {
	messages := []llm.MessageWithTools{
		{
			Role:     "tool",
			ToolName: "websearch",
			Content: llm.NewTextContent(`{
				"query": "agents hive grounding",
				"raw_count": 3,
				"filtered_count": 1,
				"sources": [
					{
						"title": "Grounding Plan",
						"url": "https://example.com/docs?utm=1#grounding",
						"snippet": "Validator schema",
						"span": {"start": 10, "end": 28}
					}
				]
			}`),
		},
	}

	evidence := BuildToolEvidence(messages)

	require.Len(t, evidence.Sources, 1)
	assert.Equal(t, "https://example.com/docs?utm=1#grounding", evidence.Sources[0].URL)
	assert.Equal(t, "websearch", evidence.Sources[0].Tool)
	assert.Equal(t, "Grounding Plan", evidence.Sources[0].Title)
	assert.Equal(t, "Validator schema", evidence.Sources[0].Snippet)
	assert.Equal(t, 10, evidence.Sources[0].Span.Start)
	assert.Equal(t, 28, evidence.Sources[0].Span.End)
	assert.Equal(t, "agents hive grounding", evidence.Query)
	assert.Equal(t, 3, evidence.RawCount)
	assert.Equal(t, 1, evidence.FilteredCount)
}

func TestGroundingValidator_BlocksUnverifiedURL(t *testing.T) {
	validator := GroundingValidator{}
	state := &AgentState{
		Response: &llm.ChatWithToolsResponse{
			Content: "参考 https://evil.example/fake",
		},
		Evidence: ToolEvidence{
			Sources: []EvidenceSource{{URL: "https://example.com/docs", Tool: "websearch"}},
		},
	}

	err := validator.AfterModel(context.Background(), state)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unverified URL")
}

func TestGroundingValidator_AllowsEvidenceURL(t *testing.T) {
	validator := GroundingValidator{}
	state := &AgentState{
		Response: &llm.ChatWithToolsResponse{
			Content: "参考 https://example.com/docs#section",
		},
		Evidence: ToolEvidence{
			Sources: []EvidenceSource{{URL: "https://example.com/docs", Tool: "websearch"}},
		},
	}

	require.NoError(t, validator.AfterModel(context.Background(), state))
}

func TestGroundingValidator_AllowsOrdinaryAnswerWithoutURLOrCitation(t *testing.T) {
	validator := GroundingValidator{}
	state := &AgentState{
		Response: &llm.ChatWithToolsResponse{
			Content: "这是一个普通总结，没有外部链接，也没有声称来源。",
		},
	}

	require.NoError(t, validator.AfterModel(context.Background(), state))
}

func TestGroundingValidator_BlocksCitationWithoutEvidence(t *testing.T) {
	validator := GroundingValidator{}
	state := &AgentState{
		Response: &llm.ChatWithToolsResponse{
			Content: "结论来自 [citation:Grounding](https://example.com/docs)。",
		},
	}

	err := validator.AfterModel(context.Background(), state)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "citation without evidence")
}

func TestGroundingValidator_BlocksMalformedSourceSpan(t *testing.T) {
	validator := GroundingValidator{}
	state := &AgentState{
		Response: &llm.ChatWithToolsResponse{
			Content: `{"answer":"结论","citations":[{"url":"https://example.com/docs","source_span":{"start":30,"end":10}}]}`,
		},
		Evidence: ToolEvidence{
			Sources: []EvidenceSource{{URL: "https://example.com/docs", Tool: "websearch"}},
		},
	}

	err := validator.AfterModel(context.Background(), state)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid source span")
}

type failingAfterModelMiddleware struct{}

func (failingAfterModelMiddleware) AfterModel(context.Context, *AgentState) error {
	return errors.New("blocked")
}

func TestMiddlewarePipeline_AfterModelStopsOnError(t *testing.T) {
	pipeline := NewMiddlewarePipeline(failingAfterModelMiddleware{})
	err := pipeline.AfterModel(context.Background(), &AgentState{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}
