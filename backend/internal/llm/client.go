package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type CompletionRequest struct {
	BaseURL         string
	APIKey          string
	Model           string
	Protocol        string
	WebSearch       bool
	RequireTool     bool
	Messages        []Message
	Temperature     float64
	ReasoningEffort string
}

type CompletionResult struct {
	Content          string
	Model            string
	PromptTokens     int
	CompletionTokens int
	Latency          time.Duration
	UsedWebSearch    bool
}

type ConnectionTestRequest struct {
	BaseURL   string
	APIKey    string
	Model     string
	Protocol  string
	WebSearch bool
}

type ConnectionTest struct {
	Latency time.Duration
	Models  []string
}

type Client interface {
	Complete(context.Context, CompletionRequest) (*CompletionResult, error)
	Test(context.Context, ConnectionTestRequest) (*ConnectionTest, error)
}

type ModelCatalogClient interface {
	Models(context.Context, string, string) ([]string, error)
}

type HTTPClient struct{ client *http.Client }

func New() *HTTPClient { return &HTTPClient{client: &http.Client{Timeout: 90 * time.Second}} }

func (c *HTTPClient) Complete(ctx context.Context, input CompletionRequest) (*CompletionResult, error) {
	if input.Protocol == "responses" {
		return c.completeResponses(ctx, input)
	}
	return c.completeChat(ctx, input)
}

func (c *HTTPClient) completeChat(ctx context.Context, input CompletionRequest) (*CompletionResult, error) {
	target, err := endpoint(input.BaseURL, "chat/completions")
	if err != nil {
		return nil, err
	}
	requestPayload := map[string]any{"model": input.Model, "messages": input.Messages}
	if effort := providerReasoningEffort(input.ReasoningEffort); effort != "" {
		requestPayload["reasoning_effort"] = effort
	} else {
		requestPayload["temperature"] = input.Temperature
	}
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(input.APIKey) != "" {
		request.Header.Set("Authorization", "Bearer "+input.APIKey)
	}
	started := time.Now()
	response, err := c.client.Do(request)
	latency := time.Since(started)
	if err != nil {
		return nil, fmt.Errorf("模型服务连接失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("模型服务返回 %d: %s", response.StatusCode, providerMessage(message))
	}
	if !isJSON(response.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("模型服务返回 %s 而不是 JSON，请检查 Base URL 是否指向 API 根路径（通常以 /v1 结尾）", contentType(response.Header.Get("Content-Type")))
	}
	var responsePayload struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(response.Body).Decode(&responsePayload); err != nil || len(responsePayload.Choices) == 0 || strings.TrimSpace(responsePayload.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("模型服务返回了无效响应")
	}
	return &CompletionResult{
		Content: responsePayload.Choices[0].Message.Content, Model: responsePayload.Model,
		PromptTokens: responsePayload.Usage.PromptTokens, CompletionTokens: responsePayload.Usage.CompletionTokens, Latency: latency,
	}, nil
}

type responsesOutputItem struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Action struct {
		Sources []struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"sources"`
	} `json:"action"`
	Content []struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Annotations []struct {
			Type  string `json:"type"`
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"annotations"`
	} `json:"content"`
}

func (c *HTTPClient) completeResponses(ctx context.Context, input CompletionRequest) (*CompletionResult, error) {
	target, err := endpoint(input.BaseURL, "responses")
	if err != nil {
		return nil, err
	}
	requestPayload := map[string]any{
		"model": input.Model, "input": responseMessages(input.Messages), "store": false,
	}
	if effort := providerReasoningEffort(input.ReasoningEffort); effort != "" {
		requestPayload["reasoning"] = map[string]string{"effort": effort}
	}
	if input.WebSearch {
		requestPayload["tools"] = []map[string]string{{"type": "web_search"}}
		requestPayload["tool_choice"] = "auto"
		requestPayload["include"] = []string{"web_search_call.action.sources"}
		if input.RequireTool {
			requestPayload["tool_choice"] = "required"
		}
	}
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(input.APIKey) != "" {
		request.Header.Set("Authorization", "Bearer "+input.APIKey)
	}
	started := time.Now()
	response, err := c.client.Do(request)
	latency := time.Since(started)
	if err != nil {
		return nil, fmt.Errorf("模型服务连接失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("模型服务返回 %d: %s", response.StatusCode, providerMessage(message))
	}
	if !isJSON(response.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("模型服务返回 %s 而不是 JSON，请检查 Base URL 是否指向 Responses API 根路径", contentType(response.Header.Get("Content-Type")))
	}
	var responsePayload struct {
		Model  string                `json:"model"`
		Status string                `json:"status"`
		Output []responsesOutputItem `json:"output"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(response.Body).Decode(&responsePayload); err != nil {
		return nil, fmt.Errorf("模型服务返回了无效响应")
	}
	if responsePayload.Error != nil && strings.TrimSpace(responsePayload.Error.Message) != "" {
		return nil, fmt.Errorf("模型服务返回错误: %s", responsePayload.Error.Message)
	}
	content, sources, usedWebSearch := responseOutput(responsePayload.Output)
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("模型服务返回了无效响应")
	}
	return &CompletionResult{
		Content: appendSources(content, sources), Model: responsePayload.Model,
		PromptTokens: responsePayload.Usage.InputTokens, CompletionTokens: responsePayload.Usage.OutputTokens,
		Latency: latency, UsedWebSearch: usedWebSearch,
	}, nil
}

func responseMessages(messages []Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		content := message.Content
		if parts, ok := content.([]ContentPart); ok {
			converted := make([]map[string]any, 0, len(parts))
			for _, part := range parts {
				switch part.Type {
				case "text":
					converted = append(converted, map[string]any{"type": "input_text", "text": part.Text})
				case "image_url":
					if part.ImageURL != nil {
						converted = append(converted, map[string]any{"type": "input_image", "image_url": part.ImageURL.URL})
					}
				}
			}
			content = converted
		}
		result = append(result, map[string]any{"role": message.Role, "content": content})
	}
	return result
}

type responseSource struct {
	URL   string
	Title string
}

func responseOutput(output []responsesOutputItem) (string, []responseSource, bool) {
	texts := make([]string, 0)
	sources := make([]responseSource, 0)
	usedWebSearch := false
	for _, item := range output {
		if item.Type == "web_search_call" {
			usedWebSearch = true
			for _, source := range item.Action.Sources {
				sources = append(sources, responseSource{URL: source.URL, Title: source.Title})
			}
		}
		if item.Type != "message" {
			continue
		}
		for _, part := range item.Content {
			if part.Type == "output_text" && strings.TrimSpace(part.Text) != "" {
				texts = append(texts, part.Text)
			}
			for _, annotation := range part.Annotations {
				if annotation.Type == "url_citation" {
					sources = append(sources, responseSource{URL: annotation.URL, Title: annotation.Title})
				}
			}
		}
	}
	return strings.Join(texts, "\n\n"), sources, usedWebSearch
}

func appendSources(content string, sources []responseSource) string {
	seen := make(map[string]bool, len(sources))
	links := make([]string, 0, len(sources))
	for _, source := range sources {
		value := strings.TrimSpace(source.URL)
		if value == "" || seen[value] {
			continue
		}
		target, err := url.Parse(value)
		if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
			continue
		}
		seen[value] = true
		title := strings.TrimSpace(source.Title)
		if title == "" {
			title = target.Host
		}
		title = strings.NewReplacer("[", "", "]", "").Replace(title)
		links = append(links, fmt.Sprintf("- [%s](<%s>)", title, value))
		if len(links) >= 8 {
			break
		}
	}
	if len(links) == 0 {
		return content
	}
	return strings.TrimSpace(content) + "\n\n**来源**\n\n" + strings.Join(links, "\n")
}

func providerReasoningEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fast":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh":
		return "xhigh"
	default:
		return ""
	}
}

func (c *HTTPClient) Models(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	target, err := endpoint(baseURL, "models")
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("模型服务连接失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("模型服务返回 %d: %s", response.StatusCode, providerMessage(message))
	}
	if !isJSON(response.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("模型列表返回 %s 而不是 JSON，请检查 Base URL 是否指向 API 根路径（通常以 /v1 结尾）", contentType(response.Header.Get("Content-Type")))
	}
	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&models); err != nil {
		return nil, fmt.Errorf("模型列表返回了无效 JSON")
	}
	unique := make(map[string]bool, len(models.Data))
	result := make([]string, 0, len(models.Data))
	for _, item := range models.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" || unique[id] || len(id) > 160 {
			continue
		}
		unique[id] = true
		result = append(result, id)
	}
	sort.Strings(result)
	if len(result) > 2000 {
		result = result[:2000]
	}
	return result, nil
}

func (c *HTTPClient) Test(ctx context.Context, input ConnectionTestRequest) (*ConnectionTest, error) {
	started := time.Now()
	models, err := c.Models(ctx, input.BaseURL, input.APIKey)
	if err != nil {
		return nil, err
	}
	completion, err := c.Complete(ctx, CompletionRequest{
		BaseURL: input.BaseURL, APIKey: input.APIKey, Model: input.Model, Protocol: input.Protocol,
		WebSearch: input.WebSearch, RequireTool: input.WebSearch,
		Messages: []Message{{Role: "user", Content: connectionTestPrompt(input.WebSearch)}}, Temperature: 0, ReasoningEffort: "fast",
	})
	if err != nil {
		return nil, fmt.Errorf("模型对话测试失败: %w", err)
	}
	if input.WebSearch && !completion.UsedWebSearch {
		return nil, fmt.Errorf("模型对话测试失败: 上游未执行联网搜索")
	}
	return &ConnectionTest{Latency: time.Since(started), Models: models}, nil
}

func connectionTestPrompt(webSearch bool) string {
	if webSearch {
		return "Search the web for the current UTC date, then reply with OK."
	}
	return "Reply with OK."
}

func isJSON(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && (mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"))
}

func contentType(value string) string {
	if value = strings.TrimSpace(value); value == "" {
		return "未知内容类型"
	}
	return value
}

func endpoint(baseURL, path string) (string, error) {
	target, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || !target.IsAbs() || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil {
		return "", fmt.Errorf("模型服务地址无效")
	}
	target.Path = strings.TrimRight(target.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return target.String(), nil
}

func providerMessage(body []byte) string {
	var payload struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && strings.TrimSpace(payload.Error.Message) != "" {
		return payload.Error.Message
	}
	if strings.TrimSpace(payload.Message) != "" {
		return payload.Message
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		return "未提供错误信息"
	}
	return message
}
