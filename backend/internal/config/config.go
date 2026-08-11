package config

import (
	"os"
	"strings"
)

type Config struct {
	Address              string
	DatabaseDSN          string
	AllowedOrigins       []string
	EncryptionKey        string
	PermissionAPIBaseURL string
	PeopleAPIBaseURL     string
	PeopleAuthorizeURL   string
	PeopleClientID       string
	PeopleClientSecret   string
	OAuthRedirectURIs    []string
}

func Load() Config {
	return Config{
		Address:              env("AI_WORKBENCH_ADDR", ":8087"),
		DatabaseDSN:          env("AI_WORKBENCH_DB_DSN", "ai-workbench.db"),
		AllowedOrigins:       split(env("AI_WORKBENCH_ALLOWED_ORIGINS", "http://localhost:5181,http://127.0.0.1:5181,http://10.251.237.216:5181,http://localhost:5178,http://127.0.0.1:5178,http://10.251.237.216:5178")),
		EncryptionKey:        env("AI_WORKBENCH_ENCRYPTION_KEY", "local-ai-workbench-encryption-key-change-me"),
		PermissionAPIBaseURL: strings.TrimRight(env("AI_WORKBENCH_PERMISSION_API_BASE_URL", "http://127.0.0.1:8081/api/v1"), "/"),
		PeopleAPIBaseURL:     strings.TrimRight(env("AI_WORKBENCH_PEOPLE_API_BASE_URL", "http://127.0.0.1:8082/api/open/people"), "/"),
		PeopleAuthorizeURL:   env("AI_WORKBENCH_PEOPLE_AUTHORIZE_URL", "http://10.251.237.216:5177/oauth/authorize"),
		PeopleClientID:       env("AI_WORKBENCH_PEOPLE_CLIENT_ID", "ai-workbench-ui"),
		PeopleClientSecret:   env("AI_WORKBENCH_PEOPLE_CLIENT_SECRET", "ai-workbench-local-client-secret-change-me"),
		OAuthRedirectURIs:    split(env("AI_WORKBENCH_OAUTH_REDIRECT_URIS", "http://localhost:5181/oauth/callback,http://127.0.0.1:5181/oauth/callback,http://10.251.237.216:5181/oauth/callback")),
	}
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
