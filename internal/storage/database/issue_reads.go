package database

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Issues are read through their indexes (schema.go: seen, first, count,
// expiry) and tallies, never all at once: a list walks the prefixes that
// hold issues, newest first, as logs are walked.

// tallyPrefix is a stored status, project and level that holds issues.
type tallyPrefix struct {
	status  string
	project int
	level   string
	count   int64
}

// issuePrefixes are the prefixes with issues that filter may match: its
// projects and levels, and the stored statuses that read as its status (an
// ignore whose time has passed reads as unresolved, stored as ignored).
func (database *Store) issuePrefixes(filter store.IssueFilter) []tallyPrefix {
	statuses := store.IssueStatuses

	switch filter.Status {
	case store.StatusUnresolved:
		statuses = []string{store.StatusUnresolved, store.StatusIgnored}
	case store.StatusIgnored, store.StatusResolved:
		statuses = []string{filter.Status}
	}

	var prefixes []tallyPrefix

	for key, count := range database.db.Tallies() {
		decoder := kv.Decode(kv.Key(key), 0)

		if decoder.Text() != "issue" {
			continue
		}

		project, status, level := int(decoder.U32()), store.IssueStatuses[decoder.U8()], levelOfRank(decoder.U8())

		if (len(filter.ProjectIDs) == 0 || slices.Contains(filter.ProjectIDs, project)) &&
			(len(filter.Levels) == 0 || slices.Contains(filter.Levels, level)) && slices.Contains(statuses, status) {
			prefixes = append(prefixes, tallyPrefix{status: status, project: project, level: level, count: count})
		}
	}

	// In key order, so equal values merge the same way every time.
	slices.SortFunc(prefixes, func(first, second tallyPrefix) int {
		return cmp.Or(cmp.Compare(statusRank(first.status), statusRank(second.status)), cmp.Compare(first.project, second.project),
			cmp.Compare(levelRank(first.level), levelRank(second.level)))
	})

	return prefixes
}

// levelOfRank is the event level whose rank in the keys is rank.
func levelOfRank(rank byte) string {
	for _, level := range config.Levels {
		if levelRank(level) == rank {
			return level
		}
	}

	return config.LogLevels[min(int(rank), len(config.LogLevels)-1)]
}

// issueMatch keeps the issues of a walk that filter matches, as they read at now.
func (database *Store) issueMatch(filter store.IssueFilter, now time.Time) func(issueRecord) bool {
	return func(issue issueRecord) bool {
		switch matches(&issue, filter, now) {
		case isMatch:
			return true
		case checkEvents:
			found, err := database.anyEventInRange(issue.ID, filter)

			return err == nil && found
		default:
			return false
		}
	}
}

// issueSort is the index a sort walks and its direction.
func issueSort(sort string) (index string, newest bool) {
	switch sort {
	case store.SortLastSeenAsc:
		return "seen", false
	case store.SortFirstSeen:
		return "first", true
	case store.SortFirstSeenAsc:
		return "first", false
	case store.SortEventCount:
		return "count", true
	case store.SortEventCountAsc:
		return "count", false
	default: // last seen; by project: last seen within each
		return "seen", true
	}
}

// walkIssues visits the issues filter matches in its sort until visit
// returns false or the search time (config.LogSearchTime) runs out.
func (database *Store) walkIssues(ctx context.Context, filter store.IssueFilter, visit func(issue issueRecord) bool) error {
	now := database.now()
	budget := searchBudget{Context: ctx, now: database.now, deadline: now.Add(config.LogSearchTime)}
	match := database.issueMatch(filter, now)
	index, newest := issueSort(filter.Sort)
	prefixes := database.issuePrefixes(filter)
	stopped := false

	walk := func(prefixes []tallyPrefix) error {
		queries := make([]kv.Query[issueRecord], 0, len(prefixes))

		for _, prefix := range prefixes {
			queries = append(queries, database.issues.By(index, kv.U8(statusRank(prefix.status)), kv.U32(uint32(prefix.project)),
				kv.U8(levelRank(prefix.level))).Where(match).WithContext(budget))
		}

		return kv.Merge(database.db, queries, newest, func(entry kv.Entry[issueRecord]) (bool, error) {
			if !visit(entry.Record) {
				stopped = true
			}

			return !stopped, nil
		})
	}

	if filter.Sort != store.SortProject && filter.Sort != store.SortProjectDesc {
		return walkError(walk(prefixes))
	}

	projects, err := database.ListProjects(ctx)

	if err != nil {
		return err
	}

	names := map[int]string{}

	for _, project := range projects {
		names[project.ID] = strings.ToLower(project.Name)
	}

	byProject := map[int][]tallyPrefix{}

	for _, prefix := range prefixes {
		byProject[prefix.project] = append(byProject[prefix.project], prefix)
	}

	order := slices.SortedFunc(func(yield func(int) bool) {
		for project := range byProject {
			if !yield(project) {
				return
			}
		}
	}, func(first, second int) int {
		return cmp.Or(cmp.Compare(names[first], names[second]), cmp.Compare(first, second))
	})

	if filter.Sort == store.SortProjectDesc {
		slices.Reverse(order)
	}

	for _, project := range order {
		if err := walk(byProject[project]); err != nil || stopped {
			return walkError(err)
		}
	}

	return nil
}

// walkError: a walk the search time ended gives what it found.
func walkError(err error) error {
	if errors.Is(err, errSearchTime) {
		return nil
	}

	return err
}

// ListIssues walks the page of the issues that match: at most
// store.LogCountLimit deep, as lists of logs.
func (database *Store) ListIssues(ctx context.Context, filter store.IssueFilter) ([]store.Issue, error) {
	offset := min(max(filter.Offset, 0), store.LogCountLimit)
	issues := []store.Issue{}
	now := database.now()

	err := database.walkIssues(ctx, filter, func(issue issueRecord) bool {
		if offset > 0 {
			offset--

			return true
		}

		issues = append(issues, issue.At(now))

		return filter.Limit <= 0 || len(issues) < filter.Limit
	})

	return issues, err
}

// CountIssues counts the issues that match, up to store.LogCountLimit
// (more: there are more). Without text or dates the tallies answer it.
func (database *Store) CountIssues(ctx context.Context, filter store.IssueFilter) (int, bool, error) {
	if filter.Query == "" && filter.SeenSince.IsZero() && filter.SeenUntil.IsZero() {
		count, err := database.tallied(filter)

		// A list pages no deeper than the limit, so its count stops there too.
		if count > store.LogCountLimit {
			return store.LogCountLimit, true, err
		}

		return count, false, err
	}

	count := 0

	err := database.walkIssues(ctx, filter, func(issueRecord) bool {
		count++

		return count <= store.LogCountLimit
	})

	if count > store.LogCountLimit {
		return store.LogCountLimit, true, err
	}

	return count, false, err
}

// tallied counts from the tallies the issues of a filter without text or
// dates; the ignores whose time has passed move from ignored to unresolved.
func (database *Store) tallied(filter store.IssueFilter) (int, error) {
	count := 0

	for _, prefix := range database.issuePrefixes(filter) {
		if prefix.status != store.StatusIgnored || filter.Status != store.StatusUnresolved {
			count += int(prefix.count)
		}
	}

	if filter.Status != store.StatusUnresolved && filter.Status != store.StatusIgnored {
		return count, nil
	}

	expired, err := database.expiredIgnores(filter.ProjectIDs, filter.Levels)

	if filter.Status == store.StatusUnresolved {
		return count + expired, err
	}

	return count - expired, err
}

// expiredIgnores counts the ignored issues of the projects and levels (none:
// any) whose time has passed: few, as people set ignores.
func (database *Store) expiredIgnores(projectIDs []int, levels []string) (int, error) {
	now := database.now()
	count := 0

	err := database.issues.By("expiry").Each(database.db, func(entry kv.Entry[issueRecord]) (bool, error) {
		issue := entry.Record

		if issue.IgnoreExpired(now) && (len(projectIDs) == 0 || slices.Contains(projectIDs, issue.ProjectID)) &&
			(len(levels) == 0 || slices.Contains(levels, issue.Level)) {
			count++
		}

		return true, nil
	})

	return count, err
}

func (database *Store) GetIssue(_ context.Context, issueID string) (store.Issue, error) {
	issue, err := database.issues.Get(database.db, kv.K(kv.Text(issueID)))

	if errors.Is(err, kv.ErrNotFound) {
		return store.Issue{}, store.ErrNotFound
	}

	return issue.At(database.now()), err
}

func (database *Store) GetIssues(ctx context.Context, issueIDs []string) ([]store.Issue, error) {
	issues := []store.Issue{}

	for _, id := range issueIDs {
		issue, err := database.GetIssue(ctx, id)

		switch {
		case errors.Is(err, store.ErrNotFound):
		case err != nil:
			return nil, err
		default:
			issues = append(issues, issue)
		}
	}

	return issues, nil
}

// UnresolvedCounts counts the unresolved issues of every project from the tallies.
func (database *Store) UnresolvedCounts(context.Context) (map[int]int, error) {
	counts := map[int]int{}

	for _, prefix := range database.issuePrefixes(store.IssueFilter{Status: store.StatusUnresolved}) {
		if prefix.status == store.StatusUnresolved {
			counts[prefix.project] += int(prefix.count)
		}
	}

	now := database.now()

	err := database.issues.By("expiry").Each(database.db, func(entry kv.Entry[issueRecord]) (bool, error) {
		if entry.Record.IgnoreExpired(now) {
			counts[entry.Record.ProjectID]++
		}

		return true, nil
	})

	return counts, err
}

// IssueLevels are the levels some issue of the projects (none: any) has.
func (database *Store) IssueLevels(_ context.Context, projectIDs []int) ([]string, error) {
	seen := map[string]bool{}

	for _, prefix := range database.issuePrefixes(store.IssueFilter{ProjectIDs: projectIDs}) {
		seen[prefix.level] = true
	}

	levels := make([]string, 0, len(seen))

	for level := range seen {
		levels = append(levels, level)
	}

	return levels, nil
}
