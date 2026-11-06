package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type queryTracer struct {
	*otelpgx.Tracer
}

func newQueryTracer(provider trace.TracerProvider) *queryTracer {
	return &queryTracer{Tracer: otelpgx.NewTracer(
		otelpgx.WithTracerProvider(provider),
		otelpgx.WithDisableSQLStatementInAttributes(),
		otelpgx.WithDisableAcquireTracer(),
	)}
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("db.operation.name", otelpgx.SQLOperationName(ctx, data.SQL)))
	if collection := queryCollection(data.SQL); collection != "" {
		span.SetAttributes(attribute.String("db.collection.name", collection))
	}
	return ctx
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if data.Err != nil && !errors.Is(data.Err, pgx.ErrNoRows) {
		errorType := fmt.Sprintf("%T", data.Err)
		var pgErr *pgconn.PgError
		if errors.As(data.Err, &pgErr) {
			errorType = pgErr.Code
		}
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("error.type", errorType))
	}
	t.Tracer.TraceQueryEnd(ctx, conn, data)
}

func queryCollection(sql string) string {
	fields := strings.Fields(sql)
	if len(fields) < 2 {
		return ""
	}
	var table string
	switch strings.ToUpper(fields[0]) {
	case "UPDATE":
		table = fields[1]
	case "SELECT", "DELETE", "INSERT":
		keyword := "FROM"
		if strings.EqualFold(fields[0], "INSERT") {
			keyword = "INTO"
		}
		for i := 1; i+1 < len(fields); i++ {
			if strings.EqualFold(fields[i], keyword) {
				table = fields[i+1]
				break
			}
		}
	}
	table, _, _ = strings.Cut(table, "(")
	return strings.Trim(table, "\";")
}
