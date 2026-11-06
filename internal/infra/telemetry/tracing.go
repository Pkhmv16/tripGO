package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type TracingConfig struct {
	Endpoint string
	Protocol string
	Sampler  string
	Ratio    float64
}

func NewTracerProvider(ctx context.Context, cfg TracingConfig, logger *slog.Logger) (*sdktrace.TracerProvider, error) {
	sampler, err := newSampler(cfg)
	if err != nil {
		return nil, err
	}
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { logger.Error("telemetry export failed", "error", err) }))
	res, err := newResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}
	var exporter sdktrace.SpanExporter
	switch cfg.Protocol {
	case "http/protobuf":
		endpoint, pathErr := url.JoinPath(cfg.Endpoint, "v1/traces")
		if pathErr != nil {
			return nil, fmt.Errorf("build OTLP trace URL: %w", pathErr)
		}
		exporter, err = otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithTimeout(2*time.Second))
	case "grpc":
		exporter, err = otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(cfg.Endpoint), otlptracegrpc.WithTimeout(2*time.Second))
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol %q", cfg.Protocol)
	}
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	return sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSampler(sampler), sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Second), sdktrace.WithExportTimeout(2*time.Second))), nil
}

func newSampler(cfg TracingConfig) (sdktrace.Sampler, error) {
	switch cfg.Sampler {
	case "always_on":
		return sdktrace.AlwaysSample(), nil
	case "always_off":
		return sdktrace.NeverSample(), nil
	case "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample()), nil
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample()), nil
	case "traceidratio", "parentbased_traceidratio":
		if math.IsNaN(cfg.Ratio) || cfg.Ratio < 0 || cfg.Ratio > 1 {
			return nil, fmt.Errorf("OTEL_TRACES_SAMPLER_ARG must be between 0 and 1")
		}
		sampler := sdktrace.TraceIDRatioBased(cfg.Ratio)
		if cfg.Sampler == "parentbased_traceidratio" {
			sampler = sdktrace.ParentBased(sampler)
		}
		return sampler, nil
	default:
		return nil, fmt.Errorf("unsupported OTEL_TRACES_SAMPLER %q", cfg.Sampler)
	}
}
