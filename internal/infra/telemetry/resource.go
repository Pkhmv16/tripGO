package telemetry

import (
	"context"
	"runtime/debug"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
)

func newResource(ctx context.Context) (*resource.Resource, error) {
	version := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	return resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "trip-service"), attribute.String("service.namespace", "tripgo"), attribute.String("deployment.environment", "local")),
		resource.WithFromEnv(),
		resource.WithAttributes(attribute.String("service.version", version)),
	)
}
