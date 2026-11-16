package positions

import (
	"context"
	"sync"

	"github.com/Pkhmv16/tripGO/internal/domain/trip"
)

type Task struct {
	candidate trip.PositionCandidate
	started   bool
}

type Queue struct {
	mu      sync.Mutex
	tasks   chan *Task
	pending map[trip.TripId]*Task
	closed  bool
}

func NewQueue(capacity int) *Queue {
	return &Queue{
		tasks:   make(chan *Task, capacity),
		pending: make(map[trip.TripId]*Task, capacity),
	}
}

func (q *Queue) TryEnqueue(_ context.Context, candidate trip.PositionCandidate) EnqueueResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return Dropped
	}
	if _, exists := q.pending[candidate.TripID]; exists {
		return Duplicate
	}

	task := &Task{candidate: candidate}
	select {
	case q.tasks <- task:
		q.pending[candidate.TripID] = task
		return Queued
	default:
		return Dropped
	}
}

func (q *Queue) Next(ctx context.Context) (*Task, bool) {
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return nil, false
		case task, ok := <-q.tasks:
			if !ok {
				return nil, false
			}
			if q.start(task) {
				return task, true
			}
		}
	}
	return nil, false
}

func (q *Queue) start(task *Task) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.pending[task.candidate.TripID] != task {
		return false
	}
	task.started = true
	return true
}

func (q *Queue) Cancel(id trip.TripId) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if task := q.pending[id]; task != nil && !task.started {
		delete(q.pending, id)
	}
}

func (q *Queue) Done(task *Task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	id := task.candidate.TripID
	if q.pending[id] == task {
		delete(q.pending, id)
	}
}

func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.closed {
		q.closed = true
		close(q.tasks)
	}
}
