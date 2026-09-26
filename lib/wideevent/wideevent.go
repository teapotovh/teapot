package wideevent

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"reflect"
	"sync"

	"github.com/iancoleman/strcase"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/teapotovh/teapot/lib/observability"
)

type Emit interface {
	Emit() bool
}

type AlwaysEmit struct{}

func (AlwaysEmit) Emit() bool { return true }

// Ensure AlwaysEmit implements Emit.
var _ Emit = AlwaysEmit{}

type NeverEmit struct{}

func (NeverEmit) Emit() bool { return false }

// Ensure NeverEmit implements Emit.
var _ Emit = NeverEmit{}

// Fields is the collection of types to be serliazed into logs/spans.
type Fields map[string]any

// WideEvent is the interface that must be implemented by all wide event structs.
type WideEvent interface {
	Emit

	Fields() Fields
}

// ExtractFields provides a generic implementation for WideEvent.Fields at the
// cost of using reflection.
func ExtractFields(val any) map[string]any {
	out := make(map[string]any)

	v := reflect.ValueOf(val)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || f.Anonymous {
			continue
		}

		name := strcase.ToSnake(f.Name)
		out[name] = v.Field(i).Interface()
	}

	return out
}

type Handle[T WideEvent] struct {
	ctx context.Context

	name     string
	children *children

	we      T
	span    trace.Span
	sampled bool
}

// WideEvent returns the wide event type stored in this handle.
func (h *Handle[T]) WideEvent() T {
	return h.we
}

// Span returns the span for this WideEvent handle.
// NOTE: manually reaching for the span is discouraged; the span is entirely
// managed by the Handle and populated from the WideEvent fields.
func (h *Handle[T]) Span() trace.Span {
	return h.span
}

// Sampled forces the whole trace to be sampled by the collector. Effectively
// sets the `sampled` *string* field to `true` on this span: the collector must
// the configured to sample on this condition.
func (h *Handle[T]) Sampled() {
	h.sampled = true
}

const WideEventMessage = "wide_event"

// End marks the end of this wide event and span. The fields from the WideEvent
// object are at this point stored in the span and logged out (if Emit() return
// true). If the provided error is non-nil the span is marked as failed. This
// will in turn make it sampled.
func (h *Handle[T]) End(err error) {
	fields := h.we.Fields()

	attrs := toOtelAttributes(fields)
	if h.sampled {
		attrs = append(attrs, attribute.String("sampled", "true"))
	}

	h.span.SetAttributes(attrs...)

	if err != nil {
		h.span.RecordError(err)
		h.span.SetStatus(codes.Error, err.Error())
	}

	h.span.End()

	parent := parent(h.ctx)
	switch parent {
	case nil:
		logger := logger(h.ctx)

		if logger != nil && h.we.Emit() {
			// We're the root WideEvent, so we should log it out
			for name, cf := range h.children.fields() {
				if _, exists := fields[name]; exists {
					logger.WarnContext(h.ctx, "overwriting field in wide event", "field", name)
				}

				switch len(cf) {
				case 1:
					fields[name] = cf[0]
				default:
					fields[name] = cf
				}
			}

			attrs := toSlogAttributes(fields)
			if err != nil {
				attrs = append(attrs, slog.Any("err", err))
			}

			level := slog.LevelInfo
			if err != nil {
				level = slog.LevelError
			}

			logger.LogAttrs(
				h.ctx,
				level,
				WideEventMessage,
				append([]slog.Attr{slog.String("wide_event", h.name)}, attrs...)...)
		}

	default:
		if h.we.Emit() {
			parent.add(h.name, fields)
		}
	}
}

type (
	parentContextKey struct{}
	loggerContextKey struct{}
)

var (
	parentKey = parentContextKey{}
	loggerKey = loggerContextKey{}
)

func parent(ctx context.Context) *children {
	val := ctx.Value(parentKey)
	if m, ok := val.(*children); ok {
		return m
	}

	return nil
}

func withParent(ctx context.Context, val *children) context.Context {
	return context.WithValue(ctx, parentKey, val)
}

func logger(ctx context.Context) *slog.Logger {
	val := ctx.Value(loggerKey)
	if l, ok := val.(*slog.Logger); ok {
		return l
	}

	return nil
}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

type children struct {
	m  map[string][]Fields
	mu sync.Mutex
}

func newChildren() *children {
	m := map[string][]Fields{}
	return &children{m: m}
}

func (c *children) fields() iter.Seq2[string, []Fields] {
	return func(yield func(string, []Fields) bool) {
		c.mu.Lock()
		defer c.mu.Unlock()

		for k, v := range c.m {
			if !yield(k, v) {
				return
			}
		}
	}
}

func (c *children) add(name string, fields Fields) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.m[name]; !ok {
		c.m[name] = make([]Fields, 0, 1)
	}

	c.m[name] = append(c.m[name], fields)
}

func Start[T any, P interface {
	*T
	WideEvent
}](ctx context.Context, name string) (context.Context, *T, Handle[P]) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, name)

	children := newChildren()

	var t T

	handle := Handle[P]{
		ctx: ctx,

		name:     strcase.ToSnake(name),
		children: children,

		span: span,
		we:   &t,
	}

	return withParent(ctx, handle.children), &t, handle
}

func toOtelAttributes(m Fields) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(m))
	for k, v := range m {
		attrs = append(attrs, toKeyValue(k, v))
	}

	return attrs
}

func toKeyValue(k string, v any) attribute.KeyValue {
	switch val := v.(type) {
	case string:
		return attribute.String(k, val)
	case bool:
		return attribute.Bool(k, val)
	case int:
		return attribute.Int(k, val)
	case int64:
		return attribute.Int64(k, val)
	case float64:
		return attribute.Float64(k, val)
	case float32:
		return attribute.Float64(k, float64(val))
	case []string:
		return attribute.StringSlice(k, val)
	case []bool:
		return attribute.BoolSlice(k, val)
	case []int:
		return attribute.IntSlice(k, val)
	case []int64:
		return attribute.Int64Slice(k, val)
	case []float64:
		return attribute.Float64Slice(k, val)
	case nil:
		return attribute.String(k, "")
	default:
		return attribute.String(k, fmt.Sprintf("%v", val))
	}
}

func toSlogAttributes(m Fields) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(m))
	for k, v := range m {
		attrs = append(attrs, slog.Any(k, v))
	}

	return attrs
}
