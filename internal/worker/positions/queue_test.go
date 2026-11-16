package positions

import (
	"sync"
	"testing"
	"time"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

const testTripID = "trip"

func TestQueueDeduplicatesConcurrentOffers(t *testing.T) {
	q := NewQueue(1)
	ctx := t.Context()
	candidate := trip.PositionCandidate{TripID: testTripID}
	const producers = 16
	results := make(chan EnqueueResult, producers)
	var workers sync.WaitGroup
	for range producers {
		workers.Go(func() {
			results <- q.TryEnqueue(ctx, candidate)
		})
	}
	workers.Wait()
	close(results)

	counts := make(map[EnqueueResult]int)
	for result := range results {
		counts[result]++
	}
	if counts[Queued] != 1 || counts[Duplicate] != producers-1 {
		t.Fatalf("enqueue outcomes = %v", counts)
	}
}

func TestQueueReportsFullAndAllowsRetry(t *testing.T) {
	q := NewQueue(1)
	ctx := t.Context()
	first := trip.PositionCandidate{TripID: "first"}
	second := trip.PositionCandidate{TripID: "second"}
	if got := q.TryEnqueue(ctx, first); got != Queued {
		t.Fatalf("first enqueue = %q, want queued", got)
	}

	result := make(chan EnqueueResult, 1)
	go func() { result <- q.TryEnqueue(ctx, second) }()
	select {
	case got := <-result:
		if got != Dropped {
			t.Fatalf("full queue outcome = %q, want dropped", got)
		}
	case <-time.After(time.Second):
		t.Fatal("enqueue blocked on a full queue")
	}

	if task, ok := q.Next(ctx); !ok || task.candidate != first {
		t.Fatalf("queue returned %v, %v, want first task", task, ok)
	}
	if got := q.TryEnqueue(ctx, second); got != Queued {
		t.Fatalf("retry after freeing space = %q, want queued", got)
	}
	q.Close()
	q.Close()
	if task, ok := q.Next(ctx); !ok || task.candidate != second {
		t.Fatalf("closed queue returned %v, %v, want remaining task", task, ok)
	}
	if got := q.TryEnqueue(ctx, first); got != Dropped {
		t.Fatalf("enqueue on closed queue = %q, want dropped", got)
	}
	if task, ok := q.Next(ctx); ok {
		t.Fatalf("unexpected task after draining closed queue: %v", task)
	}
}

func TestQueueSkipsCancelledTaskAfterReenqueue(t *testing.T) {
	q := NewQueue(2)
	ctx := t.Context()
	old := trip.PositionCandidate{TripID: testTripID, DriverID: "old-driver"}
	replacement := trip.PositionCandidate{TripID: old.TripID, DriverID: "new-driver"}
	if got := q.TryEnqueue(ctx, old); got != Queued {
		t.Fatalf("first enqueue = %q, want queued", got)
	}
	q.Cancel(old.TripID)
	if got := q.TryEnqueue(ctx, replacement); got != Queued {
		t.Fatalf("enqueue after cancellation = %q, want queued", got)
	}
	q.Close()

	task, ok := q.Next(ctx)
	if !ok || task.candidate != replacement {
		t.Fatalf("queue returned %v, %v, want replacement task", task, ok)
	}
	q.Done(task)
	if task, ok := q.Next(ctx); ok {
		t.Fatalf("unexpected additional task: %v", task)
	}
}

func TestQueueDeduplicatesUntilProcessingFinishes(t *testing.T) {
	q := NewQueue(1)
	ctx := t.Context()
	candidate := trip.PositionCandidate{TripID: testTripID}
	if got := q.TryEnqueue(ctx, candidate); got != Queued {
		t.Fatalf("first enqueue = %q, want queued", got)
	}

	task, ok := q.Next(ctx)
	if !ok {
		t.Fatal("expected a task")
	}
	q.Cancel(candidate.TripID)
	if got := q.TryEnqueue(ctx, candidate); got != Duplicate {
		t.Fatalf("enqueue while processing = %q, want duplicate", got)
	}

	q.Done(task)
	if got := q.TryEnqueue(ctx, candidate); got != Queued {
		t.Fatalf("enqueue after processing = %q, want queued", got)
	}
	q.Done(task)
	if got := q.TryEnqueue(ctx, candidate); got != Duplicate {
		t.Fatalf("enqueue after completing an old task = %q, want duplicate", got)
	}
}
