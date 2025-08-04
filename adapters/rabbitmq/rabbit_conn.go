package rabbitmq

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	berr "github.com/next-trace/scg-service-bus/contract/errors"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Concrete AMQP connection-backed constructor and publisher wrapper with auto-reconnect.

const (
	integrationExchange   = "integration"
	integrationExchangeTy = "topic"
)

type Config struct {
	URL         string
	ConnTimeout time.Duration
}

type reconnectingPublisher struct {
	cfg    Config
	mu     sync.RWMutex
	conn   *amqp.Connection
	ch     *amqp.Channel
	closed chan struct{}
	ready  chan struct{} // closed when a channel is ready
}

func newReconnectingPublisher(cfg Config) (*reconnectingPublisher, func()) {
	rp := &reconnectingPublisher{
		cfg:    cfg,
		closed: make(chan struct{}),
		ready:  make(chan struct{}),
	}
	go rp.run()
	cleanup := func() { rp.close() }
	return rp, cleanup
}

func (rp *reconnectingPublisher) Publish(ctx context.Context, m PubMsg) error {
	// Fast path: ensure channel available
	rp.mu.RLock()
	ch := rp.ch
	rp.mu.RUnlock()
	if ch == nil {
		// Wait for readiness or context cancellation
		select {
		case <-rp.ready:
			// proceed
		case <-ctx.Done():
			return ctx.Err()
		}
		rp.mu.RLock()
		ch = rp.ch
		rp.mu.RUnlock()
		if ch == nil {
			return fmt.Errorf("%w: rabbitmq not connected", berr.ErrPublishFailed)
		}
	}

	var h amqp.Table
	if len(m.Headers) > 0 {
		h = amqp.Table{}
		for k, v := range m.Headers {
			h[k] = v
		}
	}

	return ch.PublishWithContext(
		ctx,
		m.Exchange,
		m.RoutingKey,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			Headers:      h,
			ContentType:  "application/json",
			Body:         m.Body,
		},
	)
}

func (rp *reconnectingPublisher) run() {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	// #nosec G404 -- non-crypto RNG is acceptable for backoff jitter
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // non-crypto RNG is acceptable for backoff jitter

	reconnect := func() (*amqp.Connection, *amqp.Channel, error) {
		conn, err := amqp.DialConfig(rp.cfg.URL, amqp.Config{
			Locale:     "en_US",
			Properties: amqp.Table{"product": "scg-service-bus"},
			Dial:       amqp.DefaultDial(rp.cfg.ConnTimeout),
		})
		if err != nil {
			return nil, nil, err
		}
		ch, err := conn.Channel()
		if err != nil {
			_ = conn.Close()
			return nil, nil, err
		}
		if err := ch.ExchangeDeclare(
			integrationExchange,
			integrationExchangeTy,
			true,
			false,
			false,
			false,
			nil,
		); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return nil, nil, err
		}
		return conn, ch, nil
	}

	for {
		select {
		case <-rp.closed:
			return
		default:
		}

		conn, ch, err := reconnect()
		if err != nil {
			// exponential backoff with jitter
			jitter := time.Duration(rng.Int63n(int64(backoff / 2)))
			sleep := backoff + jitter/2
			if sleep > maxBackoff {
				sleep = maxBackoff
			}
			t := time.NewTimer(sleep)
			select {
			case <-rp.closed:
				t.Stop()
				return
			case <-t.C:
			}
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}

		// success
		backoff = time.Second

		rp.mu.Lock()
		rp.conn = conn
		rp.ch = ch
		// signal readiness (recreate channel each time)
		oldReady := rp.ready
		rp.ready = make(chan struct{})
		close(oldReady)
		// immediately mark new ready channel as closed since we are ready now
		close(rp.ready)
		rp.mu.Unlock()

		// Block on connection close notifications to trigger reconnect
		notify := conn.NotifyClose(make(chan *amqp.Error, 1))
		select {
		case <-rp.closed:
			_ = ch.Close()
			_ = conn.Close()
			return
		case <-notify:
			_ = ch.Close()
			_ = conn.Close()
			// loop to reconnect
		}
	}
}

func (rp *reconnectingPublisher) close() {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	select {
	case <-rp.closed:
		// already closed
		return
	default:
		close(rp.closed)
	}
	if rp.ch != nil {
		_ = rp.ch.Close()
		rp.ch = nil
	}
	if rp.conn != nil {
		_ = rp.conn.Close()
		rp.conn = nil
	}
}

// NewWithAMQPConn dials RabbitMQ with auto-reconnect, ensures integration exchange, and returns Adapter and cleanup.
func NewWithAMQPConn(cfg Config) (*Adapter, func(), error) {
	if cfg.URL == "" {
		return nil, nil, fmt.Errorf("%w: rabbitmq url required", berr.ErrPublishFailed)
	}
	pub, cleanup := newReconnectingPublisher(cfg)
	ad := New(pub)
	return ad, cleanup, nil
}
