package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompleteOpenAICompatibleRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected request %s, auth %q", request.URL.Path, request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"model": "test-model", "choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": "Hello"}}},
			"usage": map[string]int{"prompt_tokens": 4, "completion_tokens": 2},
		})
	}))
	defer server.Close()
	result, err := New().Complete(context.Background(), CompletionRequest{BaseURL: server.URL + "/v1", APIKey: "secret", Model: "test-model", Messages: []Message{{Role: "user", Content: "Hi"}}})
	if err != nil || result.Content != "Hello" || result.PromptTokens != 4 || result.CompletionTokens != 2 {
		t.Fatalf("Complete() = %#v, %v", result, err)
	}
}

func TestEndpointValidation(t *testing.T) {
	for _, value := range []string{"", "ftp://example.com/v1", "https://user@example.com/v1"} {
		if _, err := endpoint(value, "models"); err == nil {
			t.Fatalf("endpoint(%q) should fail", value)
		}
	}
}
