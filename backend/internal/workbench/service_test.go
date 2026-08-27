package workbench

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
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

func TestSharedPromptsAreReadableButOnlyOwnersCanManageThem(t *testing.T) {
	service := testService(t)
	admin := identity.Actor{ID: "internal:admin", Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{ID: "people:alice", Username: "alice", Role: identity.RoleUser}
	bob := identity.Actor{ID: "people:bob", Username: "bob", Role: identity.RoleUser}
	shared := true

	prompt, err := service.CreatePrompt(admin, PromptInput{Title: "共享总结", Content: "总结以下内容", Shared: &shared})
	if err != nil || !prompt.Shared || !prompt.CanEdit || !prompt.CanDelete {
		t.Fatalf("CreatePrompt(shared) = %#v, %v", prompt, err)
	}
	visible, err := service.Prompts(alice, "")
	if err != nil || len(visible) != 1 || visible[0].ID != prompt.ID || visible[0].CanEdit || visible[0].CanDelete {
		t.Fatalf("Prompts(alice) = %#v, %v", visible, err)
	}
	if _, err := service.UsePrompt(alice, prompt.ID); err != nil {
		t.Fatalf("UsePrompt(shared) = %v", err)
	}
	if _, err := service.UpdatePrompt(alice, prompt.ID, PromptInput{Title: "篡改", Content: "篡改"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdatePrompt(shared by non-owner) error = %v", err)
	}
	if err := service.DeletePrompt(bob, prompt.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeletePrompt(shared by non-owner) error = %v", err)
	}
	if _, err := service.CreatePrompt(alice, PromptInput{Title: "非法共享", Content: "内容", Shared: &shared}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ordinary user shared prompt error = %v", err)
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
	input := ProviderInput{Name: "Shared", BaseURL: "https://models.example/v1", DefaultModel: "model", APIKey: "secret"}
	provider, err := service.CreateProvider(admin, input)
	if err != nil || provider.Available || provider.LastTestedAt != nil || !provider.HasAPIKey {
		t.Fatalf("new provider health = %#v, %v", provider, err)
	}
	originalCiphertext := provider.APIKeyCiphertext
	input.APIKey = ""
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
	if providers[0].Available || providers[0].LastTestedAt == nil || providers[0].LastTestError != "upstream unavailable" || !providers[0].HasAPIKey || providers[0].APIKeyCiphertext != originalCiphertext {
		t.Fatalf("failed provider health = %#v", providers[0])
	}
	available, err := service.AvailableModels(identity.Actor{Username: "alice"})
	if err != nil || len(available) != 0 {
		t.Fatalf("failed provider should be unavailable: %#v, %v", available, err)
	}
}

func TestProvidersSortUsableConnectionsFirst(t *testing.T) {
	models := &providerTestModels{catalog: []string{"model"}}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	failed, err := service.CreateProvider(admin, ProviderInput{Name: "Failed", BaseURL: "https://failed.example/v1", DefaultModel: "model"})
	if err != nil {
		t.Fatal(err)
	}
	usable := createAvailableProvider(t, service, admin, ProviderInput{Name: "Usable", BaseURL: "https://usable.example/v1", DefaultModel: "model"})
	disabled := createAvailableProvider(t, service, admin, ProviderInput{Name: "Disabled", BaseURL: "https://disabled.example/v1", DefaultModel: "model"})
	enabled := false
	if _, err := service.UpdateProvider(admin, disabled.ID, ProviderInput{
		Name: disabled.Name, BaseURL: disabled.BaseURL, DefaultModel: disabled.DefaultModel, Protocol: disabled.Protocol, Enabled: &enabled,
	}); err != nil {
		t.Fatal(err)
	}

	providers, err := service.Providers(admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 3 || providers[0].ID != usable.ID || providers[1].ID != failed.ID || providers[2].ID != disabled.ID {
		t.Fatalf("provider order = %#v", providers)
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
	request       llm.CompletionRequest
	testRequest   llm.ConnectionTestRequest
	completeCalls int
}

func (models *captureModels) Complete(_ context.Context, request llm.CompletionRequest) (*llm.CompletionResult, error) {
	models.completeCalls++
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
	if len(loaded.Messages) != 2 || len(loaded.Messages[0].Attachments) != 1 || loaded.Messages[0].Attachments[0].Name != "notes.txt" || loaded.Messages[0].Attachments[0].PreviewURL != "" {
		t.Fatalf("stored attachment metadata = %#v", loaded.Messages)
	}
}

func TestImageAttachmentStoresThumbnailAndRetainsOriginalForConversation(t *testing.T) {
	models := &captureModels{}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{})

	picture := image.NewRGBA(image.Rect(0, 0, 900, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 900; x++ {
			picture.SetRGBA(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	var data bytes.Buffer
	if err := jpeg.Encode(&data, picture, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	attachment, err := service.CreateAttachment(alice, "photo.jpg", "application/octet-stream", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{Content: "描述图片", AttachmentIDs: []string{attachment.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(attachment.Path); err != nil {
		t.Fatalf("original image was not retained: %v", err)
	}
	loaded, err := service.Conversation(alice, conversation.ID)
	if err != nil || len(loaded.Messages[0].Attachments) != 1 {
		t.Fatalf("conversation = %#v, %v", loaded, err)
	}
	preview := loaded.Messages[0].Attachments[0]
	if preview.Name != "photo.jpg" || preview.ContentType != "image/jpeg" || !strings.HasPrefix(preview.PreviewURL, "data:image/jpeg;base64,") {
		t.Fatalf("preview metadata = %#v", preview)
	}
	thumbnailData, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(preview.PreviewURL, "data:image/jpeg;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	configuration, _, err := image.DecodeConfig(bytes.NewReader(thumbnailData))
	if err != nil || configuration.Width != 320 || configuration.Height != 213 {
		t.Fatalf("thumbnail = %#v, %v", configuration, err)
	}
	var retained model.Attachment
	if err := service.database.DB.First(&retained, "id = ?", attachment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if retained.ConversationID != conversation.ID || retained.MessageID != loaded.Messages[0].ID || time.Until(retained.ExpiresAt) < 29*24*time.Hour {
		t.Fatalf("retained attachment = %#v", retained)
	}
	if err := service.DeleteConversation(alice, conversation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(attachment.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conversation image was not deleted with conversation: %v", err)
	}
	var count int64
	if err := service.database.DB.Model(&model.Attachment{}).Where("id = ?", attachment.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("retained attachment rows = %d, %v", count, err)
	}
}

func TestIDPhotoReusesRetainedConversationImage(t *testing.T) {
	var receivedSource []byte
	imageTool := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := request.ParseMultipartForm(maxAttachmentBytes); err != nil {
			t.Fatal(err)
		}
		file, _, err := request.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		receivedSource, err = io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		output := image.NewNRGBA(image.Rect(0, 0, 500, 700))
		for y := 20; y < 700; y++ {
			for x := 80; x < 420; x++ {
				output.SetNRGBA(x, y, color.NRGBA{R: 210, G: 170, B: 140, A: 255})
			}
		}
		response.Header().Set("Content-Type", "image/png")
		if err := png.Encode(response, output); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(imageTool.Close)

	models := &captureModels{}
	service := testServiceWithModels(t, models)
	service.imageToolURL = imageTool.URL
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{})

	source := image.NewRGBA(image.Rect(0, 0, 600, 800))
	var sourceData bytes.Buffer
	if err := jpeg.Encode(&sourceData, source, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	attachment, err := service.CreateAttachment(alice, "portrait.jpg", "image/jpeg", sourceData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{
		Content: "先看看这张原图", AttachmentIDs: []string{attachment.ID},
	}); err != nil {
		t.Fatal(err)
	}
	if models.completeCalls != 1 {
		t.Fatalf("initial model calls = %d", models.completeCalls)
	}

	answer, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{Content: "把刚才那张做成二寸蓝底证件照"})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Model != "图片工具" || !strings.Contains(answer.Content, "二寸蓝底证件照") || !strings.Contains(answer.Content, "无需重新上传") {
		t.Fatalf("answer = %#v", answer)
	}
	if models.completeCalls != 1 || !bytes.Equal(receivedSource, sourceData.Bytes()) {
		t.Fatalf("model calls = %d, reused bytes = %d", models.completeCalls, len(receivedSource))
	}
}

func TestIncompleteIDPhotoRequestAsksForMissingParameters(t *testing.T) {
	models := &captureModels{}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{})

	picture := image.NewRGBA(image.Rect(0, 0, 300, 400))
	var data bytes.Buffer
	if err := jpeg.Encode(&data, picture, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	attachment, err := service.CreateAttachment(alice, "portrait.jpg", "image/jpeg", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	answer, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{
		Content: "帮我做成证件照", AttachmentIDs: []string{attachment.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Model != "图片工具" || !strings.Contains(answer.Content, "二寸") || !strings.Contains(answer.Content, "无需重新上传") || models.completeCalls != 0 {
		t.Fatalf("answer = %#v, model calls = %d", answer, models.completeCalls)
	}
}

func TestMissingRetainedImageAsksForOneReupload(t *testing.T) {
	models := &captureModels{}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{})
	attachmentMetadata, _ := json.Marshal([]model.MessageAttachment{{Name: "old-photo.jpg", ContentType: "image/jpeg", PreviewURL: "data:image/jpeg;base64,b2xk"}})
	if err := service.database.DB.Create(&model.Message{
		ID: "msg_old_image", ConversationID: conversation.ID, Role: "user", Content: "帮我处理这张图片", Status: "completed", AttachmentNames: string(attachmentMetadata),
	}).Error; err != nil {
		t.Fatal(err)
	}

	answer, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{Content: "刚才不是已经上传了吗"})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Model != "图片工具" || !strings.Contains(answer.Content, "重新上传一次") || !strings.Contains(answer.Content, "30 天") || models.completeCalls != 0 {
		t.Fatalf("answer = %#v, model calls = %d", answer, models.completeCalls)
	}
}

func TestImageAttachmentsCanBeMergedIntoDownloadablePDF(t *testing.T) {
	models := &captureModels{}
	service := testServiceWithModels(t, models)
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{})

	attachmentIDs := make([]string, 0, 2)
	for index, dimensions := range [][2]int{{640, 360}, {360, 640}} {
		var data bytes.Buffer
		picture := image.NewRGBA(image.Rect(0, 0, dimensions[0], dimensions[1]))
		fill := color.RGBA{R: uint8(40 + index*80), G: 120, B: 180, A: 255}
		for y := 0; y < dimensions[1]; y++ {
			for x := 0; x < dimensions[0]; x++ {
				picture.SetRGBA(x, y, fill)
			}
		}
		if err := jpeg.Encode(&data, picture, &jpeg.Options{Quality: 80}); err != nil {
			t.Fatal(err)
		}
		attachment, err := service.CreateAttachmentFromReader(alice, fmt.Sprintf("screen-%d.jpg", index+1), "image/jpeg", bytes.NewReader(data.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		attachmentIDs = append(attachmentIDs, attachment.ID)
	}

	answer, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{
		Content: "把这两张图片合并成一个 PDF 文件", AttachmentIDs: attachmentIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Model != "文件工具" || !strings.Contains(answer.Content, "下载合并后的 PDF") || models.request.Model != "" {
		t.Fatalf("answer = %#v, model request = %#v", answer, models.request)
	}
	match := regexp.MustCompile(`/attachments/([^/]+)/download\?token=([a-f0-9]+)`).FindStringSubmatch(answer.Content)
	if len(match) != 3 {
		t.Fatalf("download link not found in %q", answer.Content)
	}
	if _, _, err := service.OpenGeneratedAttachment(match[1], "incorrect-token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("incorrect token error = %v", err)
	}
	record, file, err := service.OpenGeneratedAttachment(match[1], match[2])
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	header := make([]byte, 5)
	if _, err := io.ReadFull(file, header); err != nil || string(header) != "%PDF-" {
		t.Fatalf("generated file header = %q, %v", header, err)
	}
	if record.ContentType != "application/pdf" || record.Size <= 100 || time.Until(record.ExpiresAt) < 6*24*time.Hour {
		t.Fatalf("generated attachment = %#v", record)
	}
}

func TestIDPhotoUsesConfirmedConversationParameters(t *testing.T) {
	var receivedSource []byte
	imageTool := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/remove" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if err := request.ParseMultipartForm(maxAttachmentBytes); err != nil {
			t.Fatal(err)
		}
		if request.FormValue("model") != "u2net_human_seg" {
			t.Fatalf("unexpected model: %q", request.FormValue("model"))
		}
		file, _, err := request.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		receivedSource, err = io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		output := image.NewNRGBA(image.Rect(0, 0, 800, 1000))
		for y := 40; y < 1000; y++ {
			for x := 120; x < 680; x++ {
				output.SetNRGBA(x, y, color.NRGBA{R: 220, G: 180, B: 150, A: 255})
			}
		}
		var data bytes.Buffer
		if err := png.Encode(&data, output); err != nil {
			t.Fatal(err)
		}
		response.Header().Set("Content-Type", "image/png")
		_, _ = response.Write(data.Bytes())
	}))
	t.Cleanup(imageTool.Close)

	models := &captureModels{}
	service := testServiceWithModels(t, models)
	service.imageToolURL = imageTool.URL
	admin := identity.Actor{Username: "admin", Source: "internal", Role: identity.RoleAdmin}
	alice := identity.Actor{Username: "alice", Role: identity.RoleUser}
	createAvailableProvider(t, service, admin, ProviderInput{Name: "Shared", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	conversation, _ := service.CreateConversation(alice, ConversationInput{})

	if _, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{Content: "帮我修改证件照"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{Content: "给我一个2寸的，蓝底的"}); err != nil {
		t.Fatal(err)
	}

	source := image.NewRGBA(image.Rect(0, 0, 800, 1000))
	var sourceData bytes.Buffer
	if err := jpeg.Encode(&sourceData, source, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	attachment, err := service.CreateAttachment(alice, "portrait.jpg", "image/jpeg", sourceData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	answer, err := service.SendMessage(context.Background(), alice, conversation.ID, MessageInput{AttachmentIDs: []string{attachment.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Model != "图片工具" || !strings.Contains(answer.Content, "二寸蓝底证件照") || !strings.Contains(answer.Content, "413 × 579 px") {
		t.Fatalf("answer = %#v", answer)
	}
	if models.completeCalls != 2 || len(receivedSource) == 0 {
		t.Fatalf("model calls = %d, image bytes = %d", models.completeCalls, len(receivedSource))
	}
	match := regexp.MustCompile(`/attachments/([^/]+)/download\?token=([a-f0-9]+)`).FindStringSubmatch(answer.Content)
	if len(match) != 3 {
		t.Fatalf("download link not found in %q", answer.Content)
	}
	record, file, err := service.OpenGeneratedAttachment(match[1], match[2])
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	generatedData, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(generatedData))
	if err != nil || format != "jpeg" || configuration.Width != 413 || configuration.Height != 579 {
		t.Fatalf("generated image = %#v %q, %v", configuration, format, err)
	}
	if !bytes.Contains(generatedData[:min(len(generatedData), 32)], []byte{'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x01, 0x01, 0x2c, 0x01, 0x2c}) {
		t.Fatal("generated image does not contain 300 DPI JFIF density")
	}
	if record.Name != "二寸蓝底证件照.jpg" || record.ContentType != "image/jpeg" {
		t.Fatalf("generated attachment = %#v", record)
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
