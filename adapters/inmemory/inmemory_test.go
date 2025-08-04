package inmemory_test

import (
	"sync"
	"testing"

	"github.com/next-trace/scg-service-bus/adapters/inmemory"
	cbus "github.com/next-trace/scg-service-bus/contract/bus"
)

type cmd struct{ ID string }

type dom struct{ Name string }

type integ struct{ T string }

func (i integ) Topic() string { return i.T }

func TestInmemory_EnqueueAndPublish_Recordings(t *testing.T) {
	ad := inmemory.New()

	// Enqueue a command
	if err := ad.EnqueueCommand(t.Context(), cmd{ID: "1"}, cbus.QueueOptions{Queue: "q"}); err != nil {
		t.Fatalf("enqueue cmd: %v", err)
	}

	// Enqueue a listener
	if err := ad.EnqueueListener(
		t.Context(),
		dom{Name: "n"},
		"Handler",
		cbus.QueueOptions{Queue: "listeners"},
	); err != nil {
		t.Fatalf("enqueue listener: %v", err)
	}

	// Publish an integration event
	if err := ad.PublishIntegration(
		t.Context(),
		integ{T: "topic"},
		cbus.PublishOptions{Key: "k"},
	); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if n := len(ad.Commands); n != 1 {
		t.Fatalf("want 1 command, got %d", n)
	}

	if n := len(ad.Listeners); n != 1 {
		t.Fatalf("want 1 listener, got %d", n)
	}

	if n := len(ad.Events); n != 1 {
		t.Fatalf("want 1 event, got %d", n)
	}
}

func TestInmemory_ConcurrentSafety(t *testing.T) {
	ad := inmemory.New()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)

		enqueueCmd := func(_ int) {
			defer wg.Done()

			_ = ad.EnqueueCommand(t.Context(), cmd{ID: "c"}, cbus.QueueOptions{})
		}

		enqueueListener := func(_ int) {
			defer wg.Done()

			_ = ad.EnqueueListener(t.Context(), dom{Name: "d"}, "H", cbus.QueueOptions{})
		}

		publishInteg := func(_ int) {
			defer wg.Done()

			_ = ad.PublishIntegration(t.Context(), integ{T: "t"}, cbus.PublishOptions{})
		}

		go enqueueCmd(i)
		go enqueueListener(i)
		go publishInteg(i)
	}

	wg.Wait()

	if len(ad.Commands) != 50 {
		t.Fatalf("commands=%d", len(ad.Commands))
	}

	if len(ad.Listeners) != 50 {
		t.Fatalf("listeners=%d", len(ad.Listeners))
	}

	if len(ad.Events) != 50 {
		t.Fatalf("events=%d", len(ad.Events))
	}
}
