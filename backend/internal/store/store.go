package store

import (
	"ai-workbench/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Store struct{ DB *gorm.DB }

func Open(dsn string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&model.Provider{}, &model.Prompt{}, &model.Conversation{}, &model.Message{}, &model.Session{}, &model.OAuthState{},
		&model.InternalUser{}, &model.Attachment{},
		&model.NewsArticle{}, &model.NewsFavorite{}, &model.TrackedPerson{}, &model.SocialPost{}, &model.SocialPostFavorite{}, &model.SyncState{},
		&model.FrontierSnapshot{},
	); err != nil {
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

func (s *Store) Close() error {
	database, err := s.DB.DB()
	if err != nil {
		return err
	}
	return database.Close()
}
