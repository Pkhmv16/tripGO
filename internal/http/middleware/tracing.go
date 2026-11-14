package middleware

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func Tracing(provider trace.TracerProvider, router chi.Router) http.Handler {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		if state, ok := r.Context().Value(recoveryContextKey{}).(*recoveryState); ok {
			state.ctx = r.Context()
		}
		if route := routePattern(router, r); route != "" {
			span.SetAttributes(attribute.String("http.route", route))
		}
		wrappedWriter := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		panicking := true
		defer func() {
			if panicking {
				status := wrappedWriter.Status()
				if status == 0 {
					status = http.StatusInternalServerError
				}
				span.RecordError(errRequestPanic)
				span.SetStatus(codes.Error, errRequestPanic.Error())
				span.SetAttributes(attribute.String("error.type", "panic"), attribute.Int("http.response.status_code", status))
			}
		}()
		router.ServeHTTP(wrappedWriter, r)
		panicking = false
		if wrappedWriter.Status() >= http.StatusInternalServerError {
			span.SetAttributes(attribute.String("error.type", strconv.Itoa(wrappedWriter.Status())))
		}
	})
	return otelhttp.NewHandler(handler, "HTTP", otelhttp.WithTracerProvider(provider), otelhttp.WithPropagators(propagation.TraceContext{}), otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		if route := routePattern(router, r); route != "" {
			return r.Method + " " + route
		}
		return r.Method
	}))
}

func routePattern(router chi.Router, r *http.Request) string {
	ctx := chi.NewRouteContext()
	if router.Match(ctx, r.Method, r.URL.Path) {
		return ctx.RoutePattern()
	}
	return ""
}
