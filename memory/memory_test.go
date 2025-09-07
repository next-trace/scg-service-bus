package memory

import (
	"context"
	"testing"
)

type testCmd struct{}

type testQry struct{}

type testEvt struct{}

func TestNewMemoryBus_BasicFlow(t *testing.T) {
	b, cleanup := New()
	defer cleanup()

	ctx := context.Background()

	// Bind and dispatch a command
	cmdCount := 0
	if err := b.BindCommandOf(testCmd{}, func(ctx context.Context, v any) error {
		cmdCount++
		return nil
	}); err != nil {
		t.Fatalf("bind command: %v", err)
	}
	if err := b.Dispatch(ctx, testCmd{}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if cmdCount != 1 {
		t.Fatalf("expected cmdCount=1 got %d", cmdCount)
	}

	// Bind and ask a query
	if err := b.BindQueryOf(testQry{}, func(ctx context.Context, v any) (any, error) {
		return "ok", nil
	}); err != nil {
		t.Fatalf("bind query: %v", err)
	}
	res, err := b.Ask(ctx, testQry{})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if s, ok := res.(string); !ok || s != "ok" {
		t.Fatalf("unexpected query result: %#v", res)
	}

	// Bind and publish a domain event
	evtCount := 0
	if err := b.BindDomainEventOf(testEvt{}, func(ctx context.Context, v any) error {
		evtCount++
		return nil
	}); err != nil {
		t.Fatalf("bind event: %v", err)
	}
	if err := b.PublishDomain(ctx, testEvt{}); err != nil {
		t.Fatalf("publish domain: %v", err)
	}
	if evtCount != 1 {
		t.Fatalf("expected evtCount=1 got %d", evtCount)
	}

	// Close should be a no-op
	if err := b.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
