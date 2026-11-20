package positions

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type Scanner struct {
	trips            staleTripRepository
	queue            queue
	staleAfter       time.Duration
	batchSize        int
	logger           *slog.Logger
	tracer           trace.Tracer
	staleChecks      metric.Int64Counter
	positionRequests metric.Int64Counter
	rowsRead         metric.Int64Histogram
}

func NewScanner(trips staleTripRepository, queue queue, staleAfter time.Duration, batchSize int, logger *slog.Logger, tracer trace.Tracer, staleChecks, positionRequests metric.Int64Counter, rowsRead metric.Int64Histogram) *Scanner {
	return &Scanner{
		trips: trips, queue: queue, staleAfter: staleAfter, logger: logger, tracer: tracer,
		staleChecks: staleChecks, positionRequests: positionRequests,
		batchSize: batchSize, rowsRead: rowsRead,
	}
}

func (s *Scanner) Run(ctx context.Context, ticker ticker) {
	defer ticker.Stop()
	ticks := ticker.Ticks()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok || ctx.Err() != nil {
				return
			}
			s.scanOnce(ctx)
		}
	}
}

func (s *Scanner) scanOnce(ctx context.Context) {
	ctx, span := s.tracer.Start(ctx, "positions.scan")
	defer span.End()

	var rowsRead int64
	defer func() { s.rowsRead.Record(ctx, rowsRead) }()
	var after *trip.PositionCursor
	for ctx.Err() == nil {
		candidates, err := s.trips.ListStaleTrips(ctx, s.staleAfter, s.batchSize, after)
		if err != nil {
			if ctx.Err() == nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "stale trip lookup failed")
				s.logger.ErrorContext(ctx, "stale trip lookup failed", "error", err)
			}
			return
		}
		rowsRead += int64(len(candidates))
		for _, candidate := range candidates {
			if ctx.Err() != nil || s.enqueue(ctx, candidate) == Dropped {
				return
			}
		}
		if len(candidates) < s.batchSize {
			return
		}
		last := candidates[len(candidates)-1]
		after = &trip.PositionCursor{LastActivity: last.LastActivity, TripID: last.TripID}
	}
}

func (s *Scanner) enqueue(ctx context.Context, candidate trip.PositionCandidate) EnqueueResult {
	result := s.queue.TryEnqueue(ctx, candidate)
	attrs := metric.WithAttributes(attribute.String("result", string(result)))
	s.staleChecks.Add(ctx, 1, attrs)
	if result == Dropped {
		s.positionRequests.Add(ctx, 1, attrs)
		s.logger.WarnContext(ctx, "position request queue is full", "trip_id", candidate.TripID)
	}
	return result
}
