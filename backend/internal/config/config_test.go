package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("AI_WORKBENCH_ADDR", "")
	t.Setenv("AI_WORKBENCH_OAUTH_REDIRECT_URIS", " http://localhost/a , http://localhost/b ")
	cfg := Load()
	if cfg.Address != ":8087" || len(cfg.OAuthRedirectURIs) != 2 || cfg.PeopleClientID != "ai-workbench-ui" || cfg.PeopleAuthorizeURL != "http://10.251.237.216:5177/oauth/authorize" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}
