package store

import (
	"errors"
	"time"

	"ai-workbench/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct{ DB *gorm.DB }

const builtinProvidersMigration = "builtin-providers-v1"

var builtinProviders = []model.Provider{
	{ID: "prv_builtin_openai", OwnerID: "admin", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-5.6-sol", Protocol: "responses", WebSearchEnabled: true},
	{ID: "prv_builtin_gemini", OwnerID: "admin", Name: "Google Gemini", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", DefaultModel: "gemini-3.7-flash"},
	{ID: "prv_builtin_deepseek", OwnerID: "admin", Name: "DeepSeek", BaseURL: "https://api.deepseek.com", DefaultModel: "deepseek-v4-flash"},
	{ID: "prv_builtin_qwen", OwnerID: "admin", Name: "阿里云百炼 · 通义千问", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", DefaultModel: "qwen3.7-plus"},
	{ID: "prv_builtin_kimi", OwnerID: "admin", Name: "月之暗面 · Kimi", BaseURL: "https://api.moonshot.cn/v1", DefaultModel: "kimi-k2.6"},
	{ID: "prv_builtin_glm", OwnerID: "admin", Name: "智谱 · GLM", BaseURL: "https://open.bigmodel.cn/api/paas/v4", DefaultModel: "glm-5.2"},
	{ID: "prv_builtin_grok", OwnerID: "admin", Name: "xAI · Grok", BaseURL: "https://api.x.ai/v1", DefaultModel: "grok-4.6"},
	{ID: "prv_builtin_openrouter", OwnerID: "admin", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", DefaultModel: "openrouter/auto"},
}

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&model.DataMigration{},
		&model.Provider{}, &model.Prompt{}, &model.Conversation{}, &model.Message{}, &model.Session{}, &model.OAuthState{},
		&model.InternalUser{}, &model.Attachment{},
		&model.NewsArticle{}, &model.NewsFavorite{}, &model.TrackedPerson{}, &model.SocialPost{}, &model.SocialPostFavorite{}, &model.SyncState{},
		&model.FrontierSnapshot{},
	); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := db.Model(&model.Session{}).Where("expires_at > ?", now).Update("expires_at", now.AddDate(10, 0, 0)).Error; err != nil {
		return nil, err
	}
	var adminCount int64
	if err := db.Model(&model.InternalUser{}).Where("username = ?", "admin").Count(&adminCount).Error; err != nil {
		return nil, err
	}
	if adminCount == 0 {
		passwordHash, err := bcrypt.GenerateFromPassword([]byte("admin123!"), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		admin := model.InternalUser{Username: "admin", DisplayName: "系统管理员", PasswordHash: string(passwordHash), Role: "admin", Enabled: true}
		if err := db.Create(&admin).Error; err != nil {
			return nil, err
		}
	}
	return &Store{DB: db}, nil
}

func (s *Store) SeedBuiltinProviders() error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var migration model.DataMigration
		if err := tx.First(&migration, "name = ?", builtinProvidersMigration).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		now := time.Now()
		for _, provider := range builtinProviders {
			var count int64
			if err := tx.Model(&model.Provider{}).Where("LOWER(base_url) = LOWER(?)", provider.BaseURL).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			values := map[string]any{
				"id": provider.ID, "owner_id": provider.OwnerID, "name": provider.Name,
				"base_url": provider.BaseURL, "default_model": provider.DefaultModel,
				"protocol": provider.Protocol, "web_search_enabled": provider.WebSearchEnabled,
				"api_key_ciphertext": "", "enabled": false, "created_at": now, "updated_at": now,
			}
			if err := tx.Model(&model.Provider{}).Clauses(clause.OnConflict{DoNothing: true}).Create(values).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.DataMigration{Name: builtinProvidersMigration, AppliedAt: now}).Error
	})
}

func (s *Store) Close() error {
	database, err := s.DB.DB()
	if err != nil {
		return err
	}
	return database.Close()
}
