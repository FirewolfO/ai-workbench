package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteOpenAICompatibleRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected request %s, auth %q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input["reasoning_effort"] != "high" {
			t.Fatalf("reasoning_effort = %#v", input["reasoning_effort"])
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"model": "test-model", "choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": "Hello"}}},
			"usage": map[string]int{"prompt_tokens": 4, "completion_tokens": 2},
		})
	}))
	defer server.Close()
	result, err := New().Complete(context.Background(), CompletionRequest{BaseURL: server.URL + "/v1", APIKey: "secret", Model: "test-model", ReasoningEffort: "high", Messages: []Message{{Role: "user", Content: "Hi"}}})
	if err != nil || result.Content != "Hello" || result.PromptTokens != 4 || result.CompletionTokens != 2 {
		t.Fatalf("Complete() = %#v, %v", result, err)
	}
}

func TestCompleteResponsesWithWebSearchAndCitations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" || request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected request %s, auth %q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input["store"] != false || input["tool_choice"] != "auto" {
			t.Fatalf("responses options = %#v", input)
		}
		reasoning, _ := input["reasoning"].(map[string]any)
		if reasoning["effort"] != "xhigh" {
			t.Fatalf("reasoning = %#v", reasoning)
		}
		tools, _ := input["tools"].([]any)
		if len(tools) != 1 || tools[0].(map[string]any)["type"] != "web_search" {
			t.Fatalf("tools = %#v", tools)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"model":"gpt-5.5","status":"completed",
			"output":[
				{"type":"web_search_call","status":"completed","action":{"sources":[{"type":"api","name":"oai-weather"},{"url":"https://example.com/weather","title":"天气数据"}]}},
				{"type":"message","status":"completed","content":[{"type":"output_text","text":"北京 25°C，多云。","annotations":[{"type":"url_citation","url":"https://example.com/weather","title":"天气数据"},{"type":"url_citation","url":"https://example.org/report","title":"气象报告"}]}]}
			],
			"usage":{"input_tokens":42,"output_tokens":12}
		}`))
	}))
	defer server.Close()

	result, err := New().Complete(context.Background(), CompletionRequest{
		BaseURL: server.URL, APIKey: "secret", Model: "gpt-5.5", Protocol: "responses",
		WebSearch: true, ReasoningEffort: "xhigh", Messages: []Message{{Role: "user", Content: "北京天气"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.UsedWebSearch || result.PromptTokens != 42 || result.CompletionTokens != 12 || !strings.Contains(result.Content, "北京 25°C") {
		t.Fatalf("Complete() = %#v", result)
	}
	if strings.Count(result.Content, "https://example.com/weather") != 1 || !strings.Contains(result.Content, "https://example.org/report") {
		t.Fatalf("citations = %q", result.Content)
	}
	if !strings.Contains(result.Content, "- oai-weather") {
		t.Fatalf("API source = %q", result.Content)
	}
}

func TestResponsesConvertsImageInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input struct {
			Input []struct {
				Content []map[string]any `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if len(input.Input) != 1 || len(input.Input[0].Content) != 2 || input.Input[0].Content[0]["type"] != "input_text" || input.Input[0].Content[1]["type"] != "input_image" {
			t.Fatalf("input = %#v", input.Input)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"model":"model","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`))
	}))
	defer server.Close()

	_, err := New().Complete(context.Background(), CompletionRequest{
		BaseURL: server.URL, Model: "model", Protocol: "responses",
		Messages: []Message{{Role: "user", Content: []ContentPart{{Type: "text", Text: "查看图片"}, {Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,AA=="}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestResponsesExecutesBackendFunctionTool(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		var input struct {
			Input []map[string]any `json:"input"`
			Tools []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			if len(input.Tools) != 1 || input.Tools[0]["name"] != "get_weather" {
				t.Fatalf("tools = %#v", input.Tools)
			}
			_, _ = writer.Write([]byte(`{"model":"model","output":[{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{\"location\":\"武汉\"}"}],"usage":{"input_tokens":4,"output_tokens":2}}`))
			return
		}
		last := input.Input[len(input.Input)-1]
		if last["type"] != "function_call_output" || last["call_id"] != "call_1" || !strings.Contains(last["output"].(string), "32") {
			t.Fatalf("tool output = %#v", last)
		}
		_, _ = writer.Write([]byte(`{"model":"model","output":[{"type":"message","content":[{"type":"output_text","text":"武汉现在 32°C"}]}],"usage":{"input_tokens":7,"output_tokens":3}}`))
	}))
	defer server.Close()

	called := false
	result, err := New().Complete(context.Background(), CompletionRequest{
		BaseURL: server.URL, Model: "model", Protocol: "responses", Messages: []Message{{Role: "user", Content: "武汉天气"}},
		Tools: []ToolDefinition{{Name: "get_weather", Description: "weather", Parameters: map[string]any{"type": "object"}}},
		ToolHandler: func(_ context.Context, name string, arguments json.RawMessage) (string, error) {
			called = name == "get_weather" && strings.Contains(string(arguments), "武汉")
			return `{"temperature":32}`, nil
		},
	})
	if err != nil || !called || requests != 2 || result.PromptTokens != 11 || result.CompletionTokens != 5 || result.Content != "武汉现在 32°C" {
		t.Fatalf("Complete() = %#v, called=%v requests=%d err=%v", result, called, requests, err)
	}
}

func TestChatCompletionsExecutesBackendFunctionTool(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		var input struct {
			Messages []map[string]any `json:"messages"`
			Tools    []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			function := input.Tools[0]["function"].(map[string]any)
			if function["name"] != "calculate" {
				t.Fatalf("tools = %#v", input.Tools)
			}
			_, _ = writer.Write([]byte(`{"model":"model","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_2","type":"function","function":{"name":"calculate","arguments":"{\"expression\":\"6*7\"}"}}]}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
			return
		}
		last := input.Messages[len(input.Messages)-1]
		if last["role"] != "tool" || last["tool_call_id"] != "call_2" || !strings.Contains(last["content"].(string), "42") {
			t.Fatalf("tool message = %#v", last)
		}
		_, _ = writer.Write([]byte(`{"model":"model","choices":[{"message":{"role":"assistant","content":"答案是 42"}}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`))
	}))
	defer server.Close()

	result, err := New().Complete(context.Background(), CompletionRequest{
		BaseURL: server.URL, Model: "model", Messages: []Message{{Role: "user", Content: "6*7"}},
		Tools:       []ToolDefinition{{Name: "calculate", Description: "calculator", Parameters: map[string]any{"type": "object"}}},
		ToolHandler: func(_ context.Context, _ string, _ json.RawMessage) (string, error) { return `{"result":42}`, nil },
	})
	if err != nil || requests != 2 || result.Content != "答案是 42" || result.PromptTokens != 8 || result.CompletionTokens != 5 {
		t.Fatalf("Complete() = %#v requests=%d err=%v", result, requests, err)
	}
}

func TestChatCompletionsFallsBackWhenGatewayRejectsTools(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(`{"error":{"message":"tools are not supported by this model"}}`))
			return
		}
		if _, exists := input["tools"]; exists {
			t.Fatalf("fallback still contains tools: %#v", input)
		}
		_, _ = writer.Write([]byte(`{"model":"legacy-model","choices":[{"message":{"role":"assistant","content":"普通回答"}}]}`))
	}))
	defer server.Close()

	result, err := New().Complete(context.Background(), CompletionRequest{
		BaseURL: server.URL, Model: "legacy-model", Messages: []Message{{Role: "user", Content: "你好"}},
		Tools:       []ToolDefinition{{Name: "calculate", Parameters: map[string]any{"type": "object"}}},
		ToolHandler: func(_ context.Context, _ string, _ json.RawMessage) (string, error) { return "", nil },
	})
	if err != nil || requests != 2 || result.Content != "普通回答" {
		t.Fatalf("Complete() = %#v requests=%d err=%v", result, requests, err)
	}
}

func TestEndpointValidation(t *testing.T) {
	for _, value := range []string{"", "ftp://example.com/v1", "https://user@example.com/v1"} {
		if _, err := endpoint(value, "models"); err == nil {
			t.Fatalf("endpoint(%q) should fail", value)
		}
	}
}

func TestConnectionTestValidatesJSONAndCompletion(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": []any{
				map[string]string{"id": "test-model-pro"}, map[string]string{"id": "test-model"},
				map[string]string{"id": "test-model"}, map[string]string{"id": ""},
			}})
		case "/v1/chat/completions":
			_ = json.NewEncoder(writer).Encode(map[string]any{"model": "test-model", "choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": "OK"}}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	result, err := New().Test(context.Background(), ConnectionTestRequest{BaseURL: server.URL + "/v1", APIKey: "secret", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Models) != 2 || result.Models[0] != "test-model" || result.Models[1] != "test-model-pro" {
		t.Fatalf("models = %#v", result.Models)
	}
	if requests != 2 {
		t.Fatalf("expected models and completion requests, got %d", requests)
	}
}

func TestConnectionTestRejectsHTMLSuccessPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte("<html>admin console</html>"))
	}))
	defer server.Close()
	if _, err := New().Test(context.Background(), ConnectionTestRequest{BaseURL: server.URL + "/keys", APIKey: "secret", Model: "test-model"}); err == nil {
		t.Fatal("expected HTML response to be rejected")
	}
}

func TestModelsIncludesUpstreamErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"code":"INSUFFICIENT_BALANCE","message":"Insufficient account balance"}`))
	}))
	defer server.Close()

	_, err := New().Models(context.Background(), server.URL, "secret")
	if err == nil || !strings.Contains(err.Error(), "Insufficient account balance") {
		t.Fatalf("Models() error = %v", err)
	}
}
