package workbench

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"ai-workbench/internal/identity"
	"ai-workbench/internal/llm"
	"ai-workbench/internal/model"
	"ai-workbench/internal/security"
	"ai-workbench/internal/store"
)

type fakeModels struct{}

func (fakeModels) Complete(_ context.Context, request llm.CompletionRequest) (*llm.CompletionResult, error) {
	return &llm.CompletionResult{Content: "回答", Model: request.Model, PromptTokens: 8, CompletionTokens: 3, Latency: 20 * time.Millisecond}, nil
}
func (fakeModels) Test(context.Context, string, string, string) (time.Duration, error) {
	return 10 * time.Millisecond, nil
}

func testService(t *testing.T) *Service {
	return testServiceWithModels(t, fakeModels{})
}

func testServiceWithModels(t *testing.T, models llm.Client) *Service {
	t.Helper()
	database, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	vault, err := security.NewVault("test-encryption-key-that-is-long-enough-123")
	if err != nil {
		t.Fatal(err)
	}
	return New(database, vault, models, t.TempDir())
}

type summaryModels struct{}

func (summaryModels) Complete(_ context.Context, _ llm.CompletionRequest) (*llm.CompletionResult, error) {
	return &llm.CompletionResult{Content: "```json\n[{\"id\":\"news_one\",\"summary\":\"这是一条经过压缩的中文人工智能热点概要，说明事件内容及其主要价值。\"}]\n```", Model: "model"}, nil
}
func (summaryModels) Test(context.Context, string, string, string) (time.Duration, error) {
	return 0, nil
}

func TestPersonalDataIsolationAndChat(t *testing.T) {
	service := testService(t)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	bob := identity.Actor{Username: "bob"}
	provider, err := service.CreateProvider(admin, ProviderInput{Name: "Local", BaseURL: "http://localhost:11434/v1", DefaultModel: "model", APIKey: "secret"})
	if err != nil || !provider.HasAPIKey || provider.APIKeyCiphertext == "secret" {
		t.Fatalf("CreateProvider() = %#v, %v", provider, err)
	}
	conversation, err := service.CreateConversation(alice, ConversationInput{})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{Content: "你好"})
	if err != nil || answer.Content != "回答" || answer.PromptTokens != 8 {
		t.Fatalf("SendMessage() = %#v, %v", answer, err)
	}
	if _, err := service.Conversation(bob, conversation.ID); err != ErrNotFound {
		t.Fatalf("Bob must not read Alice's conversation: %v", err)
	}
	loaded, err := service.Conversation(alice, conversation.ID)
	if err != nil || len(loaded.Messages) != 2 || loaded.Title == "新对话" {
		t.Fatalf("Conversation() = %#v, %v", loaded, err)
	}
	dashboard, err := service.Dashboard(alice)
	if err != nil || dashboard.ConversationCount != 1 || dashboard.MessageCount != 2 || dashboard.TotalTokens != 11 {
		t.Fatalf("Dashboard() = %#v, %v", dashboard, err)
	}
}

func TestPromptLifecycleAndProviderConflict(t *testing.T) {
	service := testService(t)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	actor := identity.Actor{Username: "alice", Role: identity.RoleUser}
	provider, _ := service.CreateProvider(admin, ProviderInput{Name: "Local", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	_, _ = service.CreateConversation(actor, ConversationInput{})
	if err := service.DeleteProvider(admin, provider.ID); !containsError(err, ErrConflict) {
		t.Fatalf("DeleteProvider() error = %v", err)
	}
	prompt, err := service.CreatePrompt(actor, PromptInput{Title: "总结", Content: "总结以下内容"})
	if err != nil {
		t.Fatal(err)
	}
	used, err := service.UsePrompt(actor, prompt.ID)
	if err != nil || used.UseCount != 1 {
		t.Fatalf("UsePrompt() = %#v, %v", used, err)
	}
}

func TestSummarizeNewsUsesOwnedProviderAndCachesResult(t *testing.T) {
	service := testServiceWithModels(t, summaryModels{})
	admin := identity.Actor{ID: "admin", Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	actor := identity.Actor{ID: "alice", Username: "alice", Role: identity.RoleUser}
	if _, err := service.CreateProvider(admin, ProviderInput{Name: "Local", BaseURL: "http://localhost/v1", DefaultModel: "model"}); err != nil {
		t.Fatal(err)
	}
	article := model.NewsArticle{ID: "news_one", SourceCode: "test", SourceName: "Test", Title: "Agent release", Summary: "An agent was released.", URL: "https://example.com/agent", PublishedAt: time.Now(), FetchedAt: time.Now()}
	if err := service.database.DB.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.database.DB.Model(&model.NewsArticle{}).Where("id = ?", article.ID).UpdateColumn("chinese_summary", nil).Error; err != nil {
		t.Fatal(err)
	}
	result, err := service.SummarizeNews(context.Background(), actor, []string{article.ID})
	if err != nil || result.Generated != 1 || result.Summaries[article.ID] == "" {
		t.Fatalf("SummarizeNews() = %#v, %v", result, err)
	}
	var saved model.NewsArticle
	if err := service.database.DB.First(&saved, "id = ?", article.ID).Error; err != nil || saved.ChineseSummary != result.Summaries[article.ID] {
		t.Fatalf("unexpected cached summary: %#v, %v", saved, err)
	}
	again, err := service.SummarizeNews(context.Background(), actor, []string{article.ID})
	if err != nil || again.Generated != 0 {
		t.Fatalf("cached summary should not be regenerated: %#v, %v", again, err)
	}
}

func TestAdminDashboardAggregatesAllUsersAndProvidersAreAdminOnly(t *testing.T) {
	service := testService(t)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	bob := identity.Actor{Username: "bob", Role: identity.RoleUser}
	provider, err := service.CreateProvider(admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Providers(alice); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ordinary user providers error = %v", err)
	}
	available, err := service.AvailableModels(alice)
	if err != nil || len(available) != 1 || available[0].ID != provider.ID {
		t.Fatalf("AvailableModels() = %#v, %v", available, err)
	}
	for _, actor := range []identity.Actor{alice, bob} {
		conversation, err := service.CreateConversation(actor, ConversationInput{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.SendMessage(context.Background(), actor, conversation.ID, MessageInput{Content: "hello"}); err != nil {
			t.Fatal(err)
		}
	}
	aliceDashboard, _ := service.Dashboard(alice)
	adminDashboard, _ := service.Dashboard(admin)
	if aliceDashboard.ConversationCount != 1 || aliceDashboard.TotalTokens != 11 {
		t.Fatalf("alice dashboard = %#v", aliceDashboard)
	}
	if adminDashboard.ConversationCount != 2 || adminDashboard.MessageCount != 4 || adminDashboard.TotalTokens != 22 || adminDashboard.ProviderCount != 1 {
		t.Fatalf("admin dashboard = %#v", adminDashboard)
	}
}

type captureModels struct{ request llm.CompletionRequest }

func (models *captureModels) Complete(_ context.Context, request llm.CompletionRequest) (*llm.CompletionResult, error) {
	models.request = request
	return &llm.CompletionResult{Content: "done", Model: request.Model}, nil
}
func (*captureModels) Test(context.Context, string, string, string) (time.Duration, error) {
	return 0, nil
}

func TestAttachmentIsConsumedAndDeletedAfterMessage(t *testing.T) {
	models := &captureModels{}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	_, _ = service.CreateProvider(admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{ReasoningEffort: "high"})
	attachment, err := service.CreateAttachment(alice, "notes.txt", "text/plain", []byte("release checklist"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(attachment.Path); err != nil {
		t.Fatal(err)
	}
	message, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{Content: "review", AttachmentIDs: []string{attachment.ID}})
	if err != nil || len(message.Attachments) != 0 {
		t.Fatalf("SendMessage() = %#v, %v", message, err)
	}
	if models.request.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", models.request.ReasoningEffort)
	}
	if _, err := os.Stat(attachment.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("used attachment was not deleted: %v", err)
	}
	var count int64
	if err := service.database.DB.Model(&model.Attachment{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("attachment rows = %d, %v", count, err)
	}
	loaded, _ := service.Conversation(alice, conversation.ID)
	if len(loaded.Messages) != 2 || len(loaded.Messages[0].Attachments) != 1 || loaded.Messages[0].Attachments[0] != "notes.txt" {
		t.Fatalf("stored attachment metadata = %#v", loaded.Messages)
	}
}

type blockingModels struct{ started chan struct{} }

func (models *blockingModels) Complete(ctx context.Context, _ llm.CompletionRequest) (*llm.CompletionResult, error) {
	close(models.started)
	<-ctx.Done()
	return nil, ctx.Err()
}
func (*blockingModels) Test(context.Context, string, string, string) (time.Duration, error) {
	return 0, nil
}

func TestStopGeneration(t *testing.T) {
	models := &blockingModels{started: make(chan struct{})}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	_, _ = service.CreateProvider(admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{})
	result := make(chan error, 1)
	go func() {
		_, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{Content: "think"})
		result <- err
	}()
	<-models.started
	stopped, err := service.StopGeneration(alice, conversation.ID)
	if err != nil || !stopped {
		t.Fatalf("StopGeneration() = %v, %v", stopped, err)
	}
	if err := <-result; !errors.Is(err, ErrCanceled) {
		t.Fatalf("SendMessage() error = %v", err)
	}
	loaded, _ := service.Conversation(alice, conversation.ID)
	if loaded.Messages[len(loaded.Messages)-1].Status != "stopped" {
		t.Fatalf("last message = %#v", loaded.Messages[len(loaded.Messages)-1])
	}
}

func TestConversationRejectsInvalidReasoningEffort(t *testing.T) {
	service := testService(t)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	_, _ = service.CreateProvider(admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, err := service.CreateConversation(alice, ConversationInput{})
	if err != nil {
		t.Fatal(err)
	}
	invalid := "unlimited"
	if _, err := service.UpdateConversation(alice, conversation.ID, ConversationPatch{ReasoningEffort: &invalid}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("UpdateConversation() error = %v", err)
	}
}

func TestSummarizeNewsRequiresEnabledProvider(t *testing.T) {
	service := testService(t)
	article := model.NewsArticle{ID: "news_two", SourceCode: "test", SourceName: "Test", Title: "News", URL: "https://example.com/news", PublishedAt: time.Now(), FetchedAt: time.Now()}
	if err := service.database.DB.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	_, err := service.SummarizeNews(context.Background(), identity.Actor{Username: "alice"}, []string{article.ID})
	if !errors.Is(err, ErrNoProvider) {
		t.Fatalf("expected ErrNoProvider, got %v", err)
	}
}

func containsError(err, target error) bool {
	return err != nil && (err == target || contains(err.Error(), target.Error()))
}
func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
