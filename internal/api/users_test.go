package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

type managedUser struct {
	User struct {
		ID                 string     `json:"id"`
		Username           string     `json:"username"`
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
func createWebUser(t *testing.T, admin *browser, username, role string) (string, *browser) {
	t.Helper()

	var created struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		TemporaryPassword string `json:"temporary_password"`
	}

	response := admin.call(http.MethodPost, "create_user", map[string]string{"username": username, "role": role})
	decodeOK(t, "create_user "+username, response, &created)

	user := admin.server.browser().loggedIn(username, created.TemporaryPassword)

	if response := user.changePassword(created.TemporaryPassword, "user password 123"); response.Code != http.StatusNoContent {
		t.Fatalf("choose a password: %d %s", response.Code, response.Body)
	}

	return created.User.ID, user
}

// The hierarchy: the superadmin manages everyone; an admin only users of the
// role user (never other admins, the superadmin or himself here); the rows and
// the user page say whom they may change.
func TestUIUsers(t *testing.T) {
	server := newLoginServer(t)
	root := server.browser().loggedIn("admin", adminPassword)
	otherID, _ := createWebUser(t, root, "ops2", store.RoleAdmin)
	adminID, admin := createWebUser(t, root, "ops", store.RoleAdmin)
	devID, _ := createWebUser(t, root, "dev", store.RoleUser)
	rootID := superadmin(t, server)

	var list struct {
		Items []managedUser `json:"items"`
	}

	response := admin.call(http.MethodGet, "list_users", nil)
	decodeOK(t, "list_users", response, &list)

	manage := map[string]bool{}

	for _, row := range list.Items {
		manage[row.User.ID] = row.CanManage

		if row.User.DisplayName == "" || row.User.CreatedAt.IsZero() {
			t.Errorf("row %+v", row)
		}
	}

	if len(manage) != 4 || manage[rootID] || manage[adminID] || manage[otherID] || !manage[devID] {
		t.Errorf("can_manage: %v", manage)
	}

	// The superadmin's page is read-only for an admin: changes to it are forbidden.
	var page managedUser

	response = admin.call(http.MethodGet, "get_user?user_id="+rootID, nil)
	decodeOK(t, "get_user", response, &page)

	if page.CanManage || page.User.Role != store.RoleSuperadmin {
		t.Errorf("superadmin for an admin: %+v", page)
	}

	for name, input := range map[string]map[string]string{
		"update_user":         {"user_id": rootID, "name": "Boss"},
		"reset_user_password": {"user_id": rootID},
		"delete_user":         {"user_id": rootID},
	} {
		if response := admin.call(http.MethodPost, name, input); response.Code != http.StatusForbidden {
			t.Errorf("%s of the superadmin: %d %s", name, response.Code, response.Body)
		}
	}

	for _, id := range []string{"bad%2Fid", "ABCDEFGH", store.NewID()} {
		if response := admin.call(http.MethodGet, "get_user?user_id="+id, nil); response.Code != http.StatusNotFound {
			t.Errorf("get_user %s: %d", id, response.Code)
		}
	}

	// A second superadmin is refused by the input's choices; a taken name at its field.
	if response := root.call(http.MethodPost, "create_user", map[string]string{"username": "second", "role": store.RoleSuperadmin}); response.Code != http.StatusBadRequest {
		t.Errorf("a second superadmin: %d %s", response.Code, response.Body)
	}

	response = root.call(http.MethodPost, "create_user", map[string]string{"username": "OPS", "role": store.RoleUser})

	if body := errorOf(t, response); response.Code != http.StatusConflict || body.Fields["username"] != "This username is taken." {
		t.Errorf("a username in another case: %d %s", response.Code, response.Body)
	}

	// Another admin is read-only for an admin: no change, reset, delete or new admin.
	for name, input := range map[string]map[string]string{
		"update_user":         {"user_id": otherID, "name": "Second admin"},
		"reset_user_password": {"user_id": otherID},
		"delete_user":         {"user_id": otherID},
	} {
		response := admin.call(http.MethodPost, name, input)

		if body := errorOf(t, response); response.Code != http.StatusForbidden || body.Error != "your role may not change this user" {
			t.Errorf("%s of another admin: %d %s", name, response.Code, response.Body)
		}
	}

	response = admin.call(http.MethodPost, "create_user", map[string]string{"username": "ops3", "role": store.RoleAdmin})

	if body := errorOf(t, response); response.Code != http.StatusForbidden || body.Fields["role"] != "Your role may not give this role." {
		t.Errorf("an admin creates an admin: %d %s", response.Code, response.Body)
	}

	response = admin.call(http.MethodPost, "update_user", map[string]string{"user_id": devID, "role": store.RoleAdmin})

	if body := errorOf(t, response); response.Code != http.StatusForbidden || body.Fields["role"] != "Your role may not give this role." {
		t.Errorf("an admin makes a user an admin: %d %s", response.Code, response.Body)
	}

	if response := admin.call(http.MethodPost, "update_user", map[string]string{"user_id": devID, "name": "Developer"}); response.Code != http.StatusOK {
		t.Fatalf("admin edits a user: %d %s", response.Code, response.Body)
	}

	// The superadmin manages admins.
	if response := root.call(http.MethodPost, "update_user", map[string]string{"user_id": otherID, "name": "Second admin", "role": store.RoleUser}); response.Code != http.StatusOK {
		t.Fatalf("the superadmin edits an admin: %d %s", response.Code, response.Body)
	}

	response = root.call(http.MethodGet, "get_user?user_id="+otherID, nil)
	decodeOK(t, "get_user", response, &page)

	if !page.CanManage || page.User.DisplayName != "Second admin" || page.User.Role != store.RoleUser {
		t.Errorf("after the change: %+v", page)
	}

	// An empty own name is the username.
	var account struct {
		DisplayName string `json:"display_name"`
	}

	response = admin.call(http.MethodPost, "update_account_name", map[string]string{"name": " "})
	decodeOK(t, "update_account_name", response, &account)

	if account.DisplayName != "ops" {
		t.Errorf("an empty name: %+v", account)
	}
}

// A reset by an admin ends the user's sessions and disconnects their
// agents; so does deleting the user.
func TestUIResetAndDeleteEndAccess(t *testing.T) {
	server := newLoginServer(t)
	root := server.browser().loggedIn("admin", adminPassword)

	for _, request := range []string{"reset_user_password", "delete_user"} {
		id, user := createWebUser(t, root, "dev-"+request[:5], store.RoleUser)
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
