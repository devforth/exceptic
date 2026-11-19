package lists

import (
	"net/url"
	"slices"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/ui/helpers"
)

// The kinds of badges.
const (
	KindProject = "project"
	KindLevel   = "level"
	KindStatus  = "status"
	KindRole    = "role"
)

// Badge is a project, level, status or role badge, the one "badge" partial
// for all: a link when URL is set, a plain label otherwise (inside checkboxes
// and chips), with a × when RemoveURL is set (chips).
type Badge struct {
	Kind      string // KindProject, KindLevel, KindStatus or KindRole
	Name      string
	URL       string
	RemoveURL string
}

// IsProject: a project badge carries its name for the project colors.
func (badge Badge) IsProject() bool { return badge.Kind == KindProject }

// Status is the issue status a status badge shows; nil for other badges.
func (badge Badge) Status() *IssueStatus {
	if badge.Kind != KindStatus {
		return nil
	}

	status := StatusOf(badge.Name)

	return &status
}

// Class is the badge's CSS class.
func (badge Badge) Class() string {
	switch badge.Kind {
	case KindProject:
		return "project"
	case KindStatus:
		return "status status-" + badge.Name
	case KindRole:
		return "badge role-" + badge.Name
	}

	return "badge " + helpers.LevelClass(badge.Name)
}

// Label is a badge that is not a link (inside checkboxes): {{template "badge" (label "level" .Name)}}.
func Label(kind, name string) Badge {
	return Badge{Kind: kind, Name: name}
}

// ProjectBadge, LevelBadge and StatusBadge make badge links: within a list
// (its FilterLinks given) they filter that list, elsewhere they open the
// issues with that project, level (a log level named as the event level) or
// status.
func ProjectBadge(name string, links ...FilterLinks) Badge {
	return linkBadge(KindProject, name, name, links)
}

func LevelBadge(level string, links ...FilterLinks) Badge {
	return linkBadge(KindLevel, level, config.EventLevel(level), links)
}

func StatusBadge(status string, links ...FilterLinks) Badge {
	return linkBadge(KindStatus, status, status, links)
}

// linkBadge: kind is also the query parameter; issueValue is the value for
// the issue list.
func linkBadge(kind, name, issueValue string, links []FilterLinks) Badge {
	badge := Badge{Kind: kind, Name: name, URL: "/issues?" + url.Values{kind: {issueValue}}.Encode()}

	if len(links) > 0 {
		badge.URL = links[0].With(kind, name)
	}

	return badge
}

// IssueStatus is how the UI shows an issue status: its icon and what it means.
type IssueStatus struct {
	Name, Label, Icon, Title string
}

// IssueStatuses are the statuses of an issue, in the order of the status filter.
var IssueStatuses = []IssueStatus{
	{Name: store.StatusUnresolved, Label: "Unresolved", Icon: "●", Title: "Unresolved: " + store.StatusMeanings[store.StatusUnresolved]},
	{Name: store.StatusResolved, Label: "Resolved", Icon: "✓", Title: "Resolved: " + store.StatusMeanings[store.StatusResolved]},
	{Name: store.StatusIgnored, Label: "Ignored", Icon: "⊘", Title: "Ignored: " + store.StatusMeanings[store.StatusIgnored]},
}

// StatusFilter are the choices of the status filter: every status, then all of them.
var StatusFilter = slices.Concat(IssueStatuses, []IssueStatus{{Name: store.StatusAll, Label: "All"}})

// StatusOf describes a status; the filter value store.StatusAll has no icon or meaning.
func StatusOf(name string) IssueStatus {
	for _, status := range StatusFilter {
		if status.Name == name {
			return status
		}
	}

	return IssueStatus{Name: name, Label: name}
}
