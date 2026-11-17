package positions

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type Pool struct {
	queue          *Queue
	push           pushGateway
	workers        int
	requestTimeout time.Duration
	logger         *slog.Logger
	tracer         trace.Tracer
	requests       metric.Int64Counter
	duration       metric.Float64Histogram
}

func NewPool(queue *Queue, push pushGateway, workers int, requestTimeout time.Duration, logger *slog.Logger, tracer trace.Tracer, requests metric.Int64Counter, duration metric.Float64Histogram) *Pool {
	return &Pool{
		queue: queue, push: push, workers: workers, requestTimeout: requestTimeout,
		logger: logger, tracer: tracer, requests: requests, duration: duration,
	}
}

func (p *Pool) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for range p.workers {
		workers.Go(func() { p.runWorker(ctx) })
	}
	workers.Wait()
}

func (p *Pool) runWorker(ctx context.Context) {
	for {
		task, ok := p.queue.Next(ctx)
		if !ok {
			return
		}
		p.process(ctx, task)
	}
}

func (p *Pool) process(ctx context.Context, task *Task) {
	defer p.queue.Done(task)

	ctx, span := p.tracer.Start(ctx, "positions.process", trace.WithNewRoot(), trace.WithLinks(trace.Link{SpanContext: task.scanSpan}))
	defer span.End()

	requestCtx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()

	started := time.Now()
	err := p.push.RequestPosition(requestCtx, task.candidate)
	elapsed := time.Since(started)

	result := "success"
	if err != nil {
		result = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, "position request failed")
		p.logger.WarnContext(ctx, "position request failed", "trip_id", task.candidate.TripID, "error", err)
	}
	attrs := metric.WithAttributes(attribute.String("result", result))
	p.requests.Add(ctx, 1, attrs)
	p.duration.Record(ctx, elapsed.Seconds(), attrs)
}
