package ingest

import (
	"context"
	"net/http"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/logging"
	"github.com/devforth/exceptic/internal/storage/store"
)

// shareRefresh is how often the projects with stored data are read again;
// a project whose data was all deleted counts as active for at most that long.
const shareRefresh = time.Minute

// errInboxFull: the inbox holds MaxInboxBytes; errOverShare: the project
// holds its share of it (inboxShare), while the other projects are still
// accepted. A project with nothing queued is always accepted, so the shares
// count only under load (database.InboxLimit, checked and added under one lock).
var (
	errInboxFull = &requestError{status: http.StatusTooManyRequests, message: "server busy", retryAfter: inboxFullRetry}
	errOverShare = &requestError{status: http.StatusTooManyRequests, message: "server busy: the project's share of the queue is full", retryAfter: inboxFullRetry}
)

// inboxShare is the bytes of the inbox a project may hold: MaxInboxBytes
// split among the active projects by their disk shares (100% weighs 1, 50%
// a half), the project itself always counted. Active: not paused, and
// connected (FirstContact, which a DSN reset clears) or holding data.
// The floor is config.MinInboxSharePercent of the inbox, and at least
// MinInboxShare (one largest envelope, MaxBodyBytes), so a project of a small
// share among many still queues enough, unless the whole inbox is smaller;
// the floors may add up to more than the inbox, whose own limit still holds.
func (pipeline *Pipeline) inboxShare(project config.Project) int64 {
	active := pipeline.withData.Load()
	own := shareWeight(project)
	total := own

	for _, other := range pipeline.handler.projects.List() {
		if other.ID == project.ID || other.Paused {
			continue
		}

		_, connected := pipeline.store.FirstContact(other.ID)

		if connected || (active != nil && (*active)[other.ID]) {
			total += shareWeight(other)
		}
	}

	share := int64(float64(pipeline.MaxInboxBytes) * float64(own) / float64(total))

	floor := max(pipeline.MinInboxShare, pipeline.MaxInboxBytes*config.MinInboxSharePercent/100)

	return max(share, min(floor, pipeline.MaxInboxBytes))
}

// shareWeight is the project's disk share in percent.
func shareWeight(project config.Project) int64 {
	return int64(store.Project{DiskShare: project.DiskShare}.Share())
}

// refreshShares reads the projects with data at once, then every shareRefresh until ctx ends.
func (pipeline *Pipeline) refreshShares(ctx context.Context) {
	ticker := time.NewTicker(shareRefresh)

	defer ticker.Stop()

	for {
		pipeline.readWithData(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// readWithData reads the projects with an issue (from the issue counts) or a
// log (one seek each); a failure, or a panic, keeps the last set.
func (pipeline *Pipeline) readWithData(ctx context.Context) {
	defer func() {
		if recovered := recover(); recovered != nil {
			pipeline.logger.Error("read projects with data panicked", "err", logging.PanicError(recovered))
		}
	}()

	projects := pipeline.handler.projects.List()
	ids := make([]int, len(projects))

	for index, project := range projects {
		ids[index] = project.ID
	}

	issues, err := pipeline.store.IssueProjects(ctx)

	if err != nil {
		pipeline.logger.Error("read projects with issues", "err", err)

		return
	}

	logs, err := pipeline.store.LogProjects(ctx, ids)

	if err != nil {
		pipeline.logger.Error("read projects with logs", "err", err)

		return
	}

	active := map[int]bool{}

	for _, id := range append(issues, logs...) {
		active[id] = true
	}

	pipeline.withData.Store(&active)
}
