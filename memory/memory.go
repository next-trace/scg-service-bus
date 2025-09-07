package memory

import (
	"github.com/next-trace/scg-service-bus/adapters/inmemory"
	cbus "github.com/next-trace/scg-service-bus/contract/bus"
	"github.com/next-trace/scg-service-bus/servicebus"
)

// New constructs a service bus backed by the in-memory adapter and returns it
// as a contract.Bus along with a cleanup function that closes the bus.
func New() (cbus.Bus, func()) { //nolint:ireturn
	ad := inmemory.New()
	sb := servicebus.New(&ad.Enqueuer, &ad.Publisher, nil)
	cleanup := func() { _ = sb.Close() }
	return sb, cleanup
}
