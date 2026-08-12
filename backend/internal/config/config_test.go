package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("AI_WORKBENCH_ADDR", "")
	t.Setenv("AI_WORKBENCH_OAUTH_REDIRECT_URIS", " http://localhost/a , http://localhost/b ")
	t.Setenv("AI_WORKBENCH_NEWS_REFRESH_HOUR", "")
	t.Setenv("AI_WORKBENCH_NEWS_TIMEZONE", "")
	t.Setenv("AI_WORKBENCH_NEWS_LOOKBACK_HOURS", "")
	t.Setenv("AI_WORKBENCH_GITHUB_API_BASE_URL", "")
	t.Setenv("AI_WORKBENCH_GITHUB_TOKEN", "")
	cfg := Load()
	if cfg.Address != ":8087" || len(cfg.OAuthRedirectURIs) != 2 || cfg.PeopleClientID != "ai-workbench-ui" || cfg.PeopleAuthorizeURL != "http://10.251.237.216:5177/oauth/authorize" || cfg.NewsRefreshHour != 10 || cfg.NewsRefreshTimezone.String() != "Asia/Shanghai" || cfg.NewsLookback != 24*time.Hour || cfg.GitHubAPIBaseURL != "https://api.github.com" || cfg.GitHubToken != "" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadNewsScheduleOverridesAndRejectsInvalidValues(t *testing.T) {
	t.Setenv("AI_WORKBENCH_NEWS_REFRESH_HOUR", "8")
	t.Setenv("AI_WORKBENCH_NEWS_TIMEZONE", "Europe/London")
	t.Setenv("AI_WORKBENCH_NEWS_LOOKBACK_HOURS", "12")
	cfg := Load()
	if cfg.NewsRefreshHour != 8 || cfg.NewsRefreshTimezone.String() != "Europe/London" || cfg.NewsLookback != 12*time.Hour {
		t.Fatalf("unexpected schedule config: %#v", cfg)
	}

	t.Setenv("AI_WORKBENCH_NEWS_REFRESH_HOUR", "24")
	t.Setenv("AI_WORKBENCH_NEWS_TIMEZONE", "invalid/timezone")
	cfg = Load()
	if cfg.NewsRefreshHour != 10 || cfg.NewsRefreshTimezone.String() != "Asia/Shanghai" {
		t.Fatalf("invalid schedule should use defaults: %#v", cfg)
	}
}
