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
}

type ConnectionTest struct {
	Latency time.Duration
	Models  []string
}

type Client interface {
	Complete(context.Context, CompletionRequest) (*CompletionResult, error)
	Test(context.Context, string, string, string) (*ConnectionTest, error)
}

type ModelCatalogClient interface {
	Models(context.Context, string, string) ([]string, error)
}

type HTTPClient struct{ client *http.Client }

func New() *HTTPClient { return &HTTPClient{client: &http.Client{Timeout: 90 * time.Second}} }

func (c *HTTPClient) Complete(ctx context.Context, input CompletionRequest) (*CompletionResult, error) {
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

func providerReasoningEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fast":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
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

func (c *HTTPClient) Test(ctx context.Context, baseURL, apiKey, model string) (*ConnectionTest, error) {
	started := time.Now()
	models, err := c.Models(ctx, baseURL, apiKey)
	if err != nil {
		return nil, err
	}
	_, err = c.Complete(ctx, CompletionRequest{
		BaseURL: baseURL, APIKey: apiKey, Model: model,
		Messages: []Message{{Role: "user", Content: "Reply with OK."}}, Temperature: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("模型对话测试失败: %w", err)
	}
	return &ConnectionTest{Latency: time.Since(started), Models: models}, nil
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
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && strings.TrimSpace(payload.Error.Message) != "" {
		return payload.Error.Message
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		return "未提供错误信息"
	}
	return message
}
