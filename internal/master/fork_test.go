package master

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/skills"
)

func TestForkExecutor_NilFactory(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fork := NewForkExecutor(nil, logger)

	skill := &skills.Skill{
		Metadata: skills.SkillMetadata{
			Name:    "test-fork",
			Context: "fork",
		},
		Content: "test content",
		Loaded:  skills.LevelFullContent,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := fork.ExecuteForked(ctx, skill, skills.RenderContext{Arguments: "hello"}, nil)
	if err == nil {
		t.Fatal("expected error when factory is nil")
	}
	if !errs.IsCode(err, errs.CodeSkillForkFailed) {
		t.Errorf("expected CodeSkillForkFailed, got %v", err)
	}
}

func TestForkExecutor_ContentNotLoaded(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fork := NewForkExecutor(nil, logger)

	skill := &skills.Skill{
		Metadata: skills.SkillMetadata{
			Name:    "unloaded-fork",
			Context: "fork",
		},
		Path:   "/nonexistent/path",
		Loaded: skills.LevelMetadataOnly,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := fork.ExecuteForked(ctx, skill, skills.RenderContext{}, nil)
	if err == nil {
		t.Fatal("expected error for unloaded content")
	}
	if !errs.IsCode(err, errs.CodeSkillForkFailed) {
		t.Errorf("expected CodeSkillForkFailed, got %v", err)
	}
}
