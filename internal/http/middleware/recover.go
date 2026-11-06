package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/Pkhmv16/tripGO/internal/http/domainerror"
)

var errRequestPanic = errors.New("request panicked")

type recoveryContextKey struct{}

type recoveryState struct {
	ctx context.Context
}

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := &recoveryState{ctx: r.Context()}
			r = r.WithContext(context.WithValue(r.Context(), recoveryContextKey{}, state))
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				if recovered := recover(); recovered != nil {
					if recovered == http.ErrAbortHandler {
						panic(recovered)
					}
					logger.ErrorContext(state.ctx, "request panicked",
						"panic", recovered, "stack", string(debug.Stack()),
					)
					if wrapped.Status() != 0 {
						panic(http.ErrAbortHandler)
					}
					wrapped.Header().Del("Content-Length")
					domainerror.Write(wrapped, r, errRequestPanic)
				}
			}()
			next.ServeHTTP(wrapped, r)
		})
	}
}
