package llm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestResponsesWebSearchIntegration(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("AI_WORKBENCH_TEST_RESPONSES_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("AI_WORKBENCH_TEST_RESPONSES_API_KEY"))
	model := strings.TrimSpace(os.Getenv("AI_WORKBENCH_TEST_RESPONSES_MODEL"))
	if baseURL == "" || apiKey == "" || model == "" {
		t.Skip("Responses integration environment is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := New().Complete(ctx, CompletionRequest{
		BaseURL: baseURL, APIKey: apiKey, Model: model, Protocol: "responses",
		WebSearch: true, RequireTool: true, ReasoningEffort: "fast",
		Messages: []Message{{Role: "user", Content: "Search the web for the current UTC date and reply with the date only."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.UsedWebSearch || strings.TrimSpace(result.Content) == "" || result.PromptTokens == 0 || result.CompletionTokens == 0 {
		t.Fatalf("unexpected Responses result: %#v", result)
	}
}
