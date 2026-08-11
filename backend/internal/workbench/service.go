package workbench

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"ai-workbench/internal/identity"
	"ai-workbench/internal/llm"
	"ai-workbench/internal/model"
	"ai-workbench/internal/security"
	"ai-workbench/internal/store"

	"gorm.io/gorm"
)

var (
	ErrInvalid  = errors.New("invalid input")
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrProvider = errors.New("provider unavailable")
)

type Service struct {
	database *store.Store
	vault    *security.Vault
	models   llm.Client
}

type ProviderInput struct {
	Name         string `json:"name"`
	BaseURL      string `json:"baseUrl"`
	DefaultModel string `json:"defaultModel"`
	APIKey       string `json:"apiKey"`
	Enabled      *bool  `json:"enabled,omitempty"`
}

type PromptInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Content     string `json:"content"`
	Favorite    *bool  `json:"favorite,omitempty"`
}

type ConversationInput struct {
	Title        string `json:"title"`
	ProviderID   string `json:"providerId"`
	Model        string `json:"model"`
	SystemPrompt string `json:"systemPrompt"`
}

type ConversationPatch struct {
	Title        *string `json:"title,omitempty"`
	ProviderID   *string `json:"providerId,omitempty"`
	Model        *string `json:"model,omitempty"`
	SystemPrompt *string `json:"systemPrompt,omitempty"`
	Pinned       *bool   `json:"pinned,omitempty"`
}

type Dashboard struct {
	ConversationCount int64                `json:"conversationCount"`
	MessageCount      int64                `json:"messageCount"`
	PromptCount       int64                `json:"promptCount"`
	ProviderCount     int64                `json:"providerCount"`
	TotalTokens       int64                `json:"totalTokens"`
	Recent            []model.Conversation `json:"recent"`
}

type ProviderTest struct {
	OK        bool  `json:"ok"`
	LatencyMs int64 `json:"latencyMs"`
}

func New(database *store.Store, vault *security.Vault, models llm.Client) *Service {
	return &Service{database: database, vault: vault, models: models}
}

func (s *Service) Dashboard(actor identity.Actor) (*Dashboard, error) {
	var result Dashboard
	queries := []struct {
		model any
		count *int64
	}{
		{&model.Conversation{}, &result.ConversationCount}, {&model.Prompt{}, &result.PromptCount}, {&model.Provider{}, &result.ProviderCount},
	}
	for _, query := range queries {
		if err := s.database.DB.Model(query.model).Where("owner_id = ?", actor.Username).Count(query.count).Error; err != nil {
			return nil, err
		}
	}
	if err := s.database.DB.Model(&model.Message{}).
		Where("conversation_id IN (?)", s.database.DB.Model(&model.Conversation{}).Select("id").Where("owner_id = ?", actor.Username)).
		Count(&result.MessageCount).Select("COALESCE(SUM(prompt_tokens + completion_tokens), 0)").Scan(&result.TotalTokens).Error; err != nil {
		return nil, err
	}
	if err := s.database.DB.Where("owner_id = ?", actor.Username).Order("updated_at DESC").Limit(5).Find(&result.Recent).Error; err != nil {
		return nil, err
	}
	for index := range result.Recent {
		s.decorateConversation(&result.Recent[index])
	}
	return &result, nil
}

func (s *Service) Providers(actor identity.Actor) ([]model.Provider, error) {
	var providers []model.Provider
	if err := s.database.DB.Where("owner_id = ?", actor.Username).Order("created_at ASC").Find(&providers).Error; err != nil {
		return nil, err
	}
	for index := range providers {
		providers[index].HasAPIKey = providers[index].APIKeyCiphertext != ""
	}
	return providers, nil
}

func (s *Service) CreateProvider(actor identity.Actor, input ProviderInput) (*model.Provider, error) {
	provider, err := s.providerFromInput(actor.Username, input, nil)
	if err != nil {
		return nil, err
	}
	provider.ID = newID("prv")
	if err := s.database.DB.Create(provider).Error; err != nil {
		return nil, err
	}
	provider.HasAPIKey = provider.APIKeyCiphertext != ""
	return provider, nil
}

func (s *Service) UpdateProvider(actor identity.Actor, id string, input ProviderInput) (*model.Provider, error) {
	current, err := s.provider(actor, id)
	if err != nil {
		return nil, err
	}
	next, err := s.providerFromInput(actor.Username, input, current)
	if err != nil {
		return nil, err
	}
	next.ID, next.CreatedAt = current.ID, current.CreatedAt
	if err := s.database.DB.Save(next).Error; err != nil {
		return nil, err
	}
	next.HasAPIKey = next.APIKeyCiphertext != ""
	return next, nil
}

func (s *Service) DeleteProvider(actor identity.Actor, id string) error {
	provider, err := s.provider(actor, id)
	if err != nil {
		return err
	}
	var count int64
	if err := s.database.DB.Model(&model.Conversation{}).Where("owner_id = ? AND provider_id = ?", actor.Username, id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: provider is used by conversations", ErrConflict)
	}
	return s.database.DB.Delete(provider).Error
}

func (s *Service) TestProvider(ctx context.Context, actor identity.Actor, id string) (*ProviderTest, error) {
	provider, err := s.provider(actor, id)
	if err != nil {
		return nil, err
	}
	key, err := s.providerKey(provider)
	if err != nil {
		return nil, err
	}
	latency, err := s.models.Test(ctx, provider.BaseURL, key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	return &ProviderTest{OK: true, LatencyMs: latency.Milliseconds()}, nil
}

func (s *Service) providerFromInput(owner string, input ProviderInput, current *model.Provider) (*model.Provider, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	input.DefaultModel = strings.TrimSpace(input.DefaultModel)
	if input.Name == "" || len(input.Name) > 100 || input.DefaultModel == "" || len(input.DefaultModel) > 160 {
		return nil, ErrInvalid
	}
	target, err := url.Parse(input.BaseURL)
	if err != nil || !target.IsAbs() || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil {
		return nil, ErrInvalid
	}
	provider := &model.Provider{OwnerID: owner, Name: input.Name, BaseURL: input.BaseURL, DefaultModel: input.DefaultModel, Enabled: true}
	if current != nil {
		provider.APIKeyCiphertext = current.APIKeyCiphertext
		provider.Enabled = current.Enabled
	}
	if input.Enabled != nil {
		provider.Enabled = *input.Enabled
	}
	if strings.TrimSpace(input.APIKey) != "" {
		provider.APIKeyCiphertext, err = s.vault.Encrypt(strings.TrimSpace(input.APIKey))
		if err != nil {
			return nil, err
		}
	}
	return provider, nil
}

func (s *Service) provider(actor identity.Actor, id string) (*model.Provider, error) {
	var provider model.Provider
	if err := s.database.DB.Where("id = ? AND owner_id = ?", id, actor.Username).First(&provider).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return &provider, nil
}

func (s *Service) providerKey(provider *model.Provider) (string, error) {
	if provider.APIKeyCiphertext == "" {
		return "", nil
	}
	return s.vault.Decrypt(provider.APIKeyCiphertext)
}

func (s *Service) Prompts(actor identity.Actor, search string) ([]model.Prompt, error) {
	query := s.database.DB.Where("owner_id = ?", actor.Username)
	if term := strings.TrimSpace(search); term != "" {
		like := "%" + term + "%"
		query = query.Where("title LIKE ? OR description LIKE ? OR content LIKE ?", like, like, like)
	}
	var prompts []model.Prompt
	err := query.Order("favorite DESC, updated_at DESC").Find(&prompts).Error
	return prompts, err
}

func (s *Service) CreatePrompt(actor identity.Actor, input PromptInput) (*model.Prompt, error) {
	prompt, err := promptFromInput(actor.Username, input)
	if err != nil {
		return nil, err
	}
	prompt.ID = newID("pmt")
	if err := s.database.DB.Create(prompt).Error; err != nil {
		return nil, err
	}
	return prompt, nil
}

func (s *Service) UpdatePrompt(actor identity.Actor, id string, input PromptInput) (*model.Prompt, error) {
	current, err := s.prompt(actor, id)
	if err != nil {
		return nil, err
	}
	prompt, err := promptFromInput(actor.Username, input)
	if err != nil {
		return nil, err
	}
	prompt.ID, prompt.CreatedAt, prompt.UseCount = current.ID, current.CreatedAt, current.UseCount
	if input.Favorite == nil {
		prompt.Favorite = current.Favorite
	}
	if err := s.database.DB.Save(prompt).Error; err != nil {
		return nil, err
	}
	return prompt, nil
}

func (s *Service) UsePrompt(actor identity.Actor, id string) (*model.Prompt, error) {
	prompt, err := s.prompt(actor, id)
	if err != nil {
		return nil, err
	}
	if err := s.database.DB.Model(prompt).UpdateColumn("use_count", gorm.Expr("use_count + 1")).Error; err != nil {
		return nil, err
	}
	prompt.UseCount++
	return prompt, nil
}

func (s *Service) DeletePrompt(actor identity.Actor, id string) error {
	prompt, err := s.prompt(actor, id)
	if err != nil {
		return err
	}
	return s.database.DB.Delete(prompt).Error
}

func promptFromInput(owner string, input PromptInput) (*model.Prompt, error) {
	input.Title, input.Content = strings.TrimSpace(input.Title), strings.TrimSpace(input.Content)
	if input.Title == "" || len(input.Title) > 120 || input.Content == "" || len(input.Content) > 20000 || len(input.Description) > 300 || len(input.Category) > 60 {
		return nil, ErrInvalid
	}
	prompt := &model.Prompt{OwnerID: owner, Title: input.Title, Description: strings.TrimSpace(input.Description), Category: strings.TrimSpace(input.Category), Content: input.Content}
	if input.Favorite != nil {
		prompt.Favorite = *input.Favorite
	}
	return prompt, nil
}

func (s *Service) prompt(actor identity.Actor, id string) (*model.Prompt, error) {
	var prompt model.Prompt
	if err := s.database.DB.Where("id = ? AND owner_id = ?", id, actor.Username).First(&prompt).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return &prompt, nil
}

func (s *Service) Conversations(actor identity.Actor, search string) ([]model.Conversation, error) {
	query := s.database.DB.Where("owner_id = ?", actor.Username)
	if term := strings.TrimSpace(search); term != "" {
		query = query.Where("title LIKE ?", "%"+term+"%")
	}
	var conversations []model.Conversation
	if err := query.Order("pinned DESC, updated_at DESC").Find(&conversations).Error; err != nil {
		return nil, err
	}
	for index := range conversations {
		s.decorateConversation(&conversations[index])
	}
	return conversations, nil
}

func (s *Service) CreateConversation(actor identity.Actor, input ConversationInput) (*model.Conversation, error) {
	provider, err := s.provider(actor, strings.TrimSpace(input.ProviderID))
	if err != nil {
		return nil, err
	}
	if !provider.Enabled {
		return nil, ErrInvalid
	}
	modelName := strings.TrimSpace(input.Model)
	if modelName == "" {
		modelName = provider.DefaultModel
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "新对话"
	}
	if len(title) > 160 || len(modelName) > 160 || len(input.SystemPrompt) > 10000 {
		return nil, ErrInvalid
	}
	conversation := &model.Conversation{ID: newID("cnv"), OwnerID: actor.Username, Title: title, ProviderID: provider.ID, Model: modelName, SystemPrompt: strings.TrimSpace(input.SystemPrompt)}
	if err := s.database.DB.Create(conversation).Error; err != nil {
		return nil, err
	}
	return conversation, nil
}

func (s *Service) Conversation(actor identity.Actor, id string) (*model.Conversation, error) {
	var conversation model.Conversation
	if err := s.database.DB.Where("id = ? AND owner_id = ?", id, actor.Username).First(&conversation).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if err := s.database.DB.Where("conversation_id = ?", id).Order("created_at ASC").Find(&conversation.Messages).Error; err != nil {
		return nil, err
	}
	conversation.MessageCount = int64(len(conversation.Messages))
	return &conversation, nil
}

func (s *Service) UpdateConversation(actor identity.Actor, id string, patch ConversationPatch) (*model.Conversation, error) {
	conversation, err := s.Conversation(actor, id)
	if err != nil {
		return nil, err
	}
	if patch.ProviderID != nil {
		provider, err := s.provider(actor, strings.TrimSpace(*patch.ProviderID))
		if err != nil || !provider.Enabled {
			return nil, ErrInvalid
		}
		conversation.ProviderID = provider.ID
		if patch.Model == nil {
			conversation.Model = provider.DefaultModel
		}
	}
	if patch.Title != nil {
		conversation.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Model != nil {
		conversation.Model = strings.TrimSpace(*patch.Model)
	}
	if patch.SystemPrompt != nil {
		conversation.SystemPrompt = strings.TrimSpace(*patch.SystemPrompt)
	}
	if patch.Pinned != nil {
		conversation.Pinned = *patch.Pinned
	}
	if conversation.Title == "" || conversation.Model == "" || len(conversation.Title) > 160 || len(conversation.Model) > 160 || len(conversation.SystemPrompt) > 10000 {
		return nil, ErrInvalid
	}
	conversation.Messages = nil
	if err := s.database.DB.Save(conversation).Error; err != nil {
		return nil, err
	}
	return s.Conversation(actor, id)
}

func (s *Service) DeleteConversation(actor identity.Actor, id string) error {
	conversation, err := s.Conversation(actor, id)
	if err != nil {
		return err
	}
	return s.database.DB.Delete(conversation).Error
}

func (s *Service) SendMessage(ctx context.Context, actor identity.Actor, conversationID, content string) (*model.Message, error) {
	content = strings.TrimSpace(content)
	if content == "" || len(content) > 20000 {
		return nil, ErrInvalid
	}
	conversation, err := s.Conversation(actor, conversationID)
	if err != nil {
		return nil, err
	}
	provider, err := s.provider(actor, conversation.ProviderID)
	if err != nil || !provider.Enabled {
		return nil, fmt.Errorf("%w: model provider is unavailable", ErrInvalid)
	}
	userMessage := &model.Message{ID: newID("msg"), ConversationID: conversation.ID, Role: "user", Content: content, Status: "completed"}
	if err := s.database.DB.Create(userMessage).Error; err != nil {
		return nil, err
	}
	if conversation.Title == "新对话" && len(conversation.Messages) == 0 {
		title := content
		if len([]rune(title)) > 28 {
			title = string([]rune(title)[:28])
		}
		conversation.Title = title
	}
	_ = s.database.DB.Model(conversation).Updates(map[string]any{"title": conversation.Title, "updated_at": time.Now().UTC()}).Error
	messages := make([]llm.Message, 0, len(conversation.Messages)+2)
	if conversation.SystemPrompt != "" {
		messages = append(messages, llm.Message{Role: "system", Content: conversation.SystemPrompt})
	}
	start := 0
	if len(conversation.Messages) > 40 {
		start = len(conversation.Messages) - 40
	}
	for _, message := range conversation.Messages[start:] {
		if message.Status == "completed" && (message.Role == "user" || message.Role == "assistant") {
			messages = append(messages, llm.Message{Role: message.Role, Content: message.Content})
		}
	}
	messages = append(messages, llm.Message{Role: "user", Content: content})
	key, err := s.providerKey(provider)
	if err != nil {
		return nil, err
	}
	result, err := s.models.Complete(ctx, llm.CompletionRequest{BaseURL: provider.BaseURL, APIKey: key, Model: conversation.Model, Messages: messages, Temperature: 0.7})
	if err != nil {
		failed := &model.Message{ID: newID("msg"), ConversationID: conversation.ID, Role: "assistant", Content: "模型响应失败，请检查模型连接后重试。", Model: conversation.Model, Status: "failed"}
		_ = s.database.DB.Create(failed).Error
		return nil, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	modelName := strings.TrimSpace(result.Model)
	if modelName == "" {
		modelName = conversation.Model
	}
	assistant := &model.Message{
		ID: newID("msg"), ConversationID: conversation.ID, Role: "assistant", Content: result.Content, Model: modelName,
		PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens, LatencyMs: result.Latency.Milliseconds(), Status: "completed",
	}
	if err := s.database.DB.Create(assistant).Error; err != nil {
		return nil, err
	}
	_ = s.database.DB.Model(conversation).Update("updated_at", time.Now().UTC()).Error
	return assistant, nil
}

func (s *Service) decorateConversation(conversation *model.Conversation) {
	_ = s.database.DB.Model(&model.Message{}).Where("conversation_id = ?", conversation.ID).Count(&conversation.MessageCount).Error
	var latest model.Message
	if s.database.DB.Where("conversation_id = ?", conversation.ID).Order("created_at DESC").First(&latest).Error == nil {
		conversation.LastMessage = latest.Content
	}
}

func newID(prefix string) string {
	value := make([]byte, 12)
	_, _ = rand.Read(value)
	return prefix + "_" + hex.EncodeToString(value)
}
