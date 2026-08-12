package frontier

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuildSearchQuery(t *testing.T) {
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	query, err := normalizeQuery(Query{Search: "RAG evaluation", Category: "skill", Language: "TypeScript", Period: "30d"})
	if err != nil {
		t.Fatal(err)
	}
	got := buildSearchQuery(query, now)
	for _, want := range []string{`"RAG evaluation"`, "AI skill OR agent skill", "stars:>=20", `language:"TypeScript"`, "pushed:>=2026-07-13", "archived:false", "fork:false"} {
		if !strings.Contains(got, want) {
			t.Fatalf("query %q does not contain %q", got, want)
		}
	}
}

func TestNormalizeQueryRejectsUnsupportedValues(t *testing.T) {
	for _, input := range []Query{{Category: "all"}, {Period: "weekly"}, {Sort: "forks"}, {Language: "Go:stars"}, {Search: strings.Repeat("x", 81)}} {
		if _, err := normalizeQuery(input); err != ErrInvalid {
			t.Fatalf("expected ErrInvalid for %#v, got %v", input, err)
		}
	}
}

func TestDiscoverMapsRanksAndCachesRepositories(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer test-token" || request.Header.Get("X-GitHub-Api-Version") == "" {
			t.Errorf("missing GitHub headers: %#v", request.Header)
		}
		if got := request.URL.Query().Get("q"); !strings.Contains(got, "pushed:>=2026-05-14") {
			t.Errorf("unexpected query: %s", got)
		}
		writer.Header().Set("X-RateLimit-Limit", "30")
		writer.Header().Set("X-RateLimit-Remaining", "29")
		writer.Header().Set("X-RateLimit-Reset", "1786521600")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"total_count": 2,
			"items": []map[string]any{
				{"id": 1, "name": "quiet", "full_name": "acme/quiet", "html_url": "https://github.com/acme/quiet", "description": "AI tool", "stargazers_count": 500, "forks_count": 10, "pushed_at": "2026-01-01T00:00:00Z", "created_at": "2025-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z", "owner": map[string]string{"login": "acme"}},
				{"id": 2, "name": "active", "full_name": "acme/active", "html_url": "https://github.com/acme/active", "homepage": "https://example.com", "description": "Active AI tool", "stargazers_count": 5000, "forks_count": 500, "has_discussions": true, "topics": []string{"ai"}, "license": map[string]string{"spdx_id": "MIT"}, "pushed_at": "2026-08-11T00:00:00Z", "created_at": "2025-01-01T00:00:00Z", "updated_at": "2026-08-11T00:00:00Z", "owner": map[string]string{"login": "acme"}},
			},
		})
	}))
	defer server.Close()

	service := New(server.URL, "test-token")
	service.now = func() time.Time { return time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC) }
	first, err := service.Discover(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Discover(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || first.Total != 2 || second.Items[0].FullName != "acme/active" || second.Items[0].Category != "project" || second.RateLimit.Remaining != 29 || !second.GitHubTokenSet {
		t.Fatalf("unexpected result: calls=%d result=%#v", calls.Load(), second)
	}
}

func TestDiscoverReportsRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-RateLimit-Remaining", "0")
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	_, err := New(server.URL, "").Discover(context.Background(), Query{})
	if !strings.Contains(err.Error(), ErrRateLimited.Error()) {
		t.Fatalf("expected rate limit error, got %v", err)
	}
}

func TestDiscoverFallsBackToRecentCache(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) > 1 {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"total_count": 1, "items": []map[string]any{{"id": 1, "name": "cached", "full_name": "acme/cached", "html_url": "https://github.com/acme/cached"}}})
	}))
	defer server.Close()

	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	service := New(server.URL, "")
	service.now = func() time.Time { return now }
	if _, err := service.Discover(context.Background(), Query{}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(cacheTTL + time.Minute)
	result, err := service.Discover(context.Background(), Query{})
	if err != nil || !result.Stale || len(result.Items) != 1 || calls.Load() != 2 {
		t.Fatalf("expected stale cached result, calls=%d result=%#v err=%v", calls.Load(), result, err)
	}
}

func TestDiscoverTreatsNonRateLimitForbiddenAsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-RateLimit-Remaining", "10")
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	_, err := New(server.URL, "bad-token").Discover(context.Background(), Query{})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}
