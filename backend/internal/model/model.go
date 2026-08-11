package model

import "time"

type Provider struct {
	ID               string    `gorm:"primaryKey;size:40" json:"id"`
	OwnerID          string    `gorm:"size:100;index;not null" json:"-"`
	Name             string    `gorm:"size:100;not null" json:"name"`
	BaseURL          string    `gorm:"size:500;not null" json:"baseUrl"`
	DefaultModel     string    `gorm:"size:160;not null" json:"defaultModel"`
	APIKeyCiphertext string    `gorm:"type:text;not null" json:"-"`
	Enabled          bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	HasAPIKey        bool      `gorm:"-" json:"hasApiKey"`
}

type Prompt struct {
	ID          string    `gorm:"primaryKey;size:40" json:"id"`
	OwnerID     string    `gorm:"size:100;index;not null" json:"-"`
	Title       string    `gorm:"size:120;not null" json:"title"`
	Description string    `gorm:"size:300" json:"description"`
	Category    string    `gorm:"size:60;index" json:"category"`
	Content     string    `gorm:"type:text;not null" json:"content"`
	Favorite    bool      `gorm:"not null;default:false" json:"favorite"`
	UseCount    int64     `gorm:"not null;default:0" json:"useCount"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Conversation struct {
	ID           string    `gorm:"primaryKey;size:40" json:"id"`
	OwnerID      string    `gorm:"size:100;index;not null" json:"-"`
	Title        string    `gorm:"size:160;not null" json:"title"`
	ProviderID   string    `gorm:"size:40;index;not null" json:"providerId"`
	Model        string    `gorm:"size:160;not null" json:"model"`
	SystemPrompt string    `gorm:"type:text" json:"systemPrompt"`
	Pinned       bool      `gorm:"not null;default:false;index" json:"pinned"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Messages     []Message `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE" json:"messages,omitempty"`
	MessageCount int64     `gorm:"-" json:"messageCount"`
	LastMessage  string    `gorm:"-" json:"lastMessage,omitempty"`
}

type Message struct {
	ID               string    `gorm:"primaryKey;size:40" json:"id"`
	ConversationID   string    `gorm:"size:40;index;not null" json:"conversationId"`
	Role             string    `gorm:"size:20;not null" json:"role"`
	Content          string    `gorm:"type:text;not null" json:"content"`
	Model            string    `gorm:"size:160" json:"model,omitempty"`
	PromptTokens     int       `gorm:"not null;default:0" json:"promptTokens"`
	CompletionTokens int       `gorm:"not null;default:0" json:"completionTokens"`
	LatencyMs        int64     `gorm:"not null;default:0" json:"latencyMs"`
	Status           string    `gorm:"size:20;not null;default:completed" json:"status"`
	CreatedAt        time.Time `gorm:"index" json:"createdAt"`
}

type Session struct {
	ID          uint      `gorm:"primaryKey"`
	TokenHash   string    `gorm:"size:64;uniqueIndex;not null"`
	Username    string    `gorm:"size:100;index;not null"`
	DisplayName string    `gorm:"size:120;not null"`
	ExpiresAt   time.Time `gorm:"index;not null"`
	CreatedAt   time.Time
}

type OAuthState struct {
	ID          uint      `gorm:"primaryKey"`
	StateHash   string    `gorm:"size:64;uniqueIndex;not null"`
	RedirectURI string    `gorm:"size:500;not null"`
	ExpiresAt   time.Time `gorm:"index;not null"`
	CreatedAt   time.Time
}
