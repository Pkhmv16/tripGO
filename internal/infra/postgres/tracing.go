package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

type queryTracer struct {
	*otelpgx.Tracer
	duration metric.Float64Histogram
}

type queryMetricKey struct{}

type queryMetricData struct {
	started    time.Time
	operation  string
	collection string
}

func newQueryTracer(provider trace.TracerProvider, duration metric.Float64Histogram) *queryTracer {
	return &queryTracer{Tracer: otelpgx.NewTracer(
		otelpgx.WithTracerProvider(provider),
		otelpgx.WithMeterProvider(noop.NewMeterProvider()),
		otelpgx.WithDisableSQLStatementInAttributes(),
		otelpgx.WithDisableAcquireTracer(),
	), duration: duration}
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	measurement := queryMetricData{started: time.Now(), operation: otelpgx.SQLOperationName(ctx, data.SQL), collection: queryCollection(data.SQL)}
	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("db.operation.name", measurement.operation))
	if measurement.collection != "" {
		span.SetAttributes(attribute.String("db.collection.name", measurement.collection))
	}
	return context.WithValue(ctx, queryMetricKey{}, measurement)
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	errorType := queryErrorType(data.Err)
	if errorType != "" {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("error.type", errorType))
	}
	if measurement, ok := ctx.Value(queryMetricKey{}).(queryMetricData); ok {
		attrs := []attribute.KeyValue{
			attribute.String("db.operation.name", measurement.operation),
			attribute.String("db.collection.name", measurement.collection),
		}
		if errorType != "" {
			attrs = append(attrs, attribute.String("error.type", errorType))
		}
		t.duration.Record(ctx, time.Since(measurement.started).Seconds(), metric.WithAttributes(attrs...))
	}
	t.Tracer.TraceQueryEnd(ctx, conn, data)
}

func queryErrorType(err error) string {
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		return ""
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return fmt.Sprintf("%T", err)
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
