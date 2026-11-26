package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

type managedUser struct {
	User struct {
		ID                 string     `json:"id"`
		Email              string     `json:"email"`
		Name               string     `json:"name"`
		DisplayName        string     `json:"display_name"`
		Role               string     `json:"role"`
		MustChangePassword bool       `json:"must_change_password"`
		CreatedAt          time.Time  `json:"created_at"`
		LastLoginAt        *time.Time `json:"last_login_at"`
	} `json:"user"`
	CanManage bool `json:"can_manage"`
}

// decodeOK decodes a 200 answer into result.
func decodeOK(t *testing.T, label string, response *httptest.ResponseRecorder, result any) {
	t.Helper()

	if response.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", label, response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), result); err != nil {
		t.Fatalf("%s: %v", label, err)
	}
}

// createWebUser adds a user from admin's web app, returns their id and a
// browser logged in as them with their own password.
func createWebUser(t *testing.T, admin *browser, email, role string) (string, *browser) {
	t.Helper()

	return createWebUserWith(t, admin, map[string]any{"email": email, "role": role})
}

// createWebUserIn is createWebUser of a user who sees only projects.
func createWebUserIn(t *testing.T, admin *browser, email string, projects []string) (string, *browser) {
	t.Helper()

	return createWebUserWith(t, admin, map[string]any{"email": email, "role": store.RoleUser, "projects": projects})
}

func createWebUserWith(t *testing.T, admin *browser, input map[string]any) (string, *browser) {
	t.Helper()

	var created struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		TemporaryPassword string `json:"temporary_password"`
	}

	email, _ := input["email"].(string)
	response := admin.call(http.MethodPost, "create_user", input)
	decodeOK(t, "create_user "+email, response, &created)

	user := admin.server.browser().loggedIn(email, created.TemporaryPassword)

	if response := user.changePassword(created.TemporaryPassword, "user password 123"); response.Code != http.StatusNoContent {
		t.Fatalf("choose a password: %d %s", response.Code, response.Body)
	}

	return created.User.ID, user
}

// The roles: an admin manages everyone, other admins too; a user sees the
// admins (read-only) and the users they share a project with, manages
// those users, and gives only the role user and their own projects; the
// rows and the user page say whom they may change.
func TestUIUsers(t *testing.T) {
	server := newLoginServer(t)
	root := server.browser().loggedIn("admin", adminPassword)
	rootID := server.adminID(t)
	otherID, other := createWebUser(t, root, "ops2@example.com", store.RoleAdmin)
	memberID, member := createWebUserIn(t, root, "member@example.com", []string{"backend"})
	devID, _ := createWebUserIn(t, root, "dev@example.com", []string{"backend", "frontend"})
	outsiderID, _ := createWebUserIn(t, root, "outsider@example.com", []string{"frontend"})

	rows := func(viewer *browser) map[string]bool {
		var list struct {
			Items []managedUser `json:"items"`
		}

		decodeOK(t, "list_users", viewer.call(http.MethodGet, "list_users", nil), &list)

		manage := map[string]bool{}

		for _, row := range list.Items {
			manage[row.User.ID] = row.CanManage

			if row.User.DisplayName == "" || row.User.CreatedAt.IsZero() {
				t.Errorf("row %+v", row)
			}
		}

		return manage
	}

	// An admin sees and manages everyone but themselves.
	if manage := rows(other); len(manage) != 5 || !manage[rootID] || manage[otherID] || !manage[memberID] || !manage[outsiderID] {
		t.Errorf("an admin's rows: %v", manage)
	}

	if response := other.call(http.MethodPost, "update_user", map[string]string{"user_id": rootID, "name": "Boss"}); response.Code != http.StatusOK {
		t.Errorf("an admin edits another admin: %d %s", response.Code, response.Body)
	}

	// A user sees who shares a project; not the admins nor the others.
	if manage := rows(member); len(manage) != 2 || manage[memberID] || !manage[devID] {
		t.Errorf("a user's rows: %v", manage)
	}

	if response := member.call(http.MethodGet, "get_user?user_id="+outsiderID, nil); response.Code != http.StatusForbidden {
		t.Errorf("a user reads one outside their projects: %d %s", response.Code, response.Body)
	}

	for name, input := range map[string]map[string]string{
		"update_user":         {"user_id": rootID, "name": "Boss"},
		"reset_user_password": {"user_id": rootID},
		"delete_user":         {"user_id": rootID},
	} {
		if body := errorOf(t, member.call(http.MethodPost, name, input)); !strings.Contains(body.Error, "is not in your projects") {
			t.Errorf("%s of an admin by a user: %+v", name, body)
		}
	}

	// A user gives the role user and only their projects.
	for label, test := range map[string]struct {
		input map[string]any
		field string
	}{
		"an admin":         {map[string]any{"email": "boss@example.com", "role": store.RoleAdmin}, "role"},
		"another project":  {map[string]any{"email": "front@example.com", "role": store.RoleUser, "projects": []string{"frontend"}}, "projects"},
		"every project":    {map[string]any{"email": "all@example.com", "role": store.RoleUser, "all_projects": true}, "projects"},
		"a user's project": {map[string]any{"email": "new@example.com", "role": store.RoleUser, "projects": []string{"backend"}}, ""},
	} {
		response := member.call(http.MethodPost, "create_user", test.input)

		if body := errorOf(t, response); (test.field == "") != (response.Code == http.StatusOK) || test.field != "" && body.Fields[test.field] == "" {
			t.Errorf("a user creates %s: %d %s", label, response.Code, response.Body)
		}
	}

	if response := member.call(http.MethodPost, "update_user", map[string]string{"user_id": devID, "role": store.RoleAdmin}); errorOf(t, response).Fields["role"] != "Your role may not give this role." {
		t.Errorf("a user makes a user an admin: %d %s", response.Code, response.Body)
	}

	for _, id := range []string{"bad%2Fid", "ABCDEFGH", store.NewID()} {
		if response := other.call(http.MethodGet, "get_user?user_id="+id, nil); response.Code != http.StatusNotFound {
			t.Errorf("get_user %s: %d", id, response.Code)
		}
	}

	// A taken email at its field, in any case; a new user's login is an email.
	response := root.call(http.MethodPost, "create_user", map[string]string{"email": "OPS2@example.com", "role": store.RoleUser})

	if body := errorOf(t, response); response.Code != http.StatusConflict || body.Fields["email"] != "This email is taken." {
		t.Errorf("an email in another case: %d %s", response.Code, response.Body)
	}

	for _, email := range []string{"", "ops4", "two words@example.com"} {
		response = root.call(http.MethodPost, "create_user", map[string]string{"email": email, "role": store.RoleUser})

		if body := errorOf(t, response); response.Code != http.StatusBadRequest || body.Fields["email"] != "Enter a valid email address." {
			t.Errorf("create_user with email %q: %d %s", email, response.Code, response.Body)
		}
	}

	// An admin changes a user's email: an email, not taken in any case; the login follows it.
	for email, want := range map[string]int{"developer": http.StatusBadRequest, "OPS2@example.com": http.StatusConflict} {
		response = other.call(http.MethodPost, "update_user", map[string]string{"user_id": devID, "email": email})

		if body := errorOf(t, response); response.Code != want || body.Fields["email"] == "" {
			t.Errorf("update_user email %q: %d %s", email, response.Code, response.Body)
		}
	}

	if response := other.call(http.MethodPost, "update_user", map[string]string{"user_id": devID, "email": " Developer@Example.com ", "name": "Developer"}); response.Code != http.StatusOK {
		t.Fatalf("an admin changes a user's email: %d %s", response.Code, response.Body)
	}

	var page managedUser

	decodeOK(t, "get_user", other.call(http.MethodGet, "get_user?user_id="+devID, nil), &page)

	if page.User.Email != "developer@example.com" || page.User.DisplayName != "Developer" {
		t.Errorf("after the email change: %+v", page.User)
	}

	if server.browser().login("dev@example.com", "user password 123").Code != http.StatusUnauthorized ||
		server.browser().login("developer@example.com", "user password 123").Code != http.StatusOK {
		t.Error("the login did not move to the new email")
	}

	// An admin makes another admin a user.
	if response := other.call(http.MethodPost, "update_user", map[string]string{"user_id": rootID, "role": store.RoleUser}); response.Code != http.StatusOK {
		t.Fatalf("an admin demotes another admin: %d %s", response.Code, response.Body)
	}

	// An empty own name is the email.
	var account struct {
		DisplayName string `json:"display_name"`
	}

	response = other.call(http.MethodPost, "update_account_name", map[string]string{"name": " "})
	decodeOK(t, "update_account_name", response, &account)

	if account.DisplayName != "ops2" {
		t.Errorf("an empty name: %+v", account)
	}
}

// A reset by an admin ends the user's sessions and disconnects their
// agents; so does deleting the user.
func TestUIResetAndDeleteEndAccess(t *testing.T) {
	server := newLoginServer(t)
	root := server.browser().loggedIn("admin", adminPassword)

	for _, request := range []string{"reset_user_password", "delete_user"} {
		id, user := createWebUser(t, root, "dev-"+request[:5]+"@example.com", store.RoleUser)
		token, err := server.app.OAuth.NewPersonalToken(context.Background(), id)

		if err != nil {
			t.Fatal(err)
		}

		if response := root.call(http.MethodPost, request, map[string]string{"user_id": id}); response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", request, response.Code, response.Body)
		}

		if user.open() {
			t.Errorf("%s: the session survived", request)
		}

		if response := mcpTools(server, token); response.Code != http.StatusUnauthorized {
			t.Errorf("%s: the agent still works: %d", request, response.Code)
		}
	}
}

// A user of the role user has a project: none is refused at creation and
// when a change would leave none; a part the viewer does not see counts.
func TestUserNeedsAProject(t *testing.T) {
	server := newLoginServer(t)
	root := server.browser().loggedIn("admin", adminPassword)

	response := root.call(http.MethodPost, "create_user", map[string]any{"email": "none@example.com", "role": store.RoleUser, "projects": []string{}})

	if body := errorOf(t, response); response.Code != http.StatusBadRequest || body.Fields["projects"] != "Choose at least one project." {
		t.Errorf("create without projects: %d %s", response.Code, response.Body)
	}

	devID, _ := createWebUserIn(t, root, "dev@example.com", []string{"backend", "frontend"})

	response = root.call(http.MethodPost, "update_user", map[string]any{"user_id": devID, "projects": []string{}})

	if body := errorOf(t, response); response.Code != http.StatusBadRequest || body.Fields["projects"] != "Choose at least one project." {
		t.Errorf("an update to no project: %d %s", response.Code, response.Body)
	}

	// A backend user takes backend away: frontend, which they do not see, stays.
	_, member := createWebUserIn(t, root, "member@example.com", []string{"backend"})

	if response := member.call(http.MethodPost, "update_user", map[string]any{"user_id": devID, "projects": []string{}}); response.Code != http.StatusOK {
		t.Errorf("taking the viewer's part: %d %s", response.Code, response.Body)
	}
}
