package httptrace

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/teapotovh/teapot/lib/observability"
)

type HTTPTrace struct {
	tp     trace.TracerProvider
	tracer trace.Tracer
}

func NewHTTPTrace() *HTTPTrace {
	return &HTTPTrace{}
}

func (ht *HTTPTrace) TracerMiddleware(next http.Handler) (handler http.Handler) {
	if ht.tp != nil && ht.tracer != nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(observability.ContextWithTracer(r.Context(), ht.tracer))
			next.ServeHTTP(w, r)
		})
		handler = otelhttp.NewHandler(handler, "httptracing",
			otelhttp.WithTracerProvider(ht.tp),
			otelhttp.WithPropagators(propagation.TraceContext{}),
		)

		return handler
	}

	return next
}

// WithTracing implements observability.Tracing.
func (ht *HTTPTrace) WithTracing(tp trace.TracerProvider, tracer trace.Tracer) {
	ht.tp = tp
	ht.tracer = tracer
}
