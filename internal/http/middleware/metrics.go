package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func Metrics(meter metric.Meter, router chi.Router) (func(http.Handler) http.Handler, error) {
	duration, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of HTTP server requests"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create HTTP request duration metric: %w", err)
	}
	active, err := meter.Int64UpDownCounter("http.server.active_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Number of HTTP server requests in progress"),
	)
	if err != nil {
		return nil, fmt.Errorf("create HTTP active requests metric: %w", err)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			attrs := []attribute.KeyValue{
				attribute.String("http.request.method", metricMethod(r.Method)),
				attribute.String("http.route", routePattern(router, r)),
			}
			activeOptions := metric.WithAttributes(attrs...)
			active.Add(r.Context(), 1, activeOptions)
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			panicking := true
			defer func() {
				active.Add(r.Context(), -1, activeOptions)
				status := wrapped.Status()
				if status == 0 {
					status = http.StatusOK
					if panicking {
						status = http.StatusInternalServerError
					}
				}
				attrs = append(attrs, attribute.Int("http.response.status_code", status))
				duration.Record(r.Context(), time.Since(started).Seconds(), metric.WithAttributes(attrs...))
			}()
			next.ServeHTTP(wrapped, r)
			panicking = false
		})
	}, nil
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch:
		return method
	default:
		return "_OTHER"
	}
}
