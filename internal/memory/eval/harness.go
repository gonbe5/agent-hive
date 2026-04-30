package eval

import (
	"fmt"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/memory"
)

// BuildRecords 把 fixture 转成真实 memory.MemoryRecord，供 Injector 直接运行。
func BuildRecords(c Case) ([]memory.MemoryRecord, error) {
	records := make([]memory.MemoryRecord, 0, len(c.Memories))
	for _, fixture := range c.Memories {
		mt := memory.MemoryType(fixture.Type)
		if !memory.ValidMemoryTypes[mt] {
			return nil, fmt.Errorf("%s: memory %d type invalid: %s", c.ID, fixture.ID, fixture.Type)
		}

		g := memory.Governance{
			Confidence: fixture.Confidence,
			Source:     fixture.Source,
		}
		if fixture.ExpiresAt != "" {
			ts, err := time.Parse(time.RFC3339, fixture.ExpiresAt)
			if err != nil {
				return nil, fmt.Errorf("%s: memory %d expires_at invalid: %w", c.ID, fixture.ID, err)
			}
			g.ExpiresAt = ts
		}

		records = append(records, memory.MemoryRecord{
			ID:       fixture.ID,
			UserID:   fixture.UserID,
			Type:     mt,
			Content:  fixture.Content,
			Metadata: memory.EncodeGovernance(nil, g),
		})
	}
	return records, nil
}

// AssertResult 校验注入结果满足 fixture 期望。
func AssertResult(c Case, result memory.InjectionResult) error {
	injected := map[int64]bool{}
	for _, mem := range result.Memories {
		injected[mem.ID] = true
	}
	skipped := map[int64]bool{}
	for _, id := range result.SkippedMemoryIDs {
		skipped[id] = true
	}

	for _, id := range c.WantInjectedIDs {
		if !injected[id] {
			return fmt.Errorf("%s: expected memory %d injected", c.ID, id)
		}
	}
	for _, id := range c.WantSkippedIDs {
		if injected[id] {
			return fmt.Errorf("%s: expected memory %d skipped", c.ID, id)
		}
		if !skipped[id] {
			return fmt.Errorf("%s: expected memory %d recorded as skipped", c.ID, id)
		}
	}
	for _, text := range c.ForbiddenText {
		if strings.Contains(result.Text, text) {
			return fmt.Errorf("%s: forbidden text %q injected", c.ID, text)
		}
	}
	return nil
}
