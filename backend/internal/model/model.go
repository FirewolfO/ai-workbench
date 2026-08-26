package model

import "time"

type DataMigration struct {
	Name      string    `gorm:"primaryKey;size:100"`
	AppliedAt time.Time `gorm:"not null"`
}

type Provider struct {
	ID                string     `gorm:"primaryKey;size:40" json:"id"`
	OwnerID           string     `gorm:"size:100;index;not null" json:"-"`
	Name              string     `gorm:"size:100;not null" json:"name"`
	BaseURL           string     `gorm:"size:500;not null" json:"baseUrl"`
	DefaultModel      string     `gorm:"size:160;not null" json:"defaultModel"`
	Protocol          string     `gorm:"size:32;not null;default:chat_completions" json:"protocol"`
	WebSearchEnabled  bool       `gorm:"not null;default:false" json:"webSearchEnabled"`
	ModelCatalogJSON  string     `gorm:"type:text;not null;default:'[]'" json:"-"`
	ModelsUpdatedAt   *time.Time `json:"modelsUpdatedAt,omitempty"`
	APIKeyCiphertext  string     `gorm:"type:text;not null" json:"-"`
	Enabled           bool       `gorm:"not null;default:true" json:"enabled"`
	Available         bool       `gorm:"not null;default:false;index" json:"available"`
	LastTestedAt      *time.Time `json:"lastTestedAt,omitempty"`
	LastTestLatencyMs int64      `gorm:"not null;default:0" json:"lastTestLatencyMs"`
	LastTestError     string     `gorm:"type:text;not null;default:''" json:"lastTestError"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	HasAPIKey         bool       `gorm:"-" json:"hasApiKey"`
	Models            []string   `gorm:"-" json:"models"`
}

type Prompt struct {
	ID          string    `gorm:"primaryKey;size:40" json:"id"`
	OwnerID     string    `gorm:"size:100;index;not null" json:"-"`
	Title       string    `gorm:"size:120;not null" json:"title"`
	Description string    `gorm:"size:300" json:"description"`
	Category    string    `gorm:"size:60;index" json:"category"`
	Content     string    `gorm:"type:text;not null" json:"content"`
	Shared      bool      `gorm:"not null;default:false;index" json:"shared"`
	Favorite    bool      `gorm:"not null;default:false" json:"favorite"`
	UseCount    int64     `gorm:"not null;default:0" json:"useCount"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	CanEdit     bool      `gorm:"-" json:"canEdit"`
	CanDelete   bool      `gorm:"-" json:"canDelete"`
}

type Conversation struct {
	ID              string    `gorm:"primaryKey;size:40" json:"id"`
	OwnerID         string    `gorm:"size:100;index;not null" json:"-"`
	Title           string    `gorm:"size:160;not null" json:"title"`
	ProviderID      string    `gorm:"size:40;index;not null" json:"providerId"`
	Model           string    `gorm:"size:160;not null" json:"model"`
	SystemPrompt    string    `gorm:"type:text" json:"systemPrompt"`
	ReasoningEffort string    `gorm:"size:12;not null;default:medium" json:"reasoningEffort"`
	Pinned          bool      `gorm:"not null;default:false;index" json:"pinned"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	Messages        []Message `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE" json:"messages,omitempty"`
	MessageCount    int64     `gorm:"-" json:"messageCount"`
	LastMessage     string    `gorm:"-" json:"lastMessage,omitempty"`
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
	AttachmentNames  string    `gorm:"type:text" json:"-"`
	Attachments      []string  `gorm:"-" json:"attachments,omitempty"`
	CreatedAt        time.Time `gorm:"index" json:"createdAt"`
}

type Session struct {
	ID          uint      `gorm:"primaryKey"`
	TokenHash   string    `gorm:"size:64;uniqueIndex;not null"`
	Username    string    `gorm:"size:100;index;not null"`
	DisplayName string    `gorm:"size:120;not null"`
	Source      string    `gorm:"size:20;not null;default:people"`
	Role        string    `gorm:"size:20;not null;default:user"`
	ExpiresAt   time.Time `gorm:"index;not null"`
	CreatedAt   time.Time
}

type InternalUser struct {
	Username     string    `gorm:"primaryKey;size:40" json:"username"`
	DisplayName  string    `gorm:"size:120;not null" json:"displayName"`
	PasswordHash string    `gorm:"size:100;not null" json:"-"`
	Role         string    `gorm:"size:20;not null;default:user" json:"role"`
	Enabled      bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Attachment struct {
	ID                string    `gorm:"primaryKey;size:40" json:"id"`
	OwnerID           string    `gorm:"size:120;index;not null" json:"-"`
	Name              string    `gorm:"size:255;not null" json:"name"`
	ContentType       string    `gorm:"size:120;not null" json:"contentType"`
	Path              string    `gorm:"size:1000;not null" json:"-"`
	DownloadTokenHash string    `gorm:"size:64;index" json:"-"`
	Size              int64     `gorm:"not null" json:"size"`
	ExpiresAt         time.Time `gorm:"index;not null" json:"expiresAt"`
	CreatedAt         time.Time `json:"createdAt"`
}

type OAuthState struct {
	ID          uint      `gorm:"primaryKey"`
	StateHash   string    `gorm:"size:64;uniqueIndex;not null"`
	RedirectURI string    `gorm:"size:500;not null"`
	ExpiresAt   time.Time `gorm:"index;not null"`
	CreatedAt   time.Time
}

type NewsArticle struct {
	ID             string    `gorm:"primaryKey;size:40" json:"id"`
	SourceCode     string    `gorm:"size:60;index;not null" json:"sourceCode"`
	SourceName     string    `gorm:"size:100;not null" json:"sourceName"`
	ExternalID     string    `gorm:"size:500" json:"-"`
	Title          string    `gorm:"size:500;not null" json:"title"`
	Summary        string    `gorm:"type:text" json:"summary"`
	ChineseSummary string    `gorm:"type:text" json:"chineseSummary"`
	URL            string    `gorm:"size:1000;uniqueIndex;not null" json:"url"`
	Author         string    `gorm:"size:200" json:"author"`
	PublishedAt    time.Time `gorm:"index;not null" json:"publishedAt"`
	FetchedAt      time.Time `gorm:"index;not null" json:"fetchedAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Favorite       bool      `gorm:"-" json:"favorite"`
}

type NewsFavorite struct {
	ID        string    `gorm:"primaryKey;size:40" json:"id"`
	OwnerID   string    `gorm:"size:100;uniqueIndex:idx_news_favorite_owner_article;not null" json:"-"`
	ArticleID string    `gorm:"size:40;uniqueIndex:idx_news_favorite_owner_article;index;not null" json:"articleId"`
	CreatedAt time.Time `json:"createdAt"`
}

type TrackedPerson struct {
	ID              string     `gorm:"primaryKey;size:40" json:"id"`
	OwnerID         string     `gorm:"size:100;uniqueIndex:idx_tracked_owner_handle;not null" json:"-"`
	Platform        string     `gorm:"size:20;not null;default:x" json:"platform"`
	Handle          string     `gorm:"size:40;uniqueIndex:idx_tracked_owner_handle;not null" json:"handle"`
	DisplayName     string     `gorm:"size:160;not null" json:"displayName"`
	ExternalUserID  string     `gorm:"size:100;index" json:"-"`
	ProfileImageURL string     `gorm:"size:1000" json:"profileImageUrl"`
	Enabled         bool       `gorm:"not null;default:true" json:"enabled"`
	LastFetchedAt   *time.Time `gorm:"index" json:"lastFetchedAt"`
	LastError       string     `gorm:"size:500" json:"lastError"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type SocialPost struct {
	ID          string    `gorm:"primaryKey;size:40" json:"id"`
	OwnerID     string    `gorm:"size:100;uniqueIndex:idx_social_owner_external;index;not null" json:"-"`
	PersonID    string    `gorm:"size:40;index;not null" json:"personId"`
	ExternalID  string    `gorm:"size:100;uniqueIndex:idx_social_owner_external;not null" json:"-"`
	Handle      string    `gorm:"size:40;index;not null" json:"handle"`
	DisplayName string    `gorm:"size:160;not null" json:"displayName"`
	Content     string    `gorm:"type:text;not null" json:"content"`
	URL         string    `gorm:"size:1000;not null" json:"url"`
	PublishedAt time.Time `gorm:"index;not null" json:"publishedAt"`
	LikeCount   int       `gorm:"not null;default:0" json:"likeCount"`
	RepostCount int       `gorm:"not null;default:0" json:"repostCount"`
	ReplyCount  int       `gorm:"not null;default:0" json:"replyCount"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Favorite    bool      `gorm:"-" json:"favorite"`
}

type SocialPostFavorite struct {
	ID        string    `gorm:"primaryKey;size:40" json:"id"`
	OwnerID   string    `gorm:"size:100;uniqueIndex:idx_post_favorite_owner_post;not null" json:"-"`
	PostID    string    `gorm:"size:40;uniqueIndex:idx_post_favorite_owner_post;index;not null" json:"postId"`
	CreatedAt time.Time `json:"createdAt"`
}

type SyncState struct {
	Key           string     `gorm:"primaryKey;size:100" json:"key"`
	LastAttemptAt *time.Time `json:"lastAttemptAt"`
	LastSuccessAt *time.Time `json:"lastSuccessAt"`
	LastError     string     `gorm:"size:500" json:"lastError"`
	ItemsFetched  int        `gorm:"not null;default:0" json:"itemsFetched"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type FrontierSnapshot struct {
	Category    string    `gorm:"primaryKey;size:20"`
	Payload     string    `gorm:"type:text;not null"`
	GeneratedAt time.Time `gorm:"index;not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
