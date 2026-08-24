package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address              string
	DatabaseDSN          string
	AttachmentDir        string
	AllowedOrigins       []string
	EncryptionKey        string
	PermissionAPIBaseURL string
	PeopleAPIBaseURL     string
	PeopleAuthorizeURL   string
	PeopleClientID       string
	PeopleClientSecret   string
	OAuthRedirectURIs    []string
	XAPIBaseURL          string
	XBearerToken         string
	GitHubAPIBaseURL     string
	GitHubToken          string
	FrontierRefreshHour  int
	FrontierTimezone     *time.Location
	ContentRefreshPeriod time.Duration
	NewsRefreshHour      int
	NewsRefreshTimezone  *time.Location
	NewsLookback         time.Duration
}

func Load() Config {
	return Config{
		Address:              env("AI_WORKBENCH_ADDR", ":8087"),
		DatabaseDSN:          env("AI_WORKBENCH_DB_DSN", "ai-workbench.db"),
		AttachmentDir:        env("AI_WORKBENCH_ATTACHMENT_DIR", "data/attachments"),
		AllowedOrigins:       split(env("AI_WORKBENCH_ALLOWED_ORIGINS", "http://localhost:5181,http://127.0.0.1:5181,http://10.251.237.216:5181,http://localhost:5178,http://127.0.0.1:5178,http://10.251.237.216:5178")),
		EncryptionKey:        env("AI_WORKBENCH_ENCRYPTION_KEY", "local-ai-workbench-encryption-key-change-me"),
		PermissionAPIBaseURL: strings.TrimRight(env("AI_WORKBENCH_PERMISSION_API_BASE_URL", "http://127.0.0.1:8081/api/v1"), "/"),
		PeopleAPIBaseURL:     strings.TrimRight(env("AI_WORKBENCH_PEOPLE_API_BASE_URL", "http://127.0.0.1:8082/api/open/people"), "/"),
		PeopleAuthorizeURL:   env("AI_WORKBENCH_PEOPLE_AUTHORIZE_URL", "http://10.251.237.216:5177/oauth/authorize"),
		PeopleClientID:       env("AI_WORKBENCH_PEOPLE_CLIENT_ID", "ai-workbench-ui"),
		PeopleClientSecret:   env("AI_WORKBENCH_PEOPLE_CLIENT_SECRET", "ai-workbench-local-client-secret-change-me"),
		OAuthRedirectURIs:    split(env("AI_WORKBENCH_OAUTH_REDIRECT_URIS", "http://localhost:5181/oauth/callback,http://127.0.0.1:5181/oauth/callback,http://10.251.237.216:5181/oauth/callback")),
		XAPIBaseURL:          strings.TrimRight(env("AI_WORKBENCH_X_API_BASE_URL", "https://api.x.com/2"), "/"),
		XBearerToken:         strings.TrimSpace(os.Getenv("AI_WORKBENCH_X_BEARER_TOKEN")),
		GitHubAPIBaseURL:     strings.TrimRight(env("AI_WORKBENCH_GITHUB_API_BASE_URL", "https://api.github.com"), "/"),
		GitHubToken:          strings.TrimSpace(os.Getenv("AI_WORKBENCH_GITHUB_TOKEN")),
		FrontierRefreshHour:  integer("AI_WORKBENCH_FRONTIER_REFRESH_HOUR", 11, 0, 23),
		FrontierTimezone:     location("AI_WORKBENCH_FRONTIER_TIMEZONE", "Asia/Shanghai"),
		ContentRefreshPeriod: hours("AI_WORKBENCH_CONTENT_REFRESH_HOURS", 24),
		NewsRefreshHour:      integer("AI_WORKBENCH_NEWS_REFRESH_HOUR", 10, 0, 23),
		NewsRefreshTimezone:  location("AI_WORKBENCH_NEWS_TIMEZONE", "Asia/Shanghai"),
		NewsLookback:         hours("AI_WORKBENCH_NEWS_LOOKBACK_HOURS", 24),
	}
}

func integer(key string, fallback, minimum, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value < minimum || value > maximum {
		return fallback
	}
	return value
}

func location(key, fallback string) *time.Location {
	value, err := time.LoadLocation(env(key, fallback))
	if err != nil {
		value, _ = time.LoadLocation(fallback)
	}
	return value
}

func hours(key string, fallback int) time.Duration {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value < 1 {
		value = fallback
	}
	return time.Duration(value) * time.Hour
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func split(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}
