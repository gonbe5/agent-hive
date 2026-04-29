package master

import (
	"testing"
	"time"
)

func TestSiblingAbortController_SignalAbort(t *testing.T) {
	ctrl := NewSiblingAbortController()

	// Subscribe first
	ch := ctrl.Subscribe()

	// Signal abort
	ctrl.SignalAbort()

	select {
	case <-ch:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("expected abort signal")
	}
}

func TestSiblingAbortController_MultipleAborts(t *testing.T) {
	ctrl := NewSiblingAbortController()
	ch := ctrl.Subscribe()

	// Multiple aborts should not block
	ctrl.SignalAbort()
	ctrl.SignalAbort()
	ctrl.SignalAbort()

	// Should only get one signal
	select {
	case <-ch:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("expected abort signal")
	}

	// Channel should have at most one signal
	select {
	case <-ch:
		t.Error("should only have one signal")
	default:
		// Expected
	}
}

func TestSiblingAbortController_Reset(t *testing.T) {
	ctrl := NewSiblingAbortController()

	// Subscribe and signal
	ch := ctrl.Subscribe()
	ctrl.SignalAbort()

	// Reset
	ctrl.Reset()

	// Subscribe again - should get a fresh start
	ch2 := ctrl.Subscribe()

	select {
	case <-ch2:
		t.Error("should not have signal after reset")
	default:
		// Expected
	}

	// Old channel still has the abort signal
	select {
	case <-ch:
		// Expected
	default:
		t.Error("old channel should still have abort signal")
	}
}

func TestSiblingAbortController_SubscribeBeforeActive(t *testing.T) {
	ctrl := NewSiblingAbortController()

	// Subscribe before any abort
	ch := ctrl.Subscribe()

	// Check channel is not closed initially
	select {
	case <-ch:
		t.Error("should not receive from inactive controller")
	default:
		// Expected
	}

	// Now signal abort
	ctrl.SignalAbort()

	select {
	case <-ch:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("expected abort signal after SignalAbort")
	}
}
