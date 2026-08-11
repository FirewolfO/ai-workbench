package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
