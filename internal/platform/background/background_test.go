package background

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recorder keeps the order of the passes.
type recorder struct {
	mutex  sync.Mutex
	passes []string
}

func (record *recorder) job(name string, atStart bool, first, every time.Duration, took time.Duration) Job {
	return Job{Name: name, AtStart: atStart, First: first, Every: every, Run: func(context.Context) error {
		record.mutex.Lock()
		record.passes = append(record.passes, name+">")
		record.mutex.Unlock()
		time.Sleep(took)
		record.mutex.Lock()
		record.passes = append(record.passes, "<"+name)
		record.mutex.Unlock()

		return nil
	}}
}

func (record *recorder) list() []string {
	record.mutex.Lock()

	defer record.mutex.Unlock()

	return slices.Clone(record.passes)
}

// The jobs of a lane run at start in order, then on their schedule, never two at once.
func TestLaneRunsOneAtATime(t *testing.T) {
	record := &recorder{}
	wake := make(chan struct{}, 1)
	check := record.job("check", true, 80*time.Millisecond, 80*time.Millisecond, 30*time.Millisecond)
	check.Wake = wake
	lane := Lane{Name: "database", Jobs: []Job{
		record.job("compact", true, 40*time.Millisecond, 80*time.Millisecond, 30*time.Millisecond),
		check,
	}}
	stop := Start(context.Background(), slog.New(slog.DiscardHandler), lane)

	wake <- struct{}{}

	time.Sleep(300 * time.Millisecond)
	stop()

	passes := record.list()

	if len(passes) < 8 || !slices.Equal(passes[:4], []string{"compact>", "<compact", "check>", "<check"}) {
		t.Fatalf("passes %v", passes)
	}

	for index := 0; index+1 < len(passes); index += 2 {
		if passes[index][len(passes[index])-1] != '>' || passes[index+1][0] != '<' || passes[index][:len(passes[index])-1] != passes[index+1][1:] {
			t.Fatalf("two passes at once: %v", passes)
		}
	}

	if count := slices.Index(passes[2:], "check>"); count < 0 {
		t.Fatalf("the wake ran no pass: %v", passes)
	}
}

// Wakes that come while the lane is busy add one pass of the woken job, not one per wake.
func TestWakesWhileBusyAddOnePass(t *testing.T) {
	busyWake, checkWake, markerWake := make(chan struct{}), make(chan struct{}), make(chan struct{})
	busyStarted, release, checked, markerDone := make(chan struct{}), make(chan struct{}), make(chan struct{}, 10), make(chan struct{})

	var checks atomic.Int32

	lane := Lane{Name: "database", Jobs: []Job{
		{Name: "busy", First: time.Hour, Every: time.Hour, Wake: busyWake,
			Run: func(context.Context) error { close(busyStarted); <-release; return nil }},
		{Name: "check", First: time.Hour, Every: time.Hour, Wake: checkWake,
			Run: func(context.Context) error { checks.Add(1); checked <- struct{}{}; return nil }},
		{Name: "marker", First: time.Hour, Every: time.Hour, Wake: markerWake,
			Run: func(context.Context) error { close(markerDone); return nil }},
	}}
	stop := Start(context.Background(), slog.New(slog.DiscardHandler), lane)

	defer stop()

	busyWake <- struct{}{}

	<-busyStarted

	// Wake has no buffer: the fourth send returns once the third is forwarded.
	for range 4 {
		checkWake <- struct{}{}
	}

	close(release)

	select {
	case <-checked:
	case <-time.After(2 * time.Second):
		t.Fatal("the wakes ran no pass")
	}

	// Runs after whatever passes of check are still waiting.
	markerWake <- struct{}{}

	select {
	case <-markerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the marker's wake ran no pass")
	}

	if count := checks.Load(); count != 1 {
		t.Fatalf("%d passes for wakes that came during one busy pass, want 1", count)
	}
}

func TestDescribe(t *testing.T) {
	job := Job{Name: "size check", AtStart: true, First: 10 * time.Minute, Every: 10 * time.Minute, Note: "after 50 MB written"}

	if got := job.Describe(); got != "at start, at 10m0s then every 10m0s, after 50 MB written" {
		t.Errorf("%q", got)
	}
}

// A pass that ignores its time limit is left running: the lane goes on with
// the other jobs and skips the stuck one until it returns, so stuck passes never pile up.
func TestStuckJobDoesNotStopTheLane(t *testing.T) {
	timeoutGrace = 10 * time.Millisecond

	t.Cleanup(func() { timeoutGrace = time.Second })

	var stuckPasses atomic.Int32

	release := make(chan struct{})
	record := &recorder{}
	stuck := Job{Name: "stuck", AtStart: true, First: 20 * time.Millisecond, Every: 20 * time.Millisecond, Timeout: 20 * time.Millisecond,
		Run: func(context.Context) error { stuckPasses.Add(1); <-release; return nil }}
	lane := Lane{Name: "database", Jobs: []Job{stuck, record.job("check", false, 30*time.Millisecond, 30*time.Millisecond, 0)}}
	stop := Start(context.Background(), slog.New(slog.DiscardHandler), lane)

	time.Sleep(250 * time.Millisecond)
	close(release)
	stop()

	if passes := len(record.list()) / 2; passes < 4 {
		t.Fatalf("the lane waited for the stuck job: %d passes of the other", passes)
	}

	if count := stuckPasses.Load(); count != 1 {
		t.Fatalf("%d passes of the stuck job at once, want 1", count)
	}
}

// A pass that ends in time waits for its schedule: only one stopped at its
// limit goes on soon (a pass's context is always ended once it returns).
func TestPassInTimeKeepsItsSchedule(t *testing.T) {
	var passes atomic.Int32

	quick := Job{Name: "compact", AtStart: true, First: time.Hour, Every: time.Hour, Timeout: time.Minute,
		Run: func(context.Context) error { passes.Add(1); return nil }}
	stop := Start(context.Background(), slog.New(slog.DiscardHandler), Lane{Name: "database", Jobs: []Job{quick}})

	time.Sleep(100 * time.Millisecond)
	stop()

	if count := passes.Load(); count != 1 {
		t.Fatalf("%d passes within 100ms of a job due hourly, want 1", count)
	}
}

// A pass that stops at its time limit (a resumable cleanup) goes on soon, not at its next schedule.
func TestJobAtItsLimitGoesOnSoon(t *testing.T) {
	var passes sync.WaitGroup

	passes.Add(3)

	count := 0
	slow := Job{Name: "cleanup", AtStart: true, First: time.Hour, Every: time.Hour, Timeout: 10 * time.Millisecond,
		Run: func(ctx context.Context) error {
			if count++; count <= 3 {
				defer passes.Done()
			}

			<-ctx.Done()

			return ctx.Err()
		}}
	stop := Start(context.Background(), slog.New(slog.DiscardHandler), Lane{Name: "database", Jobs: []Job{slow}})
	finished := make(chan struct{})

	go func() { passes.Wait(); close(finished) }()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Error("a pass stopped at its limit waited for its schedule")
	}

	stop()
}

// Stopping waits at most timeoutGrace for a pass that ignores ctx, not for its time limit.
func TestStopDoesNotWaitForTheTimeLimit(t *testing.T) {
	timeoutGrace = 20 * time.Millisecond

	t.Cleanup(func() { timeoutGrace = time.Second })

	started, release := make(chan struct{}), make(chan struct{})
	deaf := Job{Name: "compact", AtStart: true, First: time.Hour, Every: time.Hour, Timeout: time.Hour,
		Run: func(context.Context) error { close(started); <-release; return nil }}
	stop := Start(context.Background(), slog.New(slog.DiscardHandler), Lane{Name: "database", Jobs: []Job{deaf}})

	defer close(release)

	<-started

	stopped := make(chan struct{})

	go func() { stop(); close(stopped) }()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stopping waited for the pass's time limit")
	}
}

// A pass left running past its limit is waited for when the lane stops, and its error is logged.
func TestStopCollectsAPassLeftRunning(t *testing.T) {
	timeoutGrace = 100 * time.Millisecond

	t.Cleanup(func() { timeoutGrace = time.Second })

	var (
		finished atomic.Bool
		output   bytes.Buffer
	)

	release := make(chan struct{})
	stuck := Job{Name: "compact", AtStart: true, First: time.Hour, Every: time.Hour, Timeout: 10 * time.Millisecond,
		Run: func(context.Context) error {
			<-release
			finished.Store(true)

			return errors.New("disk full")
		}}
	stop := Start(context.Background(), slog.New(slog.NewTextHandler(&output, nil)), Lane{Name: "database", Jobs: []Job{stuck}})

	time.Sleep(300 * time.Millisecond) // past Timeout + timeoutGrace: left running

	go func() { time.Sleep(30 * time.Millisecond); close(release) }()

	stop()

	if !finished.Load() {
		t.Error("stopping did not wait for the pass left running")
	}

	if !strings.Contains(output.String(), `"background job failed"`) || !strings.Contains(output.String(), "disk full") {
		t.Errorf("the error of the pass left running was not logged:\n%s", output.String())
	}
}
