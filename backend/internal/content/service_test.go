package content

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ai-workbench/internal/identity"
	"ai-workbench/internal/store"
)

func TestRefreshNewsListAndFavorite(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(writer, `<?xml version="1.0"?><rss><channel><item><title>New &amp; useful model</title><link>https://example.com/model</link><description><![CDATA[<p>A <strong>useful</strong> release.</p>]]></description><pubDate>Mon, 11 Aug 2025 10:00:00 +0000</pubDate><author>Research team</author></item></channel></rss>`)
	}))
	defer feed.Close()
	database := testStore(t)
	service := New(database, []Source{{Code: "test", Name: "Test Feed", URL: feed.URL}}, "", "", 24*time.Hour)
	state, err := service.RefreshNews(context.Background())
	if err != nil || state.ItemsFetched != 1 || state.LastSuccessAt == nil {
		t.Fatalf("unexpected refresh result: %#v, %v", state, err)
	}
	actor := identity.Actor{ID: "alice"}
	result, err := service.News(actor, "useful", "test", false)
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("unexpected news result: %#v, %v", result, err)
	}
	if result.Items[0].Summary != "A useful release." || result.Items[0].URL != "https://example.com/model" {
		t.Fatalf("unexpected normalized item: %#v", result.Items[0])
	}
	if err := service.FavoriteNews(actor, result.Items[0].ID, true); err != nil {
		t.Fatal(err)
	}
	favorites, err := service.News(actor, "", "", true)
	if err != nil || len(favorites.Items) != 1 || !favorites.Items[0].Favorite {
		t.Fatalf("unexpected favorites: %#v, %v", favorites, err)
	}
}

func TestAtomFeedAndXPeopleRefresh(t *testing.T) {
	atom := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = fmt.Fprint(writer, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><title>Agent update</title><link rel="alternate" href="https://example.com/agent"/><summary>Details</summary><published>2025-08-11T10:00:00Z</published><author><name>Lab</name></author></entry></feed>`)
	}))
	defer atom.Close()
	client := newFeedClient()
	items, err := client.fetch(context.Background(), Source{Code: "atom", Name: "Atom", URL: atom.URL})
	if err != nil || len(items) != 1 || items[0].URL != "https://example.com/agent" {
		t.Fatalf("unexpected Atom result: %#v, %v", items, err)
	}

	xAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/users/by/username/btibor91":
			_, _ = fmt.Fprint(writer, `{"data":{"id":"42","name":"Tibor Blaho","username":"btibor91","profile_image_url":"https://example.com/tibor.jpg"}}`)
		case "/users/42/tweets":
			_, _ = fmt.Fprint(writer, `{"data":[{"id":"100","text":"Codex product update","created_at":"2025-08-11T12:00:00Z","public_metrics":{"like_count":20,"retweet_count":3,"reply_count":2}}]}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer xAPI.Close()
	database := testStore(t)
	service := New(database, []Source{{Code: "unused", Name: "Unused", URL: atom.URL}}, xAPI.URL, "secret", 24*time.Hour)
	actor := identity.Actor{ID: "alice"}
	people, err := service.People(actor)
	if err != nil || len(people.People) != 1 || people.People[0].Handle != "btibor91" || !people.XConfigured {
		t.Fatalf("unexpected default people: %#v, %v", people, err)
	}
	if _, err := service.RefreshPeople(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	posts, err := service.Posts(actor, people.People[0].ID, "Codex", false)
	if err != nil || len(posts) != 1 || posts[0].LikeCount != 20 {
		t.Fatalf("unexpected posts: %#v, %v", posts, err)
	}
	if err := service.FavoritePost(actor, posts[0].ID, true); err != nil {
		t.Fatal(err)
	}
	favorites, err := service.Posts(actor, "", "", true)
	if err != nil || len(favorites) != 1 || !favorites[0].Favorite {
		t.Fatalf("unexpected post favorites: %#v, %v", favorites, err)
	}
}

func TestPeopleValidationAndMissingXConfiguration(t *testing.T) {
	database := testStore(t)
	service := New(database, nil, "https://api.x.com/2", "", 24*time.Hour)
	actor := identity.Actor{ID: "alice"}
	if _, err := service.AddPerson(actor, "invalid handle!", ""); err != ErrInvalid {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
	if _, err := service.RefreshPeople(context.Background(), actor); err != ErrXNotConfigured {
		t.Fatalf("expected ErrXNotConfigured, got %v", err)
	}
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
