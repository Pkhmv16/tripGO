package telemetry

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type MetricsConfig struct {
	Endpoint      string
	Protocol      string
	OTLPTimeout   time.Duration
	Interval      time.Duration
	ExportTimeout time.Duration
}

func NewMeterProvider(ctx context.Context, cfg MetricsConfig) (*sdkmetric.MeterProvider, error) {
	res, err := newResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}
	var exporter sdkmetric.Exporter
	switch cfg.Protocol {
	case "http/protobuf":
		endpoint, pathErr := url.JoinPath(cfg.Endpoint, "v1/metrics")
		if pathErr != nil {
			return nil, fmt.Errorf("build OTLP metrics URL: %w", pathErr)
		}
		exporter, err = otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint), otlpmetrichttp.WithTimeout(cfg.OTLPTimeout))
	case "grpc":
		exporter, err = otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpointURL(cfg.Endpoint), otlpmetricgrpc.WithTimeout(cfg.OTLPTimeout))
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol %q", cfg.Protocol)
	}
	if err != nil {
		return nil, fmt.Errorf("create metrics exporter: %w", err)
	}
	reader := sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(cfg.Interval), sdkmetric.WithTimeout(cfg.ExportTimeout))
	return sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(reader)), nil
}
