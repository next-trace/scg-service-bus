# SCG Service Bus

[![CI](https://github.com/next-trace/scg-service-bus/actions/workflows/ci.yml/badge.svg)](https://github.com/next-trace/scg-service-bus/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/next-trace/scg-service-bus.svg)](https://pkg.go.dev/github.com/next-trace/scg-service-bus)
[![Go Report Card](https://goreportcard.com/badge/github.com/next-trace/scg-service-bus)](https://goreportcard.com/report/github.com/next-trace/scg-service-bus)
A thin, idiomatic Go mediator aligned with Laravel 12 Bus semantics. Provides in-process dispatch/ask and domain event fan-out, plus minimal adapters to enqueue jobs and publish integration events. No globals.

## Install & Requirements
- Minimum Go: 1.25.0 (from go.mod)
- Module:

```bash
go get github.com/next-trace/scg-service-bus
```

- Build tags:
  - Kafka franz-go helper requires build tag `franz` when you use adapters/kafka.NewWithKgo.
  - NATS and RabbitMQ adapters do not require build tags.

## Concepts
- Messages
  - Command: intent to change state (contract/bus.Command)
  - Query: read-only request, returns a response (contract/bus.Query)
  - DomainEvent: in-process fan-out; handlers may be queued (contract/bus.DomainEvent)
  - IntegrationEvent: async, destined to external brokers; must implement Topic() string
- Handlers (generics in contract/bus)
  - CommandHandler[C], QueryHandler[Q,R], DomainEventHandler[E]
- Queue semantics
  - Queueable (alias: ShouldQueue) on commands → Dispatch enqueues when a JobEnqueuer is configured
  - QueueableListener (alias: ShouldQueueListener) on domain event handlers → handler is enqueued if a JobEnqueuer exists
- Errors
  - Canonical errors live in contract/errors/errors.go and are wrapped with %w (fmt.Errorf) across the codebase

## Quickstart (sync, in-process)

```go
package main

import (
	"context"
	"fmt"

	"github.com/next-trace/scg-service-bus/servicebus"
)

type CreateUser struct{ ID, Name string }

type GetUser struct{ ID string }

type User struct{ ID, Name string }

func main() {
	bus := servicebus.New(nil, nil, nil)

	// Bind using the provided "Of" helpers (untyped) or the generic helpers below.
	_ = bus.BindCommandOf(CreateUser{}, func(ctx context.Context, v any) error {
		c := v.(CreateUser)
		fmt.Println("created:", c.ID, c.Name)
		return nil
	})

	_ = bus.BindQueryOf(GetUser{}, func(ctx context.Context, v any) (any, error) {
		q := v.(GetUser)
		return User{ID: q.ID, Name: "Jane"}, nil
	})

	// Sync dispatch and ask
	_ = bus.DispatchSync(context.Background(), CreateUser{ID: "u-1", Name: "Jane"})

	res, _ := bus.Ask(context.Background(), GetUser{ID: "u-1"})
	u := res.(User)
	fmt.Println("user:", u.ID, u.Name)
}
```

Typed helpers (generics):

```go
package main

import (
	"context"
	cbus "github.com/next-trace/scg-service-bus/contract/bus"
	"github.com/next-trace/scg-service-bus/servicebus"
)

type CreateUser struct{ ID, Name string }

type CreateUserHandler struct{}

func (CreateUserHandler) Handle(ctx context.Context, c CreateUser) error { return nil }

var _ cbus.CommandHandler[CreateUser] = (*CreateUserHandler)(nil)

// Query

type GetUser struct{ ID string }

type User struct{ ID, Name string }

type GetUserHandler struct{}

func (GetUserHandler) Handle(ctx context.Context, q GetUser) (User, error) { return User{ID: q.ID, Name: "Jane"}, nil }

var _ cbus.QueryHandler[GetUser, User] = (*GetUserHandler)(nil)

func main() {
	bus := servicebus.New(nil, nil, nil)
	_ = servicebus.BindCommand[CreateUser](bus, CreateUserHandler{})
	_ = servicebus.BindQuery[GetUser, User](bus, GetUserHandler{})

	u, err := servicebus.Ask[GetUser, User](context.Background(), bus, GetUser{ID: "u-1"})
	_ = u
	_ = err
}
```

Domain events:

```go
// Bind domain event handler
_ = bus.BindDomainEventOf(UserCreated{}, func(ctx context.Context, e any) error { return nil })

// Or type-safe
// _ = servicebus.BindDomainEvent[UserCreated](bus, SendWelcomeEmail{})

_ = bus.PublishDomain(ctx, UserCreated{ID: "u-1"})
```

Integration events:

```go
// Define an integration event with a Topic
 type UserExported struct{ ID string }
 func (UserExported) Topic() string { return "users.exported" }
 
 // Publish via the configured publisher/adapter
 _ = bus.PublishIntegration(ctx, UserExported{ID: "u-1"}, cbus.PublishOptions{Key: "u-1"})
```

## Async and queueing semantics
- Commands
  - bus.Dispatch(ctx, cmd) enqueues when cmd implements Queueable and a JobEnqueuer is configured. Otherwise, it executes synchronously (same as DispatchSync).
- Domain events
  - PublishDomain(ctx, e) fans out synchronously to all handlers by default.
  - If a handler implements QueueableListener and a JobEnqueuer is configured, that handler is enqueued instead of being called inline.
- Integration events
  - PublishIntegration(ctx, e, opts) always goes through the configured EventPublisher. If none is set, returns contract/errors.ErrAsyncNotConfigured.

## Tracing and header propagation
The bus itself is transport-agnostic. For cross-service tracing, adapters may inject trace context into outbound message headers.

- Interface: contract/bus.HeaderPropagator exposes Inject(ctx context.Context, headers map[string]string).
- Default: contract/bus.NopHeaderPropagator does nothing and is safe for concurrent use.
- RabbitMQ: the adapter supports an optional Propagator that injects tracing headers for EnqueueCommand, EnqueueListener, and PublishIntegration.
  - If you build the adapter with your own Publisher, use NewWithPropagator(publisher, propagator).
  - If you use NewWithAMQPConn, set the Propagator field after construction: ad.Propagator = myPropagator.

Example (RabbitMQ + OpenTelemetry-style propagator, pseudo-code):

```go
var hp cbus.HeaderPropagator = MyOTELPropagator{}
ad, cleanup, err := rabbitmq.NewWithAMQPConn(rabbitmq.Config{URL: "amqp://guest:guest@localhost:5672/"})
if err != nil { panic(err) }
defer cleanup()
// enable propagation
ad.Propagator = hp
bus := servicebus.New(ad, ad, nil)
```

## Adapters
All adapters satisfy contract/bus.Adapter (both JobEnqueuer and EventPublisher) or subsets thereof. Wire them into servicebus.New(enqueuer, publisher, logger).

- InMemory (adapters/inmemory)
  - Constructor: inmemory.New() -> *inmemory.Adapter
  - Use for tests/examples; stores enqueued commands/listeners and published events in memory.
  - Wiring:

```go
ad := inmemory.New()
bus := servicebus.New(ad, ad, nil)
```

- Kafka (adapters/kafka)
  - Constructors:
    - kafka.New(w Writer) where Writer has: Write(topic string, key, value []byte, headers map[string]string) error
    - kafka.NewWithKgo(cfg Config) (requires build tag franz)
  - Topic mapping:
    - EnqueueCommand → topic "jobs.<CommandType>" or "jobs.<QueueOptions.Queue>"
    - EnqueueListener → topic "listeners.<EventType>.<handlerName>" or "jobs.<QueueOptions.Queue>"
    - PublishIntegration → topic from e.Topic() or opts.TopicOverride; Key from opts.Key
  - Delay: sets header "x-delay" (seconds) when QueueOptions.DelaySeconds > 0. No native delay; infra/consumers must respect it.
  - Context: Writer has no context; adapter checks ctx before calling Writer and then performs a synchronous write (caller cancellation after that point is not observed).
  - Wiring examples:

```go
// Minimal custom writer example
package main

import (
	"context"
	"github.com/next-trace/scg-service-bus/adapters/kafka"
	"github.com/next-trace/scg-service-bus/servicebus"
)

type MyWriter struct{}

func (MyWriter) Write(topic string, key, value []byte, headers map[string]string) error {
	// send to Kafka using your client
	return nil
}

func main() {
	ad := kafka.New(MyWriter{})
	bus := servicebus.New(ad, ad, nil)
	_ = bus // use bus
}
```

```bash
# franz-go helper
go build -tags=franz ./...
```

- NATS (adapters/nats)
  - Constructors:
    - nats.New(c Client) where Client has: Publish(subject string, data []byte, headers map[string]string) error
    - nats.NewWithNATS(cfg Config)
  - Subject mapping:
    - EnqueueCommand → subject "cmd.<CommandType>" or "cmd.<QueueOptions.Queue>"
    - EnqueueListener → subject "listeners.<EventType>.<handlerName>" or "cmd.<QueueOptions.Queue>"
    - PublishIntegration → subject from e.Topic() or opts.TopicOverride; optional header "key" from opts.Key
  - Delay: sets header "x-delay" (seconds) when QueueOptions.DelaySeconds > 0 (advisory only).
  - Wiring example:

```go
ad, cleanup, err := nats.NewWithNATS(nats.Config{URL: "nats://localhost:4222"})
if err != nil { panic(err) }
defer cleanup()
bus := servicebus.New(ad, ad, nil)
```

- RabbitMQ (adapters/rabbitmq)
  - Constructors:
    - rabbitmq.New(p Publisher) where Publisher has: Publish(ctx context.Context, m PubMsg) error
    - rabbitmq.NewWithAMQPChannel(ch *amqp.Channel)
    - rabbitmq.NewWithAMQPConn(cfg Config) declares a durable topic exchange named "integration" for integration events (for convenience), but the default Adapter publishes to the default exchange "" by default.
  - RoutingKey mapping (default exchange "" used by the Adapter):
    - EnqueueCommand → "cmd.<CommandType>" or "cmd.<QueueOptions.Queue>"
    - EnqueueListener → "listener.<EventType>.<handlerName>" or "cmd.<QueueOptions.Queue>"
    - PublishIntegration → routing key from e.Topic() or opts.TopicOverride; when using NewWithAMQPConn, messages are published with DeliveryMode Persistent.
  - Delay: sets header "x-delay" (seconds). Requires RabbitMQ delayed-message plugin or equivalent infra to take effect.
  - Wiring example:

```go
ad, cleanup, err := rabbitmq.NewWithAMQPConn(rabbitmq.Config{URL: "amqp://guest:guest@localhost:5672/"})
if err != nil { panic(err) }
defer cleanup()
// optional: enable trace header propagation
aProp := MyPropagator{}
ad.Propagator = aProp
bus := servicebus.New(ad, ad, nil)
```

## Delays
QueueOptions.DelaySeconds is propagated as "x-delay" header by all built-in adapters (Kafka/NATS/RabbitMQ). None of the adapters enforce native delays or return ErrDelayUnsupported; honoring delay is infrastructure-dependent (broker plugins, consumers, or middleware).

## Errors
- Canonical errors are defined in contract/errors/errors.go (e.g., ErrHandlerExists, ErrHandlerNotFound, ErrHandlerTypeMismatch, ErrAsyncNotConfigured, ErrEnqueueFailed, ErrPublishFailed, ErrDelayUnsupported, ErrSerializationFailed).
- Library operations wrap these with %w adding context (use errors.Is/As to check).

## Example
A runnable example is included in examples/main.go. Run:

```bash
go run ./examples
```

The example uses the in-memory adapter by default. To switch to a real adapter, replace the constructor accordingly (see Adapters section). For franz-go Kafka helper, build with:

```bash
go run -tags=franz ./examples
```

## Testing & Quality
- CI: GitHub Actions runs build, tests (race, coverage), lint (golangci-lint), and security checks (govulncheck, gosec).
  - Status: see the CI badge at the top of this README.
- Coverage: A coverage.txt artifact is produced by CI; download it from the workflow run artifacts. You can also run locally:
  
  ```bash
  go test -race -coverprofile=coverage.txt -covermode=atomic ./...
  go tool cover -func=coverage.txt | tail -n 1
  ```

## Versioning

This project follows [Semantic Versioning](https://semver.org/) (`MAJOR.MINOR.PATCH`).

- **MAJOR**: Breaking API changes
- **MINOR**: New features (backward-compatible)
- **PATCH**: Bug fixes and improvements (backward-compatible)

Consumers should always pin to a specific tag (e.g. `v1.2.3`) to avoid accidental breaking changes.


## Security
- Report vulnerabilities via GitHub Security Advisories or by opening a private issue.
- Avoid posting sensitive details in public issues.

## Changelog
- Changes are tracked in Git history and GitHub Releases. Use tags to pin versions.

## License & Contributing
- License: MIT
- Contributions: PRs and issues welcome; keep changes minimal and idiomatic.
