package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			panicking := true
			defer func() {
				status := wrapped.Status()
				if status == 0 {
					status = http.StatusOK
					if panicking {
						status = http.StatusInternalServerError
					}
				}
				route := ""
				if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
					route = routeContext.RoutePattern()
				}
				level := slog.LevelInfo
				if status >= http.StatusInternalServerError {
					level = slog.LevelError
				}
				logger.LogAttrs(r.Context(), level, "request handled",
					slog.String("method", r.Method),
					slog.String("route", route),
					slog.Int("status", status),
					slog.Float64("duration_ms", float64(time.Since(started))/float64(time.Millisecond)),
				)
			}()
			next.ServeHTTP(wrapped, r)
			panicking = false
		})
	}
}
