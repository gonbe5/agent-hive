package journal

import (
	"context"
	"time"
)

// NoopJournal 空实现，零开销，向后兼容。
type NoopJournal struct{}

func (NoopJournal) StartSession(context.Context, string, string) error { return nil }
func (NoopJournal) LogToolCall(context.Context, ToolCallEntry) error  { return nil }
func (NoopJournal) LogFileChange(context.Context, FileChangeEntry) error {
	return nil
}
func (NoopJournal) LogDecision(context.Context, DecisionEntry) error { return nil }
func (NoopJournal) EndSession(context.Context, string, string) error { return nil }
func (NoopJournal) GetJournal(context.Context, string, int) (*SessionJournal, error) {
	return nil, nil
}
func (NoopJournal) DeleteSession(context.Context, string) error { return nil }
func (NoopJournal) GetJournalEvents(context.Context, string, int, time.Time) ([]JournalEvent, error) {
	return nil, ErrJournalNotAvailable
}
func (NoopJournal) GetJournalStats(context.Context, []string) (map[string]*JournalStats, error) {
	return nil, ErrJournalNotAvailable
}
