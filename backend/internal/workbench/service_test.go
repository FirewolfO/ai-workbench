package workbench

import (
	"context"
	"testing"
	"time"

	"ai-workbench/internal/identity"
	"ai-workbench/internal/llm"
	"ai-workbench/internal/security"
	"ai-workbench/internal/store"
)

type fakeModels struct{}

func (fakeModels) Complete(_ context.Context, request llm.CompletionRequest) (*llm.CompletionResult, error) {
	return &llm.CompletionResult{Content: "回答", Model: request.Model, PromptTokens: 8, CompletionTokens: 3, Latency: 20 * time.Millisecond}, nil
}
func (fakeModels) Test(context.Context, string, string) (time.Duration, error) {
	return 10 * time.Millisecond, nil
}

func testService(t *testing.T) *Service {
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
	return New(database, vault, fakeModels{})
}

func TestPersonalDataIsolationAndChat(t *testing.T) {
	service := testService(t)
	alice := identity.Actor{Username: "alice"}
	bob := identity.Actor{Username: "bob"}
	provider, err := service.CreateProvider(alice, ProviderInput{Name: "Local", BaseURL: "http://localhost:11434/v1", DefaultModel: "model", APIKey: "secret"})
	if err != nil || !provider.HasAPIKey || provider.APIKeyCiphertext == "secret" {
		t.Fatalf("CreateProvider() = %#v, %v", provider, err)
	}
	conversation, err := service.CreateConversation(alice, ConversationInput{ProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := service.SendMessage(context.Background(), alice, conversation.ID, "你好")
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
	actor := identity.Actor{Username: "alice"}
	provider, _ := service.CreateProvider(actor, ProviderInput{Name: "Local", BaseURL: "http://localhost/v1", DefaultModel: "model"})
	_, _ = service.CreateConversation(actor, ConversationInput{ProviderID: provider.ID})
	if err := service.DeleteProvider(actor, provider.ID); !containsError(err, ErrConflict) {
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
