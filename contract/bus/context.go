package bus

import "context"

// Context is re-exported for convenience in handler signatures.
// It avoids importing context in user packages when referencing bus types.
type Context = context.Context

// HeaderPropagator abstracts injecting tracing context into headers.
// Implementations may bridge to OpenTelemetry or any other propagation standard.
// This keeps adapters decoupled from concrete tracing libraries (code-to-interface).
// Implementors should mutate the provided headers map by inserting keys that
// carry the context across process boundaries. Implementations must be safe for concurrent use.
type HeaderPropagator interface {
	Inject(ctx context.Context, headers map[string]string)
}

// NopHeaderPropagator is a no-op implementation useful for tests or when tracing is disabled.
type NopHeaderPropagator struct{}

func (NopHeaderPropagator) Inject(ctx context.Context, headers map[string]string) {
	_ = ctx
	_ = headers
}
