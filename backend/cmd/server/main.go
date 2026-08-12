package main

import (
	"context"
	"log"
	"net/http"

	"ai-workbench/internal/api"
	"ai-workbench/internal/config"
	"ai-workbench/internal/content"
	"ai-workbench/internal/identity"
	"ai-workbench/internal/llm"
	"ai-workbench/internal/security"
	"ai-workbench/internal/store"
	"ai-workbench/internal/workbench"
)

func main() {
	cfg := config.Load()
	if len(cfg.PeopleClientSecret) < 32 {
		log.Fatal("People OAuth Client Secret 至少需要 32 个字符")
	}
	vault, err := security.NewVault(cfg.EncryptionKey)
	if err != nil {
		log.Fatal(err)
	}
	database, err := store.Open(cfg.DatabaseDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	identities := identity.New(database, cfg.PermissionAPIBaseURL, cfg.PeopleAPIBaseURL, cfg.PeopleAuthorizeURL, cfg.PeopleClientID, cfg.PeopleClientSecret, cfg.OAuthRedirectURIs)
	service := workbench.New(database, vault, llm.New())
	contentService := content.New(database, content.DefaultSources, cfg.XAPIBaseURL, cfg.XBearerToken, cfg.ContentRefreshPeriod, content.DailySchedule{
		Hour: cfg.NewsRefreshHour, Location: cfg.NewsRefreshTimezone, Lookback: cfg.NewsLookback,
	})
	contentService.Start(context.Background())
	server := api.New(cfg.Address, cfg.AllowedOrigins, identities, service, contentService)
	log.Printf("AI Workbench 服务监听于 %s", cfg.Address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
