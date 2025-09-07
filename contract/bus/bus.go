package bus

import "context"

// Bus is a minimal, tech-agnostic interface that mirrors the capabilities of the
// concrete service bus while remaining non-generic for interface compatibility.
//
// Typed helpers remain available via generic helper functions in the servicebus package.
// This interface is intended for consumers that want to depend only on contracts.
type Bus interface {
	// Bind (untyped) – type-safe bindings continue via helper funcs in servicebus.
	BindCommandOf(sample any, handler func(ctx context.Context, v any) error) error
	BindQueryOf(sample any, handler func(ctx context.Context, v any) (any, error)) error
	BindDomainEventOf(sample any, handler func(ctx context.Context, v any) error) error

	// Exec
	Dispatch(ctx context.Context, cmd Command) error
	DispatchSync(ctx context.Context, cmd Command) error

	// Query
	Ask(ctx context.Context, query any) (any, error)

	// Events
	PublishDomain(ctx context.Context, event DomainEvent) error
	PublishIntegration(ctx context.Context, event IntegrationEvent, opts PublishOptions) error

	// Lifecycle
	Close() error
}
