package eval

import (
	"context"
	"testing"

	"github.com/chef-guo/agents-hive/internal/memory"
	"go.uber.org/zap"
)

type fixtureStore struct {
	records []memory.MemoryRecord
}

func (s *fixtureStore) Save(context.Context, *memory.MemoryRecord) (int64, error) { return 0, nil }
func (s *fixtureStore) Get(_ context.Context, id int64) (*memory.MemoryRecord, error) {
	for i := range s.records {
		if s.records[i].ID == id {
			return &s.records[i], nil
		}
	}
	return nil, nil
}
func (s *fixtureStore) Update(context.Context, *memory.MemoryRecord) error { return nil }
func (s *fixtureStore) Delete(context.Context, int64) error                { return nil }
func (s *fixtureStore) Search(context.Context, memory.SearchOptions) (*memory.SearchResult, error) {
	return &memory.SearchResult{Memories: s.records, Total: len(s.records)}, nil
}
func (s *fixtureStore) List(context.Context, memory.SearchOptions) (*memory.SearchResult, error) {
	return &memory.SearchResult{Memories: s.records, Total: len(s.records)}, nil
}
func (s *fixtureStore) Stats(context.Context) (*memory.MemoryStats, error) {
	return &memory.MemoryStats{}, nil
}
func (s *fixtureStore) SetEmbedding(memory.EmbeddingProvider, memory.VectorStore) {}
func (s *fixtureStore) Close() error                                              { return nil }

func TestBuildRecordsAndAssertResult(t *testing.T) {
	c := Case{
		ID: "mc",
		Memories: []MemoryFixture{
			{ID: 1, UserID: "u1", Type: "user", Content: "可信", Confidence: 0.9},
		},
		WantInjectedIDs: []int64{1},
	}

	records, err := BuildRecords(c)
	if err != nil {
		t.Fatalf("BuildRecords returned error: %v", err)
	}
	if records[0].ID != 1 {
		t.Fatalf("record ID = %d, want 1", records[0].ID)
	}
	if err := AssertResult(c, memory.InjectionResult{
		Text:     "可信",
		Memories: []memory.InjectedMemory{{ID: 1, Type: memory.MemoryTypeUser}},
	}); err != nil {
		t.Fatalf("AssertResult returned error: %v", err)
	}
}

func TestAssertResultRejectsForbiddenText(t *testing.T) {
	err := AssertResult(Case{ID: "mc", ForbiddenText: []string{"secret"}}, memory.InjectionResult{Text: "secret"})
	if err == nil {
		t.Fatal("expected forbidden text error")
	}
}

func TestAssertResultRequiresSkippedIDEvidence(t *testing.T) {
	err := AssertResult(Case{ID: "mc", WantSkippedIDs: []int64{2}}, memory.InjectionResult{})
	if err == nil {
		t.Fatal("expected missing skipped id error")
	}
	err = AssertResult(Case{ID: "mc", WantSkippedIDs: []int64{2}}, memory.InjectionResult{
		SkippedMemoryIDs: []int64{2},
	})
	if err != nil {
		t.Fatalf("AssertResult returned error: %v", err)
	}
}

func TestFixturesRunThroughInjector(t *testing.T) {
	cases, err := LoadCases("testdata")
	if err != nil {
		t.Fatalf("LoadCases returned error: %v", err)
	}
	for _, loaded := range cases {
		if err := ValidateCase(loaded.Case); err != nil {
			t.Fatalf("ValidateCase(%s) returned error: %v", loaded.Path, err)
		}
		records, err := BuildRecords(loaded.Case)
		if err != nil {
			t.Fatalf("BuildRecords(%s) returned error: %v", loaded.Path, err)
		}
		inj := memory.NewInjector(&fixtureStore{records: records}, 2000, 10, zap.NewNop())
		got, err := inj.InjectContextDetailed(context.Background(), loaded.Case.Query, "s1", loaded.Case.UserID)
		if err != nil {
			t.Fatalf("InjectContextDetailed(%s) returned error: %v", loaded.Path, err)
		}
		if err := AssertResult(loaded.Case, got); err != nil {
			t.Fatalf("AssertResult(%s) returned error: %v", loaded.Path, err)
		}
	}
}
