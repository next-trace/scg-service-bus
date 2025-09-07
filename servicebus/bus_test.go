package servicebus_test

import (
	"context"
	"errors"
	"testing"
	"time"

	cbus "github.com/next-trace/scg-service-bus/contract/bus"
	berr "github.com/next-trace/scg-service-bus/contract/errors"
	"github.com/next-trace/scg-service-bus/servicebus"
)

type testCmd struct{ ID string }

type testQry struct{ ID string }

type testRes struct{ ID string }

type testDom struct{ ID string }

type queueCmd struct{ ID string }

func (queueCmd) QueueName() string    { return "commands" }
func (queueCmd) Delay() time.Duration { return time.Second }

type qListener struct{}

func (qListener) Handle(ctx context.Context, e testDom) error { return nil }
func (qListener) QueueName() string                           { return "listeners" }
func (qListener) Delay() time.Duration                        { return 2 * time.Second }

// fakes

type fakeEnq struct {
	cmds         []any
	listeners    []string
	cmdOpts      []cbus.QueueOptions
	listenerOpts []cbus.QueueOptions
}

func (f *fakeEnq) EnqueueCommand(ctx context.Context, cmd cbus.Command, opts cbus.QueueOptions) error {
	f.cmds = append(f.cmds, cmd)
	f.cmdOpts = append(f.cmdOpts, opts)

	return nil
}

func (f *fakeEnq) EnqueueListener(
	ctx context.Context,
	evt cbus.DomainEvent,
	handler string,
	opts cbus.QueueOptions,
) error {
	f.listeners = append(f.listeners, handler)
	f.listenerOpts = append(f.listenerOpts, opts)

	return nil
}

type testOut struct{ T string }

func (o testOut) Topic() string { return o.T }

type fakePub struct {
	events []cbus.IntegrationEvent
	opts   []cbus.PublishOptions
}

func (f *fakePub) PublishIntegration(ctx context.Context, e cbus.IntegrationEvent, opts cbus.PublishOptions) error {
	f.events = append(f.events, e)
	f.opts = append(f.opts, opts)

	return nil
}

func Test_BindAndErrors(t *testing.T) {
	b := servicebus.New(nil, nil, nil)
	if err := b.BindCommandOf(testCmd{}, func(ctx context.Context, v any) error { return nil }); err != nil {
		t.Fatalf("bind cmd: %v", err)
	}

	err := b.BindCommandOf(testCmd{}, func(ctx context.Context, v any) error { return nil })
	if !errors.Is(err, berr.ErrHandlerExists) {
		t.Fatalf("want ErrHandlerExists, got %v", err)
	}

	if err := b.DispatchSync(t.Context(), struct{ X int }{1}); !errors.Is(err, berr.ErrHandlerNotFound) {
		t.Fatalf("want ErrHandlerNotFound, got %v", err)
	}

	b2 := servicebus.New(nil, nil, nil)
	_ = b2.BindQueryOf(testQry{}, func(ctx context.Context, q any) (any, error) { return 1, nil })

	_, err = servicebus.Ask[testQry, testRes](t.Context(), b2, testQry{ID: "g1"})
	if !errors.Is(err, berr.ErrHandlerTypeMismatch) {
		t.Fatalf("want ErrHandlerTypeMismatch, got %v", err)
	}
}

func Test_DispatchSync_Ask_PublishDomain(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	var seen []testCmd

	_ = b.BindCommandOf(testCmd{}, func(ctx context.Context, v any) error {
		seen = append(seen, v.(testCmd))
		return nil
	})

	if err := b.DispatchSync(t.Context(), testCmd{ID: "c1"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(seen) != 1 || seen[0].ID != "c1" {
		t.Fatalf("seen=%v", seen)
	}

	_ = b.BindQueryOf(testQry{}, func(ctx context.Context, q any) (any, error) {
		return testRes{ID: q.(testQry).ID}, nil
	})

	raw, err := b.Ask(t.Context(), testQry{ID: "g1"})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}

	if raw.(testRes).ID != "g1" {
		t.Fatalf("bad res: %+v", raw)
	}

	calls := 0
	_ = b.BindDomainEventOf(testDom{}, func(ctx context.Context, e any) error {
		calls++
		return nil
	})

	if err := b.PublishDomain(t.Context(), testDom{ID: "d1"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func Test_QueueablePaths(t *testing.T) {
	enq := &fakeEnq{}
	b := servicebus.New(enq, nil, nil)

	if err := b.Dispatch(t.Context(), queueCmd{ID: "a1"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(enq.cmds) != 1 {
		t.Fatalf("want 1 enqueued cmd, got %d", len(enq.cmds))
	}

	if err := servicebus.BindDomainEvent[testDom](b, qListener{}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	if err := b.PublishDomain(t.Context(), testDom{ID: "d1"}); err != nil {
		t.Fatalf("publish dom: %v", err)
	}

	if len(enq.listeners) != 1 {
		t.Fatalf("want 1 enqueued listener, got %d", len(enq.listeners))
	}
}

func Test_PublishIntegrationErrors(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	err := b.PublishIntegration(t.Context(), testOut{T: "orders"}, cbus.PublishOptions{Key: "k"})
	if !errors.Is(err, berr.ErrAsyncNotConfigured) {
		t.Fatalf("want ErrAsyncNotConfigured, got %v", err)
	}

	pub := &fakePub{}

	b = servicebus.New(nil, pub, nil)

	err = b.PublishIntegration(t.Context(), testOut{T: "orders"}, cbus.PublishOptions{Key: "k"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	if len(pub.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(pub.events))
	}
}

// CONSOLIDATED TESTS BELOW
// Note: This file consolidates former facade_bus_test.go, more_bus_test.go, and untyped_bus_test.go
// into a single bus_test.go for the servicebus package.

// ---- consolidated from facade_bus_test.go ----

type fCmd struct{ ID string }

type fQry struct{ K string }

type fRes struct{ V string }

type fDom struct{ N string }

type qlRaw struct{}

func (qlRaw) Handle(ctx context.Context, e fDom) error { return nil }
func (qlRaw) QueueName() string                        { return "fdom" }
func (qlRaw) Delay() time.Duration                     { return 0 }

type fakeEnq2 struct{ listeners []string }

func (f *fakeEnq2) EnqueueCommand(ctx context.Context, cmd cbus.Command, opts cbus.QueueOptions) error {
	return nil
}

func (f *fakeEnq2) EnqueueListener(
	ctx context.Context,
	evt cbus.DomainEvent,
	handler string,
	opts cbus.QueueOptions,
) error {
	f.listeners = append(f.listeners, handler)
	return nil
}

type fakePub2 struct{}

func (fakePub2) PublishIntegration(ctx context.Context, e cbus.IntegrationEvent, opts cbus.PublishOptions) error {
	return nil
}

func Test_CommandBus_QueryBus_And_AskGeneric(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	// bind command
	_ = b.BindCommandOf(fCmd{}, func(ctx context.Context, v any) error { return nil })

	// use CommandBus Dispatch and DispatchNow
	cb := servicebus.NewCommandBus(b)
	if err := cb.Dispatch(t.Context(), fCmd{ID: "1"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if err := cb.DispatchNow(t.Context(), fCmd{ID: "2"}); err != nil {
		t.Fatalf("dispatch now: %v", err)
	}

	// bind query and use QueryBus + AskGeneric
	_ = b.BindQueryOf(fQry{}, func(ctx context.Context, q any) (any, error) { return fRes{V: q.(fQry).K}, nil })

	qb := servicebus.NewQueryBus(b)

	anyRes, err := qb.Ask(t.Context(), fQry{K: "k"})
	if err != nil || anyRes.(fRes).V != "k" {
		t.Fatalf("ask via qb: %v res=%+v", err, anyRes)
	}

	r, err := servicebus.AskGeneric[fQry, fRes](t.Context(), qb, fQry{K: "g"})
	if err != nil || r.V != "g" {
		t.Fatalf("ask generic: %v r=%+v", err, r)
	}
}

func Test_BindDomainEventRaw_EnqueueAndSync(t *testing.T) {
	// Enqueue path
	enq := &fakeEnq2{}
	b := servicebus.New(enq, fakePub2{}, nil)

	_ = b.BindDomainEventRaw(fDom{}, qlRaw{}, func(ctx context.Context, e any) error { return nil })

	if err := b.PublishDomain(t.Context(), fDom{N: "n"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if len(enq.listeners) != 1 {
		t.Fatalf("expected 1 enqueued listener, got %d", len(enq.listeners))
	}

	// Sync path (no enqueuer)
	b2 := servicebus.New(nil, nil, nil)
	called := 0
	_ = b2.BindDomainEventRaw(fDom{}, struct{}{}, func(ctx context.Context, e any) error { called++; return nil })

	if err := b2.PublishDomain(t.Context(), fDom{N: "n"}); err != nil {
		t.Fatalf("publish2: %v", err)
	}

	if called != 1 {
		t.Fatalf("want called once, got %d", called)
	}
}

// ---- consolidated from more_bus_test.go ----

type gCmd struct{ ID string }

type gQry struct{ K string }

type gRes struct{ V string }

type gCmdHandler struct{ seen *[]string }

func (h gCmdHandler) Handle(ctx context.Context, c gCmd) error {
	*h.seen = append(*h.seen, c.ID)
	return nil
}

type gQryHandler struct{}

func (gQryHandler) Handle(ctx context.Context, q gQry) (gRes, error) { return gRes{V: q.K}, nil }

type badCmd struct{ X int }

func Test_GenericBindAndDispatch(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	// Bind generic command handler
	var seen []string
	if err := servicebus.BindCommand[gCmd](b, gCmdHandler{seen: &seen}); err != nil {
		t.Fatalf("bind cmd: %v", err)
	}

	// Duplicate should error
	if err := servicebus.BindCommand[gCmd](b, gCmdHandler{seen: &seen}); !errors.Is(err, berr.ErrHandlerExists) {
		t.Fatalf("want ErrHandlerExists, got %v", err)
	}

	// Dispatch happy path
	if err := b.DispatchSync(t.Context(), gCmd{ID: "x"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(seen) != 1 || seen[0] != "x" {
		t.Fatalf("seen=%v", seen)
	}

	// Bind generic query
	if err := servicebus.BindQuery[gQry, gRes](b, gQryHandler{}); err != nil {
		t.Fatalf("bind query: %v", err)
	}
	// Ask generic happy path
	r, err := servicebus.Ask[gQry, gRes](t.Context(), b, gQry{K: "k"})
	if err != nil || r.V != "k" {
		t.Fatalf("ask: %v r=%+v", err, r)
	}

	// Type mismatch on dispatch via generics path
	// Use internal dispatchWithMiddleware by calling DispatchSync with a bad type bound under generics
	// First, register a handler that expects gCmd; then pass wrong type
	err = b.DispatchSync(t.Context(), badCmd{X: 1})
	// no handler for badCmd
	if !errors.Is(err, berr.ErrHandlerNotFound) {
		t.Fatalf("want ErrHandlerNotFound, got %v", err)
	}
}

func Test_DispatchWithMiddleware_OrderAndWrapping(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	_ = b.BindCommandOf(gCmd{}, func(ctx context.Context, v any) error { return nil })

	calls := []string{}
	mw1 := func(next func(ctx context.Context, cmd any) error) func(ctx context.Context, cmd any) error {
		return func(ctx context.Context, cmd any) error {
			calls = append(calls, "mw1-before")
			err := next(ctx, cmd)

			calls = append(calls, "mw1-after")

			return err
		}
	}
	mw2 := func(next func(ctx context.Context, cmd any) error) func(ctx context.Context, cmd any) error {
		return func(ctx context.Context, cmd any) error {
			calls = append(calls, "mw2-before")
			err := next(ctx, cmd)

			calls = append(calls, "mw2-after")

			return err
		}
	}

	// Global registration order matters
	opt := servicebus.WithCommandMiddleware(mw1, mw2)
	opt(b)

	if err := b.DispatchWithMiddleware(t.Context(), gCmd{ID: "1"}); err != nil {
		t.Fatalf("dispatch with mw: %v", err)
	}

	want := []string{"mw1-before", "mw2-before", "mw2-after", "mw1-after"}
	if len(calls) != len(want) {
		t.Fatalf("calls len=%d want=%d", len(calls), len(want))
	}

	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("order mismatch at %d: %s != %s", i, calls[i], want[i])
		}
	}

	// Alias DispatchNow should work
	if err := b.DispatchNow(t.Context(), gCmd{ID: "2"}); err != nil {
		t.Fatalf("dispatch now: %v", err)
	}
}

func Test_Chain_StopsOnFirstError(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	var i int

	_ = b.BindCommandOf(gCmd{}, func(ctx context.Context, v any) error {
		i++
		if i == 2 {
			return errors.New("boom")
		}

		return nil
	})

	err := b.Chain(t.Context(), gCmd{ID: "1"}, gCmd{ID: "2"}, gCmd{ID: "3"})
	if err == nil {
		t.Fatalf("expected error")
	}

	if i != 2 { // third should not run
		t.Fatalf("ran %d handlers, want 2", i)
	}
}

func Test_Batch_Progress_Error_AndCancel(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	// Handler errors on specific ID
	_ = b.BindCommandOf(gCmd{}, func(ctx context.Context, v any) error {
		id := v.(gCmd).ID
		if id == "bad" {
			return errors.New("bad")
		}
		// Simulate minimal work
		return nil
	})

	var prog []int

	var errs []string

	opts := []servicebus.BatchOpt{
		servicebus.WithBatchProgress(func(done, total int) { prog = append(prog, done) }),
		servicebus.WithBatchOnError(func(_ int, cmd cbus.Command, _ error) {
			errs = append(errs, cmd.(gCmd).ID)
		}),
	}

	cmds := []cbus.Command{gCmd{ID: "a"}, gCmd{ID: "bad"}, gCmd{ID: "b"}}
	err := b.Batch(t.Context(), cmds, opts...)

	if err == nil {
		t.Fatalf("expected aggregated error")
	}

	// Progress should be 1,2,3
	if len(prog) != 3 || prog[0] != 1 || prog[2] != 3 {
		t.Fatalf("progress=%v", prog)
	}
	// OnError should capture the "bad" command
	if len(errs) != 1 || errs[0] != "bad" {
		t.Fatalf("errs=%v", errs)
	}

	// Now test cancel before loop starts
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = b.Batch(ctx, []cbus.Command{gCmd{ID: "x"}})

	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled joined, got %v", err)
	}
}

// ---- consolidated from untyped_bus_test.go ----

type uCmd struct{ X int }

type uQry struct{ ID string }

type uEvt struct{ Name string }

func Test_Untyped_Bindings_And_Close(t *testing.T) {
	b := servicebus.New(nil, nil, nil)
	ctx := context.Background()

	// Bind untyped command and dispatch via DispatchSync and DispatchNow
	cnt := 0
	if err := b.BindCommandOf(uCmd{}, func(ctx context.Context, v any) error {
		c := v.(uCmd)
		cnt += c.X
		return nil
	}); err != nil {
		t.Fatalf("bind command: %v", err)
	}
	if err := b.DispatchSync(ctx, uCmd{X: 2}); err != nil {
		t.Fatalf("dispatch sync: %v", err)
	}
	if err := b.DispatchNow(ctx, uCmd{X: 3}); err != nil { // alias
		t.Fatalf("dispatch now: %v", err)
	}
	if cnt != 5 {
		t.Fatalf("expected cnt=5 got %d", cnt)
	}

	// Bind untyped query and ask
	if err := b.BindQueryOf(uQry{}, func(ctx context.Context, v any) (any, error) {
		q := v.(uQry)
		return q.ID + "-ok", nil
	}); err != nil {
		t.Fatalf("bind query: %v", err)
	}
	res, err := b.Ask(ctx, uQry{ID: "A"})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if s, ok := res.(string); !ok || s != "A-ok" {
		t.Fatalf("unexpected query result: %#v", res)
	}

	// Bind untyped domain event and publish
	ecnt := 0
	if err := b.BindDomainEventOf(uEvt{}, func(ctx context.Context, v any) error {
		_ = v.(uEvt)
		ecnt++
		return nil
	}); err != nil {
		t.Fatalf("bind event: %v", err)
	}
	if err := b.PublishDomain(ctx, uEvt{Name: "hi"}); err != nil {
		t.Fatalf("publish domain: %v", err)
	}
	if ecnt != 1 {
		t.Fatalf("expected ecnt=1 got %d", ecnt)
	}

	// PublishDomain with no handlers should be no-op
	if err := b.PublishDomain(ctx, struct{ cbus.DomainEvent }{}); err != nil {
		t.Fatalf("publish domain no handlers: %v", err)
	}

	// Close (no-op)
	if err := b.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
