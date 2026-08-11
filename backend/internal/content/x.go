package content

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ai-workbench/internal/model"
)

type xClient struct {
	baseURL string
	token   string
	http    *http.Client
}

type xUser struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Username        string `json:"username"`
	ProfileImageURL string `json:"profile_image_url"`
}

type xPost struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	Metrics   struct {
		LikeCount    int `json:"like_count"`
		RetweetCount int `json:"retweet_count"`
		ReplyCount   int `json:"reply_count"`
	} `json:"public_metrics"`
}

func newXClient(baseURL, token string) *xClient {
	return &xClient{baseURL: strings.TrimRight(baseURL, "/"), token: strings.TrimSpace(token), http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *xClient) configured() bool { return c.baseURL != "" && c.token != "" }

func (c *xClient) fetchPosts(ctx context.Context, person *model.TrackedPerson) ([]model.SocialPost, error) {
	user, err := c.lookupUser(ctx, person.Handle)
	if err != nil {
		return nil, err
	}
	person.ExternalUserID = user.ID
	person.Handle = user.Username
	person.DisplayName = user.Name
	person.ProfileImageURL = user.ProfileImageURL
	parameters := url.Values{
		"max_results":  {"20"},
		"exclude":      {"retweets,replies"},
		"tweet.fields": {"created_at,public_metrics"},
	}
	var payload struct {
		Data []xPost `json:"data"`
	}
	if err := c.get(ctx, "/users/"+url.PathEscape(user.ID)+"/tweets?"+parameters.Encode(), &payload); err != nil {
		return nil, err
	}
	posts := make([]model.SocialPost, 0, len(payload.Data))
	for _, item := range payload.Data {
		posts = append(posts, model.SocialPost{
			ID: newID("post"), ExternalID: item.ID, Content: item.Text,
			URL: "https://x.com/" + user.Username + "/status/" + item.ID, PublishedAt: item.CreatedAt,
			LikeCount: item.Metrics.LikeCount, RepostCount: item.Metrics.RetweetCount, ReplyCount: item.Metrics.ReplyCount,
		})
	}
	return posts, nil
}

func (c *xClient) lookupUser(ctx context.Context, handle string) (*xUser, error) {
	var payload struct {
		Data xUser `json:"data"`
	}
	path := "/users/by/username/" + url.PathEscape(handle) + "?user.fields=profile_image_url"
	if err := c.get(ctx, path, &payload); err != nil {
		return nil, err
	}
	if payload.Data.ID == "" {
		return nil, fmt.Errorf("X user @%s was not found", handle)
	}
	return &payload.Data, nil
}

func (c *xClient) get(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("User-Agent", "AI-Workbench/1.0")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("X API HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(target); err != nil {
		return err
	}
	return nil
}
