package starlings

import (
	"context"
	"errors"
	"testing"
)

type extensionTestKey struct{}

type closingExtension struct{ closed bool }

func (e *closingExtension) Close(context.Context) { e.closed = true }

func TestExtensionCreatesOnceAndClosesWithClient(t *testing.T) {
	c := New("token")
	calls := 0
	create := func() (any, error) {
		calls++
		return &closingExtension{}, nil
	}
	first, err := c.Extension(extensionTestKey{}, create)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Extension(extensionTestKey{}, create)
	if err != nil || first != second || calls != 1 {
		t.Fatalf("second lookup = %v, %v after %d creates; want the first value from one create", second, err, calls)
	}
	_ = c.Close()
	if !first.(*closingExtension).closed {
		t.Fatal("Close did not close the extension")
	}
}

func TestExtensionRetriesAfterCreateError(t *testing.T) {
	c := New("token")
	failure := errors.New("not yet")
	if _, err := c.Extension(extensionTestKey{}, func() (any, error) { return nil, failure }); !errors.Is(err, failure) {
		t.Fatalf("create error = %v, want %v", err, failure)
	}
	v, err := c.Extension(extensionTestKey{}, func() (any, error) { return 7, nil })
	if err != nil || v != 7 {
		t.Fatalf("retry = %v, %v; want 7", v, err)
	}
}

func TestOnOrderedRunsBeforeApplicationHandlersWithAsyncEvents(t *testing.T) {
	c := New("token", WithAsyncEvents(true))
	On(c, func(*VoiceStateUpdate) {})
	before := 0
	if slot := c.slotFor("VOICE_STATE_UPDATE"); slot != nil {
		before = slot.internal
	}
	OnOrdered(c, func(*VoiceStateUpdate) {})
	slot := c.slotFor("VOICE_STATE_UPDATE")
	if slot == nil || slot.internal != before+1 {
		t.Fatalf("ordered handlers = %v, want %d", slot, before+1)
	}
	if len(slot.handlers) != slot.internal+1 {
		t.Fatalf("application handler was dropped: %d handlers, %d ordered", len(slot.handlers), slot.internal)
	}
}
