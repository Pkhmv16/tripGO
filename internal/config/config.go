package config

import (
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	OTelEndpoint          string
	OTelProtocol          string
	OTelSampler           string
	OTelSamplerRatio      float64
	HTTPAddr              string
	HTTPReadTimeout       time.Duration
	HTTPReadHeaderTimeout time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
	HTTPMaxBodyBytes      int64

	LogLevel        slog.Level
	ShutdownTimeout time.Duration

	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration
	DatabaseRollbackTimeout time.Duration
	IdempotencyTTL          time.Duration
}

func LoadConfig() (Config, error) {
	var cfg Config

	cfg.HTTPAddr = strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if cfg.HTTPAddr == "" {
		return Config{}, fmt.Errorf("HTTP_ADDR is required")
	}

	_, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid HTTP_ADDR: %w", err)
	}

	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Config{}, fmt.Errorf("HTTP_ADDR must contain a port between 1 and 65535")
	}

	cfg.DatabaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	cfg.HTTPMaxBodyBytes, err = readPositiveInt64("HTTP_MAX_BODY_BYTES")
	if err != nil {
		return Config{}, err
	}

	level := strings.TrimSpace(os.Getenv("LOG_LEVEL"))
	switch level {
	case "debug":
		cfg.LogLevel = slog.LevelDebug
	case "info":
		cfg.LogLevel = slog.LevelInfo
	case "warn":
		cfg.LogLevel = slog.LevelWarn
	case "error":
		cfg.LogLevel = slog.LevelError
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL must be debug, info, warn or error")
	}

	cfg.DatabaseMaxConns, err = readInt32("DATABASE_MAX_CONNS")
	if err != nil {
		return Config{}, err
	}

	cfg.DatabaseMinConns, err = readInt32("DATABASE_MIN_CONNS")
	if err != nil {
		return Config{}, err
	}

	if cfg.DatabaseMaxConns <= 0 {
		return Config{}, fmt.Errorf("DATABASE_MAX_CONNS must be positive")
	}

	if cfg.DatabaseMinConns < 0 || cfg.DatabaseMinConns > cfg.DatabaseMaxConns {
		return Config{}, fmt.Errorf("DATABASE_MIN_CONNS must be between 0 and DATABASE_MAX_CONNS")
	}

	durations := []struct {
		name   string
		target *time.Duration
	}{
		{"HTTP_READ_TIMEOUT", &cfg.HTTPReadTimeout},
		{"HTTP_READ_HEADER_TIMEOUT", &cfg.HTTPReadHeaderTimeout},
		{"HTTP_WRITE_TIMEOUT", &cfg.HTTPWriteTimeout},
		{"HTTP_IDLE_TIMEOUT", &cfg.HTTPIdleTimeout},
		{"SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout},
		{"DATABASE_MAX_CONN_LIFETIME", &cfg.DatabaseMaxConnLifetime},
		{"DATABASE_CONNECT_TIMEOUT", &cfg.DatabaseConnectTimeout},
		{"DATABASE_QUERY_TIMEOUT", &cfg.DatabaseQueryTimeout},
		{"DATABASE_ROLLBACK_TIMEOUT", &cfg.DatabaseRollbackTimeout},
		{"IDEMPOTENCY_TTL", &cfg.IdempotencyTTL},
	}

	for _, settings := range durations {
		value, err := readPositiveDuration(settings.name)
		if err != nil {
			return Config{}, err
		}

		*settings.target = value
	}

	if cfg.IdempotencyTTL < time.Microsecond {
		return Config{}, fmt.Errorf(
			"IDEMPOTENCY_TTL must be at least 1 microsecond",
		)
	}

	if err := cfg.loadTracing(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func readInt32(name string) (int32, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}

	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid int32: %w", name, err)
	}

	return int32(value), nil
}

func readPositiveDuration(name string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}

	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}

	return value, nil
}

func readPositiveInt64(name string) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid int64: %w", name, err)
	}

	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}

	return value, nil
}

func (cfg *Config) loadTracing() error {
	cfg.OTelEndpoint = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	endpoint, err := url.Parse(cfg.OTelEndpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT must be an HTTP or HTTPS URL")
	}
	cfg.OTelProtocol = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"))
	if cfg.OTelProtocol != "grpc" && cfg.OTelProtocol != "http/protobuf" {
		return fmt.Errorf("OTEL_EXPORTER_OTLP_PROTOCOL must be grpc or http/protobuf")
	}
	cfg.OTelSampler = strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER"))
	if cfg.OTelSampler == "" {
		cfg.OTelSampler = "parentbased_always_on"
	}
	cfg.OTelSamplerRatio = 1
	if raw := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG")); raw != "" {
		cfg.OTelSamplerRatio, err = strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(cfg.OTelSamplerRatio) || cfg.OTelSamplerRatio < 0 || cfg.OTelSamplerRatio > 1 {
			return fmt.Errorf("OTEL_TRACES_SAMPLER_ARG must be between 0 and 1")
		}
	}
	return nil
}
