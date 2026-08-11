package store

import (
	"ai-workbench/internal/model"

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
	if err := db.AutoMigrate(&model.Provider{}, &model.Prompt{}, &model.Conversation{}, &model.Message{}, &model.Session{}, &model.OAuthState{}); err != nil {
		return nil, err
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
