package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type ToolHandler func(context.Context, string, json.RawMessage) (string, error)

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
	Tools           []ToolDefinition
	ToolHandler     ToolHandler
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

func New() *HTTPClient { return &HTTPClient{client: &http.Client{Timeout: 4 * time.Minute}} }

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
	messages := make([]any, 0, len(input.Messages)+8)
	for _, message := range input.Messages {
		messages = append(messages, message)
	}
	requestPayload := map[string]any{"model": input.Model}
	if effort := providerReasoningEffort(input.ReasoningEffort); effort != "" {
		requestPayload["reasoning_effort"] = effort
	} else {
		requestPayload["temperature"] = input.Temperature
	}
	if len(input.Tools) > 0 {
		requestPayload["tools"] = chatTools(input.Tools)
		requestPayload["tool_choice"] = "auto"
	}
	started := time.Now()
	promptTokens, completionTokens := 0, 0
	modelName := ""
	for round := 0; round < 5; round++ {
		requestPayload["messages"] = messages
		var responsePayload chatCompletionResponse
		if err := c.postJSON(ctx, target, input.APIKey, requestPayload, &responsePayload, "请检查 Base URL 是否指向 API 根路径（通常以 /v1 结尾）"); err != nil {
			if round == 0 && len(input.Tools) > 0 && isUnsupportedToolError(err) {
				delete(requestPayload, "tools")
				delete(requestPayload, "tool_choice")
				continue
			}
			return nil, err
		}
		if len(responsePayload.Choices) == 0 {
			return nil, fmt.Errorf("模型服务返回了无效响应")
		}
		promptTokens += responsePayload.Usage.PromptTokens
		completionTokens += responsePayload.Usage.CompletionTokens
		if strings.TrimSpace(responsePayload.Model) != "" {
			modelName = responsePayload.Model
		}
		answer := responsePayload.Choices[0].Message
		if len(answer.ToolCalls) == 0 {
			if strings.TrimSpace(answer.Content) == "" {
				return nil, fmt.Errorf("模型服务返回了无效响应")
			}
			return &CompletionResult{Content: answer.Content, Model: modelName, PromptTokens: promptTokens, CompletionTokens: completionTokens, Latency: time.Since(started)}, nil
		}
		if input.ToolHandler == nil {
			return nil, fmt.Errorf("模型请求了未配置的后台工具")
		}
		messages = append(messages, answer)
		for _, call := range answer.ToolCalls {
			output, toolErr := input.ToolHandler(ctx, call.Function.Name, json.RawMessage(call.Function.Arguments))
			if toolErr != nil {
				output = toolErrorOutput(toolErr)
			}
			messages = append(messages, map[string]any{"role": "tool", "tool_call_id": call.ID, "content": output})
		}
	}
	return nil, fmt.Errorf("模型工具调用次数超过限制")
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatAssistantMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatAssistantMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type responsesOutputItem struct {
	Type      string `json:"type"`
	Status    string `json:"status"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Action    struct {
		Sources []struct {
			URL   string `json:"url"`
			Title string `json:"title"`
			Name  string `json:"name"`
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
	inputItems := make([]any, 0, len(input.Messages)+8)
	for _, message := range responseMessages(input.Messages) {
		inputItems = append(inputItems, message)
	}
	requestPayload := map[string]any{"model": input.Model, "store": false}
	if effort := providerReasoningEffort(input.ReasoningEffort); effort != "" {
		requestPayload["reasoning"] = map[string]string{"effort": effort}
	}
	tools := responseTools(input.Tools)
	if input.WebSearch {
		tools = append([]map[string]any{{"type": "web_search"}}, tools...)
	}
	if len(tools) > 0 {
		requestPayload["tools"] = tools
		requestPayload["tool_choice"] = "auto"
		include := []string{"reasoning.encrypted_content"}
		if input.WebSearch {
			include = append(include, "web_search_call.action.sources")
		}
		requestPayload["include"] = include
		if input.RequireTool {
			requestPayload["tool_choice"] = "required"
		}
	}
	started := time.Now()
	promptTokens, completionTokens := 0, 0
	modelName := ""
	allSources := make([]responseSource, 0)
	usedWebSearch := false
	droppedEncryptedReasoning := false
	droppedFunctions := false
	for round := 0; round < 5; round++ {
		requestPayload["input"] = inputItems
		var responsePayload struct {
			Model  string            `json:"model"`
			Status string            `json:"status"`
			Output []json.RawMessage `json:"output"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := c.postJSON(ctx, target, input.APIKey, requestPayload, &responsePayload, "请检查 Base URL 是否指向 Responses API 根路径"); err != nil {
			if !droppedEncryptedReasoning && isUnsupportedEncryptedReasoning(err) {
				droppedEncryptedReasoning = true
				if input.WebSearch {
					requestPayload["include"] = []string{"web_search_call.action.sources"}
				} else {
					delete(requestPayload, "include")
				}
				continue
			}
			if !droppedFunctions && len(input.Tools) > 0 && isUnsupportedToolError(err) {
				droppedFunctions = true
				if input.WebSearch {
					requestPayload["tools"] = []map[string]any{{"type": "web_search"}}
				} else {
					delete(requestPayload, "tools")
					delete(requestPayload, "tool_choice")
				}
				continue
			}
			return nil, err
		}
		if responsePayload.Error != nil && strings.TrimSpace(responsePayload.Error.Message) != "" {
			return nil, fmt.Errorf("模型服务返回错误: %s", responsePayload.Error.Message)
		}
		promptTokens += responsePayload.Usage.InputTokens
		completionTokens += responsePayload.Usage.OutputTokens
		if strings.TrimSpace(responsePayload.Model) != "" {
			modelName = responsePayload.Model
		}
		output := make([]responsesOutputItem, 0, len(responsePayload.Output))
		for _, raw := range responsePayload.Output {
			var item responsesOutputItem
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, fmt.Errorf("模型服务返回了无效响应")
			}
			output = append(output, item)
		}
		content, sources, searched := responseOutput(output)
		allSources = append(allSources, sources...)
		usedWebSearch = usedWebSearch || searched
		calls := responseFunctionCalls(output)
		if len(calls) == 0 {
			if strings.TrimSpace(content) == "" {
				return nil, fmt.Errorf("模型服务返回了无效响应")
			}
			return &CompletionResult{
				Content: appendSources(content, allSources), Model: modelName, PromptTokens: promptTokens,
				CompletionTokens: completionTokens, Latency: time.Since(started), UsedWebSearch: usedWebSearch,
			}, nil
		}
		if input.ToolHandler == nil {
			return nil, fmt.Errorf("模型请求了未配置的后台工具")
		}
		for _, raw := range responsePayload.Output {
			inputItems = append(inputItems, raw)
		}
		for _, call := range calls {
			output, toolErr := input.ToolHandler(ctx, call.Name, json.RawMessage(call.Arguments))
			if toolErr != nil {
				output = toolErrorOutput(toolErr)
			}
			inputItems = append(inputItems, map[string]any{"type": "function_call_output", "call_id": call.CallID, "output": output})
		}
	}
	return nil, fmt.Errorf("模型工具调用次数超过限制")
}

func chatTools(definitions []ToolDefinition) []map[string]any {
	result := make([]map[string]any, 0, len(definitions))
	for _, definition := range definitions {
		result = append(result, map[string]any{"type": "function", "function": map[string]any{
			"name": definition.Name, "description": definition.Description, "parameters": definition.Parameters,
		}})
	}
	return result
}

func responseTools(definitions []ToolDefinition) []map[string]any {
	result := make([]map[string]any, 0, len(definitions))
	for _, definition := range definitions {
		result = append(result, map[string]any{
			"type": "function", "name": definition.Name, "description": definition.Description, "parameters": definition.Parameters,
		})
	}
	return result
}

func responseFunctionCalls(output []responsesOutputItem) []responsesOutputItem {
	result := make([]responsesOutputItem, 0)
	for _, item := range output {
		if item.Type == "function_call" && strings.TrimSpace(item.CallID) != "" && strings.TrimSpace(item.Name) != "" {
			result = append(result, item)
		}
	}
	return result
}

func toolErrorOutput(err error) string {
	value, _ := json.Marshal(map[string]string{"error": err.Error()})
	return string(value)
}

type modelServiceError struct {
	status  int
	message string
}

func (e *modelServiceError) Error() string {
	return fmt.Sprintf("模型服务返回 %d: %s", e.status, e.message)
}

func isUnsupportedToolError(err error) bool {
	var serviceError *modelServiceError
	if !errors.As(err, &serviceError) || (serviceError.status != http.StatusBadRequest && serviceError.status != http.StatusUnprocessableEntity) {
		return false
	}
	message := strings.ToLower(serviceError.message)
	return strings.Contains(message, "tool") || strings.Contains(message, "function")
}

func isUnsupportedEncryptedReasoning(err error) bool {
	var serviceError *modelServiceError
	if !errors.As(err, &serviceError) || (serviceError.status != http.StatusBadRequest && serviceError.status != http.StatusUnprocessableEntity) {
		return false
	}
	message := strings.ToLower(serviceError.message)
	return strings.Contains(message, "reasoning.encrypted_content") || (strings.Contains(message, "include") && strings.Contains(message, "reasoning"))
}

func (c *HTTPClient) postJSON(ctx context.Context, target, apiKey string, payload any, result any, hint string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("模型服务连接失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return &modelServiceError{status: response.StatusCode, message: providerMessage(message)}
	}
	if !isJSON(response.Header.Get("Content-Type")) {
		return fmt.Errorf("模型服务返回 %s 而不是 JSON，%s", contentType(response.Header.Get("Content-Type")), hint)
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("模型服务返回了无效响应")
	}
	return nil
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
	Name  string
}

func responseOutput(output []responsesOutputItem) (string, []responseSource, bool) {
	texts := make([]string, 0)
	sources := make([]responseSource, 0)
	usedWebSearch := false
	for _, item := range output {
		if item.Type == "web_search_call" {
			usedWebSearch = true
			for _, source := range item.Action.Sources {
				sources = append(sources, responseSource{URL: source.URL, Title: source.Title, Name: source.Name})
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
	items := make([]string, 0, len(sources))
	for _, source := range sources {
		value := strings.TrimSpace(source.URL)
		label := strings.TrimSpace(source.Title)
		if label == "" {
			label = strings.TrimSpace(source.Name)
		}
		label = strings.NewReplacer("[", "", "]", "", "\r", " ", "\n", " ").Replace(label)
		if len([]rune(label)) > 160 {
			label = string([]rune(label)[:160])
		}
		if value == "" {
			key := "name:" + label
			if label != "" && !seen[key] {
				seen[key] = true
				items = append(items, "- "+label)
			}
			if len(items) >= 8 {
				break
			}
			continue
		}
		if seen[value] {
			continue
		}
		target, err := url.Parse(value)
		if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
			continue
		}
		seen[value] = true
		if label == "" {
			label = target.Host
		}
		items = append(items, fmt.Sprintf("- [%s](<%s>)", label, value))
		if len(items) >= 8 {
			break
		}
	}
	if len(items) == 0 {
		return content
	}
	return strings.TrimSpace(content) + "\n\n**来源**\n\n" + strings.Join(items, "\n")
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
