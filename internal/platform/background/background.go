// Package background runs the server's periodic jobs on lanes: the jobs of a
// lane run one at a time, lanes side by side. A pass that does not stop at its
// time limit is left running beside the lane, at most one per job.
package background

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/devforth/exceptic/internal/platform/logging"
)

// Job is one periodic pass.
type Job struct {
	Name    string
	AtStart bool          // a pass when the lane starts
	First   time.Duration // the first scheduled pass, after the start
	Every   time.Duration
	Wake    <-chan struct{} // passes between the scheduled ones, which stay where they are
	Note    string          // what wakes it, for Describe
	// Timeout ends a pass's ctx; a job that stops at it gets its next pass
	// soon after the others. One that does not stop is left running, and the
	// lane goes on without it, skipping its passes until it returns.
	Timeout time.Duration
	Run     func(ctx context.Context) error
}

// Describe reads the schedule: "at start, at 5m0s then every 10m0s, after 50 MB written".
func (job Job) Describe() string {
	var parts []string

	if job.AtStart {
		parts = append(parts, "at start")
	}

	parts = append(parts, fmt.Sprintf("at %s then every %s", job.First, job.Every))

	if job.Note != "" {
		parts = append(parts, job.Note)
	}

	return strings.Join(parts, ", ")
}

// Lane is a name and the jobs that share it.
type Lane struct {
	Name string
	Jobs []Job
}

// timeoutGrace is how long a pass past its Timeout, or one running when the
// lane stops, may take to stop before the lane goes on without it.
var timeoutGrace = time.Second

// pass is one run of a job; err is set once done is closed.
type pass struct {
	done    chan struct{}
	err     error
	overran bool // ran past its time limit, so its error is reported when collected
}

// Run runs the lane's passes until ctx ends; a failed pass, or one that panicked, is logged.
func (lane Lane) Run(ctx context.Context, logger *slog.Logger) {
	next := make([]time.Time, len(lane.Jobs))
	running := make([]*pass, len(lane.Jobs)) // a pass the lane went on without

	failed := func(index int, err error) {
		logger.Error("background job failed", "lane", lane.Name, "job", lane.Jobs[index].Name, "err", err)
	}

	collect := func(index int, left *pass) {
		if left.overran && left.err != nil && !errors.Is(left.err, context.DeadlineExceeded) && !errors.Is(left.err, context.Canceled) {
			failed(index, left.err)
		}
	}

	defer func() {
		stopBy := time.After(timeoutGrace)

		for index, left := range running {
			if left == nil {
				continue
			}

			select {
			case <-left.done:
				collect(index, left)
			case <-stopBy:
				logger.Error("background job still runs as the lane stops", "lane", lane.Name, "job", lane.Jobs[index].Name)

				return
			}
		}
	}()

	run := func(index int) {
		job := lane.Jobs[index]

		if left := running[index]; left != nil {
			select {
			case <-left.done:
				running[index] = nil

				collect(index, left)
			default:
				logger.Warn("background job skipped: its last pass still runs", "lane", lane.Name, "job", job.Name)

				return
			}
		}

		passCtx, cancel := ctx, context.CancelFunc(func() {})

		if job.Timeout > 0 {
			passCtx, cancel = context.WithTimeout(ctx, job.Timeout)
		}

		current := &pass{done: make(chan struct{})}

		go func() {
			defer close(current.done)
			defer cancel()
			// A panic fails the pass: one on stored data would otherwise end
			// the process at every start.
			defer func() {
				if recovered := recover(); recovered != nil {
					current.err = logging.PanicError(recovered)
				}
			}()

			current.err = job.Run(passCtx)
		}()

		var limit <-chan time.Time

		if job.Timeout > 0 {
			limit = time.After(job.Timeout + timeoutGrace)
		}

		select {
		case <-current.done:
		case <-ctx.Done():
			running[index] = current // waited for as the lane stops

			return
		case <-limit:
			current.overran = true
			running[index] = current

			logger.Error("background job still runs past its time limit; the lane goes on", "lane", lane.Name, "job", job.Name, "limit", job.Timeout)

			return
		}

		switch {
		case ctx.Err() != nil:
		// Stopped at its time limit: the deadline, not the cancel every pass's
		// context gets once it returns.
		case errors.Is(current.err, context.DeadlineExceeded) || (current.err == nil && errors.Is(passCtx.Err(), context.DeadlineExceeded)):
			next[index] = time.Now() // goes on soon, after the passes due before it
		case current.err != nil:
			failed(index, current.err)
		}
	}

	for index, job := range lane.Jobs {
		if job.AtStart && ctx.Err() == nil {
			run(index)
		}
	}

	started := time.Now()
	// At most one waiting pass per job however often it is woken: a job is
	// in woken only while its flag is set, so the buffer never fills.
	woken := make(chan int, len(lane.Jobs))
	waiting := make([]atomic.Bool, len(lane.Jobs))

	for index, job := range lane.Jobs {
		if next[index].IsZero() {
			next[index] = started.Add(job.First)
		}

		if job.Wake != nil {
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-job.Wake:
						if waiting[index].CompareAndSwap(false, true) {
							woken <- index
						}
					}
				}
			}()
		}
	}

	timer := time.NewTimer(0)

	defer timer.Stop()

	for ctx.Err() == nil {
		soonest := 0

		for index := range next {
			if next[index].Before(next[soonest]) {
				soonest = index
			}
		}

		timer.Reset(time.Until(next[soonest]))

		select {
		case <-ctx.Done():
			return
		case index := <-woken:
			waiting[index].Store(false)
			run(index)
		case now := <-timer.C:
			for index, job := range lane.Jobs {
				if next[index].After(now) {
					continue
				}

				scheduled := next[index]

				run(index)

				if !next[index].Equal(scheduled) {
					continue // stopped at its time limit: again soon
				}

				for !next[index].After(time.Now()) {
					next[index] = next[index].Add(job.Every)
				}
			}
		}
	}
}

// Start runs every lane in a goroutine of its own; the returned function
// stops them and waits, at most timeoutGrace for a pass that does not stop.
func Start(ctx context.Context, logger *slog.Logger, lanes ...Lane) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{}, len(lanes))

	for _, lane := range lanes {
		go func() {
			defer func() { done <- struct{}{} }()

			lane.Run(ctx, logger)
		}()
	}

	return func() {
		cancel()

		for range lanes {
			<-done
		}
	}
}

// Describe lists every lane and job with its schedule, for the startup record.
func Describe(lanes ...Lane) []any {
	var fields []any

	for _, lane := range lanes {
		for _, job := range lane.Jobs {
			fields = append(fields, lane.Name+" › "+job.Name, job.Describe())
		}
	}

	return fields
}
