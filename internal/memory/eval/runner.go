package eval

import (
	"context"

	"github.com/chef-guo/agents-hive/internal/memory"
	"go.uber.org/zap"
)

// Result 是单条 memory/context fixture 的执行结果。
type Result struct {
	CaseID string `json:"case_id"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason,omitempty"`
}

// Summary 汇总 memory/context eval 执行结果，可被 CLI 或质量门禁消费。
type Summary struct {
	Total          int      `json:"total"`
	Passed         int      `json:"passed"`
	RequiredTotal  int      `json:"required_total"`
	RequiredPassed int      `json:"required_passed"`
	RequiredFailed []string `json:"required_failed,omitempty"`
	OptionalFailed []string `json:"optional_failed,omitempty"`
	Results        []Result `json:"results"`
}

// RunCases 加载并执行指定目录下的 memory/context eval fixtures。
func RunCases(ctx context.Context, dir string) (Summary, error) {
	loaded, err := LoadCases(dir)
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{
		Total:   len(loaded),
		Results: make([]Result, 0, len(loaded)),
	}
	for _, lc := range loaded {
		if lc.Case.Required {
			summary.RequiredTotal++
		}

		result := Result{CaseID: lc.Case.ID, Passed: true}
		if err := runCase(ctx, lc); err != nil {
			result.Passed = false
			result.Reason = err.Error()
			if lc.Case.Required {
				summary.RequiredFailed = append(summary.RequiredFailed, lc.Case.ID)
			} else {
				summary.OptionalFailed = append(summary.OptionalFailed, lc.Case.ID)
			}
		} else {
			summary.Passed++
			if lc.Case.Required {
				summary.RequiredPassed++
			}
		}
		summary.Results = append(summary.Results, result)
	}
	return summary, nil
}

func runCase(ctx context.Context, loaded LoadedCase) error {
	if err := ValidateCase(loaded.Case); err != nil {
		return err
	}
	records, err := BuildRecords(loaded.Case)
	if err != nil {
		return err
	}
	inj := memory.NewInjector(&caseStore{records: records}, 2000, 10, zap.NewNop())
	got, err := inj.InjectContextDetailed(ctx, loaded.Case.Query, "eval-session", loaded.Case.UserID)
	if err != nil {
		return err
	}
	return AssertResult(loaded.Case, got)
}

type caseStore struct {
	records []memory.MemoryRecord
}

func (s *caseStore) Save(context.Context, *memory.MemoryRecord) (int64, error) { return 0, nil }
func (s *caseStore) Get(_ context.Context, id int64) (*memory.MemoryRecord, error) {
	for i := range s.records {
		if s.records[i].ID == id {
			return &s.records[i], nil
		}
	}
	return nil, nil
}
func (s *caseStore) Update(context.Context, *memory.MemoryRecord) error { return nil }
func (s *caseStore) Delete(context.Context, int64) error                { return nil }
func (s *caseStore) Search(context.Context, memory.SearchOptions) (*memory.SearchResult, error) {
	return &memory.SearchResult{Memories: s.records, Total: len(s.records)}, nil
}
func (s *caseStore) List(context.Context, memory.SearchOptions) (*memory.SearchResult, error) {
	return &memory.SearchResult{Memories: s.records, Total: len(s.records)}, nil
}
func (s *caseStore) Stats(context.Context) (*memory.MemoryStats, error) {
	return &memory.MemoryStats{}, nil
}
func (s *caseStore) SetEmbedding(memory.EmbeddingProvider, memory.VectorStore) {}
func (s *caseStore) Close() error                                              { return nil }
