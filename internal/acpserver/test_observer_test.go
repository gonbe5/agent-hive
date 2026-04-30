package acpserver

import (
	"context"
	"sync"

	"github.com/chef-guo/agents-hive/internal/tools"
)

type recordingACPObserver struct {
	mu     sync.Mutex
	events []tools.DelegationEvent
}

func (o *recordingACPObserver) RecordDelegation(_ context.Context, ev tools.DelegationEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, ev)
}

func (o *recordingACPObserver) snapshot() []tools.DelegationEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]tools.DelegationEvent, len(o.events))
	copy(out, o.events)
	return out
}
