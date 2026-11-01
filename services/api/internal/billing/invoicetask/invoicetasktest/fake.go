// Package invoicetasktest has a fake invoicetask.Enqueuer for tests.
package invoicetasktest

import (
	"context"
	"slices"
	"sync"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask"
)

// Fake records the tasks it is asked to enqueue, in order, and runs nothing.
type Fake struct {
	mu    sync.Mutex
	tasks []invoicetask.Task
	err   error
}

// Enqueue implements invoicetask.Enqueuer. While FailWith has set an error it returns that error
// and records nothing.
func (f *Fake) Enqueue(_ context.Context, t invoicetask.Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.tasks = append(f.tasks, t)
	return nil
}

// Tasks returns the tasks enqueued so far.
func (f *Fake) Tasks() []invoicetask.Task {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.tasks)
}

// FailWith makes the next enqueues fail with err; nil makes them succeed again.
func (f *Fake) FailWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}
