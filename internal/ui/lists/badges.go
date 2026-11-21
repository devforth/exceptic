package lists

import (
	"net/url"

	"github.com/devforth/exceptic/internal/ui/helpers"
)

// The kinds of badges.
const (
	KindProject = "project"
	KindLevel   = "level"
)

// Badge is a project or level badge, the one "badge" partial for both:
// a link when URL is set, a plain label otherwise (inside checkboxes).
type Badge struct {
	Kind string // KindProject or KindLevel
	Name string
	URL  string
}

// IsProject: a project badge carries its name for the project colors.
func (badge Badge) IsProject() bool { return badge.Kind == KindProject }

// Class is the badge's CSS class.
func (badge Badge) Class() string {
	if badge.IsProject() {
		return "project"
	}

	return "badge " + helpers.LevelClass(badge.Name)
}

// Label is a badge that is not a link (inside checkboxes): {{template "badge" (label "level" .Name)}}.
func Label(kind, name string) Badge {
	return Badge{Kind: kind, Name: name}
}

// ProjectBadge links to the issues of the project (the web app's list).
func ProjectBadge(name string) Badge {
	return Badge{Kind: KindProject, Name: name, URL: "/issues?" + url.Values{"project": {name}}.Encode()}
}
