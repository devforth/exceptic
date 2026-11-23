package store

import (
	"errors"
	"reflect"
	"testing"

	"github.com/devforth/exceptic/internal/platform/config"
)

func TestAccessSeesAndCovers(t *testing.T) {
	all, backend, both, none := Access{}, OnlyProjects(1), OnlyProjects(2, 1, 2), OnlyProjects()

	if !all.Sees(7) || !backend.Sees(1) || backend.Sees(2) || none.Sees(1) {
		t.Error("Sees")
	}

	if !reflect.DeepEqual(both.ProjectIDs, []int{1, 2}) {
		t.Errorf("not normalized: %v", both.ProjectIDs)
	}

	for _, test := range []struct {
		name          string
		access, other Access
		want          bool
	}{
		{"all covers all", all, all, true},
		{"all covers a list", all, backend, true},
		{"a list never covers all", both, all, false},
		{"a larger list", both, backend, true},
		{"a smaller list", backend, both, false},
		{"anything covers none", none, none, true},
	} {
		if got := test.access.Covers(test.other); got != test.want {
			t.Errorf("%s: %v", test.name, got)
		}
	}
}

func TestAccessRestrict(t *testing.T) {
	for _, test := range []struct {
		name      string
		access    Access
		requested []int
		want      []int
		ok        bool
	}{
		{"all, none requested", Access{}, nil, nil, true},
		{"all, some requested", Access{}, []int{2}, []int{2}, true},
		{"a list, none requested", OnlyProjects(1, 3), nil, []int{1, 3}, true},
		{"a list, within", OnlyProjects(1, 3), []int{3}, []int{3}, true},
		{"a list, outside", OnlyProjects(1), []int{2}, nil, false},
		{"an empty list", OnlyProjects(), nil, nil, false},
	} {
		ids := test.requested

		if ok := test.access.Restrict(&ids); !reflect.DeepEqual(ids, test.want) || ok != test.ok {
			t.Errorf("%s: %v %v", test.name, ids, ok)
		}
	}
}

func TestAccessSeesEach(t *testing.T) {
	for _, test := range []struct {
		name   string
		access Access
		ids    []int
		want   bool
	}{
		{"all, none given", Access{}, nil, true},
		{"a list, none given", OnlyProjects(1), nil, false},
		{"a list, within", OnlyProjects(1, 2), []int{1, 2}, true},
		{"a list, one outside", OnlyProjects(1), []int{1, 2}, false},
	} {
		if got := test.access.SeesEach(test.ids); got != test.want {
			t.Errorf("%s: %v", test.name, got)
		}
	}
}

func TestAccessVisibleAndShown(t *testing.T) {
	projects := config.Projects{{ID: 1, Name: "backend"}, {ID: 2, Name: "frontend"}}

	if visible := OnlyProjects(2).Visible(projects); len(visible) != 1 || visible[0].Name != "frontend" {
		t.Errorf("visible %v", visible)
	}

	if visible := OnlyProjects().Visible(projects); len(visible) != 0 {
		t.Errorf("an empty list sees %v", visible)
	}

	for _, test := range []struct {
		name           string
		target, viewer Access
		want           Access
	}{
		{"every project is shown as it is", Access{}, OnlyProjects(1), Access{}},
		{"to a viewer with every project", OnlyProjects(1, 2), Access{}, OnlyProjects(1, 2)},
		{"only the viewer's part", OnlyProjects(1, 2), OnlyProjects(1, 3), OnlyProjects(1)},
		{"nothing in common", OnlyProjects(2), OnlyProjects(1), OnlyProjects()},
	} {
		if got := test.target.Shown(test.viewer); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: %+v", test.name, got)
		}
	}
}

// A limited viewer changes only their part of a user's access and keeps the rest.
func TestAccessReplace(t *testing.T) {
	for _, test := range []struct {
		name                   string
		stored, viewer, chosen Access
		want                   Access
		locked                 bool
	}{
		{"every project gives anything", OnlyProjects(2), Access{}, Access{}, Access{}, false},
		{"every project narrows", Access{}, Access{}, OnlyProjects(1), OnlyProjects(1), false},
		{"a limited viewer adds theirs", OnlyProjects(2), OnlyProjects(1), OnlyProjects(1), OnlyProjects(1, 2), false},
		{"a limited viewer removes theirs", OnlyProjects(1, 2), OnlyProjects(1), OnlyProjects(), OnlyProjects(2), false},
		{"not beyond their own", OnlyProjects(), OnlyProjects(1), OnlyProjects(1, 2), OnlyProjects(), true},
		{"never every project", OnlyProjects(1), OnlyProjects(1), Access{}, OnlyProjects(1), true},
		{"a user with every project sent back unchanged", Access{}, OnlyProjects(1), Access{}, Access{}, false},
		{"a user with every project is locked", Access{}, OnlyProjects(1), OnlyProjects(1), Access{}, true},
	} {
		got, err := test.stored.Replace(test.viewer, test.chosen)

		if !reflect.DeepEqual(got, test.want) || errors.Is(err, ErrAccessLocked) != test.locked {
			t.Errorf("%s: %+v %v", test.name, got, err)
		}
	}
}

// The superadmin sees every project whatever is stored.
func TestProjectAccess(t *testing.T) {
	limited := OnlyProjects(1)

	if !(User{Role: RoleSuperadmin, Access: limited}).ProjectAccess().All() {
		t.Error("superadmin limited")
	}

	if (User{Role: RoleAdmin, Access: limited}).ProjectAccess().All() {
		t.Error("admin not limited")
	}

	if !(User{Role: RoleAdmin, Access: limited}).CanGiveAccess(limited) || (User{Role: RoleAdmin, Access: limited}).CanGiveAccess(Access{}) {
		t.Error("CanGiveAccess")
	}
}
