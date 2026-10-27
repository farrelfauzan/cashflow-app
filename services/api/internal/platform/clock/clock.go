// Package clock is the only source of "now" for business rules, so tests can move time and
// every date rule (H-3, H0, payroll day) is computed in Asia/Jakarta.
package clock

import (
	"fmt"
	"sync"
	"time"
	_ "time/tzdata" // embed zone data so Asia/Jakarta loads inside a minimal container image
)

// Jakarta is the time zone every date rule uses.
var Jakarta = mustLocation("Asia/Jakarta")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(fmt.Sprintf("clock: loading %s: %v", name, err))
	}
	return loc
}

// Clock reports the current time.
type Clock interface {
	Now() time.Time
}

type system struct{}

func (system) Now() time.Time { return time.Now() }

// System returns the real clock.
func System() Clock { return system{} }

// StartOfDay returns midnight in Jakarta on the Jakarta calendar day that contains t.
func StartOfDay(t time.Time) time.Time {
	t = t.In(Jakarta)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Jakarta)
}

// Fake is a clock that only moves when a test says so.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a fake clock set to t.
func NewFake(t time.Time) *Fake { return &Fake{now: t} }

// Now returns the fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Set moves the fake clock to t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}

// Advance moves the fake clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
