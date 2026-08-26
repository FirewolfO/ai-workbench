package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai-workbench/internal/store"
)

func TestPermissionAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/permission/auth/me" || request.Header.Get("Authorization") != "Bearer platform-token" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"user": map[string]string{"id": "1", "username": "alice", "displayName": "Alice"}}})
	}))
	defer server.Close()
	database, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	client := New(database, server.URL+"/permission", server.URL+"/people", "http://people.test/oauth/authorize", "client", "secret", []string{"http://localhost/callback"})
	actor, err := client.Authenticate(context.Background(), "platform-token")
	if err != nil || actor.Username != "alice" || actor.Source != "permission" {
		t.Fatalf("Authenticate() = %#v, %v", actor, err)
	}
}

func TestAuthorizationURLRejectsUnknownRedirect(t *testing.T) {
	database, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	client := New(database, "http://permission", "http://people", "http://people.test/oauth/authorize", "client", "secret", []string{"http://localhost/callback"})
	if _, err := client.AuthorizationURL("http://evil.test/callback"); err == nil {
		t.Fatal("expected redirect URI validation error")
	}
}

func TestInternalAdminAndUserLifecycle(t *testing.T) {
	database, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	client := New(database, "http://permission", "http://people", "http://people.test/oauth/authorize", "client", "secret", []string{"http://localhost/callback"})

	adminSession, err := client.InternalLogin(context.Background(), "admin", "admin123!")
	if err != nil || !adminSession.User.IsAdmin() || adminSession.User.Source != "internal" {
		t.Fatalf("InternalLogin(admin) = %#v, %v", adminSession, err)
	}
	if adminSession.ExpiresAt.Before(time.Now().AddDate(9, 11, 0)) {
		t.Fatalf("session should remain valid until logout: %v", adminSession.ExpiresAt)
	}
	created, err := client.CreateUser(adminSession.User, UserInput{Username: "alice", DisplayName: "Alice"})
	if err != nil || created.InitialPassword != "alice@123" || created.User.Role != RoleUser {
		t.Fatalf("CreateUser() = %#v, %v", created, err)
	}
	aliceSession, err := client.InternalLogin(context.Background(), "alice", "alice@123")
	if err != nil || aliceSession.User.IsAdmin() {
		t.Fatalf("InternalLogin(alice) = %#v, %v", aliceSession, err)
	}
	if _, err := client.Users(aliceSession.User); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ordinary user must not list users: %v", err)
	}
	if _, err := client.UpdateUser(adminSession.User, "alice", UserPatch{Password: "new-password-123"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.InternalLogin(context.Background(), "alice", "alice@123"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old password should be invalid: %v", err)
	}
	if _, err := client.InternalLogin(context.Background(), "alice", "new-password-123"); err != nil {
		t.Fatalf("new password should work: %v", err)
	}
	if err := client.DeleteUser(adminSession.User, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.InternalLogin(context.Background(), "alice", "new-password-123"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("deleted user should not log in: %v", err)
	}
}

func TestInternalAdminCannotBeDeletedOrDisabled(t *testing.T) {
	database, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	client := New(database, "http://permission", "http://people", "http://people.test/oauth/authorize", "client", "secret", nil)
	admin, err := client.InternalLogin(context.Background(), "admin", "admin123!")
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	if _, err := client.UpdateUser(admin.User, "admin", UserPatch{Enabled: &disabled}); !errors.Is(err, ErrConflict) {
		t.Fatalf("admin disable error = %v", err)
	}
	if err := client.DeleteUser(admin.User, "admin"); !errors.Is(err, ErrConflict) {
		t.Fatalf("admin delete error = %v", err)
	}
}
