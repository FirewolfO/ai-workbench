package workbench

import (
	"context"
	"errors"
	"os"
	"strings"
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
func (fakeModels) Test(context.Context, llm.ConnectionTestRequest) (*llm.ConnectionTest, error) {
	return &llm.ConnectionTest{Latency: 10 * time.Millisecond, Models: []string{"model"}}, nil
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

func createAvailableProvider(t *testing.T, service *Service, admin identity.Actor, input ProviderInput) *model.Provider {
	t.Helper()
	provider, err := service.CreateProvider(admin, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.TestProvider(context.Background(), admin, provider.ID); err != nil {
		t.Fatal(err)
	}
	return provider
}

type summaryModels struct{}

func (summaryModels) Complete(_ context.Context, _ llm.CompletionRequest) (*llm.CompletionResult, error) {
	return &llm.CompletionResult{Content: "```json\n[{\"id\":\"news_one\",\"summary\":\"这是一条经过压缩的中文人工智能热点概要，说明事件内容及其主要价值。\"}]\n```", Model: "model"}, nil
}
func (summaryModels) Test(context.Context, llm.ConnectionTestRequest) (*llm.ConnectionTest, error) {
	return &llm.ConnectionTest{Models: []string{"model"}}, nil
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
	if _, err := service.TestProvider(context.Background(), admin, provider.ID); err != nil {
		t.Fatal(err)
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
	provider := createAvailableProvider(t, service, admin, ProviderInput{Name: "Local", BaseURL: "http://localhost/v1", DefaultModel: "model"})
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
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Local", BaseURL: "http://localhost/v1", DefaultModel: "model"})
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
	if unavailable, err := service.AvailableModels(alice); err != nil || len(unavailable) != 0 {
		t.Fatalf("untested provider should be unavailable: %#v, %v", unavailable, err)
	}
	if _, err := service.TestProvider(context.Background(), admin, provider.ID); err != nil {
		t.Fatal(err)
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

type providerTestModels struct {
	testErr error
	catalog []string
}

func (*providerTestModels) Complete(_ context.Context, request llm.CompletionRequest) (*llm.CompletionResult, error) {
	return &llm.CompletionResult{Content: "OK", Model: request.Model}, nil
}
func (models *providerTestModels) Test(context.Context, llm.ConnectionTestRequest) (*llm.ConnectionTest, error) {
	return &llm.ConnectionTest{Latency: 12 * time.Millisecond, Models: append([]string(nil), models.catalog...)}, models.testErr
}
func (models *providerTestModels) Models(context.Context, string, string) ([]string, error) {
	return append([]string(nil), models.catalog...), models.testErr
}

func TestProviderHealthLifecycle(t *testing.T) {
	models := &providerTestModels{catalog: []string{"model", "model-pro"}}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	input := ProviderInput{Name: "Shared", BaseURL: "https://models.example/v1", DefaultModel: "model"}
	provider, err := service.CreateProvider(admin, input)
	if err != nil || provider.Available || provider.LastTestedAt != nil {
		t.Fatalf("new provider health = %#v, %v", provider, err)
	}
	result, err := service.TestProvider(context.Background(), admin, provider.ID)
	if err != nil || result.ModelCount != 2 {
		t.Fatalf("TestProvider() = %#v, %v", result, err)
	}
	providers, _ := service.Providers(admin)
	if !providers[0].Available || providers[0].LastTestedAt == nil || providers[0].LastTestLatencyMs != 12 || providers[0].LastTestError != "" || len(providers[0].Models) != 2 {
		t.Fatalf("successful provider health = %#v", providers[0])
	}
	input.Name = "Renamed"
	updated, err := service.UpdateProvider(admin, provider.ID, input)
	if err != nil || !updated.Available || updated.LastTestedAt == nil {
		t.Fatalf("non-connection update should retain health = %#v, %v", updated, err)
	}
	input.DefaultModel = "other-model"
	updated, err = service.UpdateProvider(admin, provider.ID, input)
	if err != nil || updated.Available || updated.LastTestedAt != nil || len(updated.Models) != 1 || updated.Models[0] != "other-model" || updated.ModelsUpdatedAt != nil {
		t.Fatalf("connection update should reset health = %#v, %v", updated, err)
	}
	models.testErr = errors.New("upstream unavailable")
	if _, err := service.TestProvider(context.Background(), admin, provider.ID); !errors.Is(err, ErrProvider) {
		t.Fatalf("failed provider test error = %v", err)
	}
	providers, _ = service.Providers(admin)
	if providers[0].Available || providers[0].LastTestedAt == nil || providers[0].LastTestError != "upstream unavailable" {
		t.Fatalf("failed provider health = %#v", providers[0])
	}
	available, err := service.AvailableModels(identity.Actor{Username: "alice"})
	if err != nil || len(available) != 0 {
		t.Fatalf("failed provider should be unavailable: %#v, %v", available, err)
	}
}

func TestRefreshAvailableModelsUpdatesStaleCatalog(t *testing.T) {
	models := &providerTestModels{catalog: []string{"model", "model-pro"}}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	provider := createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "https://models.example/v1", DefaultModel: "model"})

	stale := time.Now().Add(-modelCatalogRefreshInterval - time.Minute)
	if err := service.database.DB.Model(&model.Provider{}).Where("id = ?", provider.ID).Update("models_updated_at", stale).Error; err != nil {
		t.Fatal(err)
	}
	models.catalog = []string{"model-next", "model"}
	if err := service.RefreshAvailableModels(context.Background(), identity.Actor{Username: "alice"}); err != nil {
		t.Fatal(err)
	}
	available, err := service.AvailableModels(identity.Actor{Username: "alice"})
	if err != nil || len(available) != 1 || len(available[0].Models) != 2 || available[0].Models[0] != "model" || available[0].Models[1] != "model-next" {
		t.Fatalf("refreshed models = %#v, %v", available, err)
	}

	models.testErr = errors.New("temporary catalog failure")
	if err := service.database.DB.Model(&model.Provider{}).Where("id = ?", provider.ID).Update("models_updated_at", stale).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.RefreshAvailableModels(context.Background(), identity.Actor{Username: "alice"}); err != nil {
		t.Fatal(err)
	}
	available, _ = service.AvailableModels(identity.Actor{Username: "alice"})
	if len(available[0].Models) != 2 || available[0].Models[1] != "model-next" {
		t.Fatalf("transient failure should preserve catalog: %#v", available)
	}
}

type captureModels struct {
	request     llm.CompletionRequest
	testRequest llm.ConnectionTestRequest
}

func (models *captureModels) Complete(_ context.Context, request llm.CompletionRequest) (*llm.CompletionResult, error) {
	models.request = request
	return &llm.CompletionResult{Content: "done", Model: request.Model}, nil
}
func (models *captureModels) Test(_ context.Context, request llm.ConnectionTestRequest) (*llm.ConnectionTest, error) {
	models.testRequest = request
	return &llm.ConnectionTest{Models: []string{"model"}}, nil
}

func TestResponsesProviderConfigurationFlowsToCompletion(t *testing.T) {
	models := &captureModels{}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	webSearch := true
	provider := createAvailableProvider(t, service, admin, ProviderInput{
		Name: "Sub2API", BaseURL: "http://models.example", DefaultModel: "model", Protocol: "responses", WebSearchEnabled: &webSearch,
	})
	if provider.Protocol != "responses" || !provider.WebSearchEnabled || models.testRequest.Protocol != "responses" || !models.testRequest.WebSearch {
		t.Fatalf("provider configuration = %#v, test = %#v", provider, models.testRequest)
	}
	conversation, err := service.CreateConversation(alice, ConversationInput{ProviderID: provider.ID, Model: "model", ReasoningEffort: "xhigh"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{Content: "今天北京天气"}); err != nil {
		t.Fatal(err)
	}
	if models.request.Protocol != "responses" || !models.request.WebSearch || models.request.ReasoningEffort != "xhigh" {
		t.Fatalf("completion request = %#v", models.request)
	}

	chatProtocol := ProviderInput{Name: provider.Name, BaseURL: provider.BaseURL, DefaultModel: provider.DefaultModel, Protocol: "chat_completions", WebSearchEnabled: &webSearch}
	updated, err := service.UpdateProvider(admin, provider.ID, chatProtocol)
	if err != nil {
		t.Fatal(err)
	}
	if updated.WebSearchEnabled || updated.Available {
		t.Fatalf("chat provider should disable search and require retest: %#v", updated)
	}
}

func TestAttachmentIsConsumedAndDeletedAfterMessage(t *testing.T) {
	models := &captureModels{}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
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
func (*blockingModels) Test(context.Context, llm.ConnectionTestRequest) (*llm.ConnectionTest, error) {
	return &llm.ConnectionTest{Models: []string{"model"}}, nil
}

func TestStopGeneration(t *testing.T) {
	models := &blockingModels{started: make(chan struct{})}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
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

type queuedModels struct {
	started chan struct{}
	release chan struct{}
}

func (models *queuedModels) Complete(ctx context.Context, request llm.CompletionRequest) (*llm.CompletionResult, error) {
	close(models.started)
	select {
	case <-models.release:
		return &llm.CompletionResult{Content: "后台回答", Model: request.Model, PromptTokens: 13, CompletionTokens: 5, Latency: time.Second}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (*queuedModels) Test(context.Context, llm.ConnectionTestRequest) (*llm.ConnectionTest, error) {
	return &llm.ConnectionTest{Models: []string{"model"}}, nil
}

func TestQueueMessageCompletesInBackground(t *testing.T) {
	models := &queuedModels{started: make(chan struct{}), release: make(chan struct{})}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{})

	queued, err := service.QueueMessage(alice, conversation.ID, MessageInput{Content: "复杂联网问题"})
	if err != nil || queued.Status != "generating" {
		t.Fatalf("QueueMessage() = %#v, %v", queued, err)
	}
	<-models.started
	loaded, _ := service.Conversation(alice, conversation.ID)
	if len(loaded.Messages) != 2 || loaded.Messages[1].ID != queued.ID || loaded.Messages[1].Status != "generating" {
		t.Fatalf("queued conversation = %#v", loaded.Messages)
	}

	close(models.release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		loaded, _ = service.Conversation(alice, conversation.ID)
		if loaded.Messages[1].Status == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	answer := loaded.Messages[1]
	if answer.Status != "completed" || answer.Content != "后台回答" || answer.PromptTokens != 13 || answer.CompletionTokens != 5 {
		t.Fatalf("completed message = %#v", answer)
	}
}

func TestStopQueuedGeneration(t *testing.T) {
	models := &blockingModels{started: make(chan struct{})}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{})
	queued, err := service.QueueMessage(alice, conversation.ID, MessageInput{Content: "停止这个任务"})
	if err != nil {
		t.Fatal(err)
	}
	<-models.started
	stopped, err := service.StopGeneration(alice, conversation.ID)
	if err != nil || !stopped {
		t.Fatalf("StopGeneration() = %v, %v", stopped, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		loaded, _ := service.Conversation(alice, conversation.ID)
		for _, message := range loaded.Messages {
			if message.ID == queued.ID && message.Status == "stopped" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("queued generation was not marked stopped")
}

func TestNewMarksInterruptedGenerationAsFailed(t *testing.T) {
	service := testService(t)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{})
	message := model.Message{ID: "msg_generating", ConversationID: conversation.ID, Role: "assistant", Content: "正在生成", Model: "model", Status: "generating"}
	if err := service.database.DB.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	_ = New(service.database, service.vault, fakeModels{}, t.TempDir())
	var recovered model.Message
	if err := service.database.DB.First(&recovered, "id = ?", message.ID).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "failed" || !strings.Contains(recovered.Content, "服务重启") {
		t.Fatalf("recovered message = %#v", recovered)
	}
}

func TestConversationRejectsInvalidReasoningEffort(t *testing.T) {
	service := testService(t)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
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
