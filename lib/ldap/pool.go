package ldap

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-ldap/ldap/v3"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/teapotovh/teapot/lib/observability"
)

const (
	BackoffInitialInterval = 100 * time.Millisecond
	BackoffMultiplier      = 1.5
	BackoffMaxRetries      = 3
)

type unit struct{}

type pool struct {
	logger *slog.Logger

	url     string
	base    string
	timeout time.Duration

	demand chan unit
	source chan *ldap.Conn
}

func newPool(url, base string, timeout time.Duration, size uint32, logger *slog.Logger) *pool {
	p := &pool{
		logger: logger,

		url:     url,
		base:    base,
		timeout: timeout,

		demand: make(chan unit, size),
		source: make(chan *ldap.Conn, size),
	}

	for range size {
		p.demand <- unit{}
	}

	return p
}

// fill loops filling the pool as needed. When a connection is demanded, it dials
// one *ladp.Client and pushes it into the pool.
// Call this n times to have n workers filling the pool. Adjust n for your
// desired refill rate, which is determined by n and the connection latency,
// using Little's law.
func (p *pool) fill(ctx context.Context, i uint32, dial prometheus.Histogram) {
	p.logger.DebugContext(ctx, "started LDAP pool filler worker", "id", i)
	defer func() {
		p.logger.DebugContext(ctx, "stopped LDAP pool filler worker", "id", i)
	}()

	for {
		select {
		case <-p.demand: // wait for an actual open slot
		case <-ctx.Done():
			return
		}

		start := time.Now()

		expoBackoff := backoff.NewExponentialBackOff()
		expoBackoff.InitialInterval = BackoffInitialInterval
		expoBackoff.Multiplier = BackoffMultiplier

		conn, err := backoff.Retry(
			ctx,
			p.dial,
			backoff.WithMaxTries(BackoffMaxRetries),
			backoff.WithBackOff(expoBackoff),
		)
		if err != nil {
			p.logger.ErrorContext(ctx, "obtaining a new LDAP client failed", "retries", BackoffMaxRetries, "err", err)

			p.demand <- unit{} // retry this slot

			continue
		}

		duration := time.Since(start).Seconds()
		dial.Observe(duration)

		select {
		case p.source <- conn:
		case <-ctx.Done():
			if err := conn.Close(); err != nil {
				p.logger.ErrorContext(
					ctx,
					"error closing fresh LDAP connection during pool worker termination",
					"err",
					err,
				)
			}

			return
		}
	}
}

func (p *pool) dial() (*ldap.Conn, error) {
	conn, err := ldap.DialURL(p.url,
		ldap.DialWithDialer(&net.Dialer{Timeout: p.timeout}),
	)
	if err != nil {
		return nil, fmt.Errorf("could not enstablish a connection to the LDAP server: %w", err)
	}

	return conn, nil
}

// get returns a live, ready-to-bind connection.
func (p *pool) get(ctx context.Context) (conn *ldap.Conn, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "Pool.Get")
	defer func() { observability.SpanEnd(span, err) }()

	for {
		span.AddEvent("Attempt to get LDAP client from pool", trace.WithAttributes(
			attribute.String("pool.url", p.url),
			attribute.Int("pool.size", len(p.source)),
			attribute.Int("pool.capacity", cap(p.source)),
			attribute.Int("pool.demand", len(p.demand)),
		))

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case conn := <-p.source:
			p.demand <- unit{}

			if p.isAlive(ctx, conn) {
				return conn, nil
			}

			if err := conn.Close(); err != nil {
				return nil, fmt.Errorf("error while closing pooled non-alive connection: %w", err)
			}
		}
	}
}

func (p *pool) isAlive(ctx context.Context, conn *ldap.Conn) bool {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "isAlive")

	req := ldap.NewSearchRequest(p.base, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		0, 5, false, "(objectClass=*)", []string{"1.1"}, nil)

	_, err := conn.Search(req)
	if err != nil {
		p.logger.WarnContext(ctx, "received not-alive connection from LDAP pool", "err", err)
	}

	observability.SpanEnd(span, err)

	return err == nil
}
