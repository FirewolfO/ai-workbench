package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"ai-workbench/internal/content"
	"ai-workbench/internal/frontier"
	"ai-workbench/internal/identity"
	"ai-workbench/internal/workbench"
)

type authenticator interface {
	AuthorizationURL(string) (string, error)
	Exchange(context.Context, string, string, string) (*identity.SessionResult, error)
	InternalLogin(context.Context, string, string) (*identity.SessionResult, error)
	Authenticate(context.Context, string) (*identity.Actor, error)
	Users(identity.Actor) ([]identity.InternalUserView, error)
	CreateUser(identity.Actor, identity.UserInput) (*identity.CreatedUser, error)
	UpdateUser(identity.Actor, string, identity.UserPatch) (*identity.InternalUserView, error)
	DeleteUser(identity.Actor, string) error
	Logout(string) error
}

type Server struct {
	address        string
	allowedOrigins map[string]bool
	auth           authenticator
	workbench      *workbench.Service
	content        *content.Service
	frontier       *frontier.Service
}

type actorContextKey struct{}

func New(address string, allowedOrigins []string, auth authenticator, service *workbench.Service, contentService *content.Service, frontierService *frontier.Service) *http.Server {
	server := &Server{address: address, allowedOrigins: map[string]bool{}, auth: auth, workbench: service, content: contentService, frontier: frontierService}
	for _, origin := range allowedOrigins {
		server.allowedOrigins[origin] = true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /api/v1/auth/oauth/url", server.oauthURL)
	mux.HandleFunc("POST /api/v1/auth/oauth/callback", server.oauthCallback)
	mux.HandleFunc("POST /api/v1/auth/internal/login", server.internalLogin)
	mux.Handle("GET /api/v1/auth/me", server.requireAuth(http.HandlerFunc(server.me)))
	mux.Handle("POST /api/v1/auth/logout", server.requireAuth(http.HandlerFunc(server.logout)))
	mux.Handle("GET /api/v1/admin/users", server.requireAuth(http.HandlerFunc(server.users)))
	mux.Handle("POST /api/v1/admin/users", server.requireAuth(http.HandlerFunc(server.createUser)))
	mux.Handle("PATCH /api/v1/admin/users/{username}", server.requireAuth(http.HandlerFunc(server.updateUser)))
	mux.Handle("DELETE /api/v1/admin/users/{username}", server.requireAuth(http.HandlerFunc(server.deleteUser)))
	mux.Handle("GET /api/v1/dashboard", server.userAccess(http.HandlerFunc(server.dashboard)))
	mux.Handle("GET /api/v1/models", server.userAccess(http.HandlerFunc(server.models)))
	mux.Handle("GET /api/v1/providers", server.requireAuth(http.HandlerFunc(server.providers)))
	mux.Handle("POST /api/v1/providers", server.requireAuth(http.HandlerFunc(server.createProvider)))
	mux.Handle("PUT /api/v1/providers/{id}", server.requireAuth(http.HandlerFunc(server.updateProvider)))
	mux.Handle("DELETE /api/v1/providers/{id}", server.requireAuth(http.HandlerFunc(server.deleteProvider)))
	mux.Handle("POST /api/v1/providers/{id}/test", server.requireAuth(http.HandlerFunc(server.testProvider)))
	mux.Handle("GET /api/v1/prompts", server.userAccess(http.HandlerFunc(server.prompts)))
	mux.Handle("POST /api/v1/prompts", server.userAccess(http.HandlerFunc(server.createPrompt)))
	mux.Handle("PUT /api/v1/prompts/{id}", server.userAccess(http.HandlerFunc(server.updatePrompt)))
	mux.Handle("DELETE /api/v1/prompts/{id}", server.userAccess(http.HandlerFunc(server.deletePrompt)))
	mux.Handle("POST /api/v1/prompts/{id}/use", server.userAccess(http.HandlerFunc(server.usePrompt)))
	mux.Handle("GET /api/v1/conversations", server.userAccess(http.HandlerFunc(server.conversations)))
	mux.Handle("POST /api/v1/conversations", server.userAccess(http.HandlerFunc(server.createConversation)))
	mux.Handle("GET /api/v1/conversations/{id}", server.userAccess(http.HandlerFunc(server.conversation)))
	mux.Handle("PATCH /api/v1/conversations/{id}", server.userAccess(http.HandlerFunc(server.updateConversation)))
	mux.Handle("DELETE /api/v1/conversations/{id}", server.userAccess(http.HandlerFunc(server.deleteConversation)))
	mux.Handle("POST /api/v1/conversations/{id}/messages", server.userAccess(http.HandlerFunc(server.sendMessage)))
	mux.Handle("POST /api/v1/conversations/{id}/messages/async", server.userAccess(http.HandlerFunc(server.queueMessage)))
	mux.Handle("POST /api/v1/conversations/{id}/stop", server.userAccess(http.HandlerFunc(server.stopGeneration)))
	mux.Handle("POST /api/v1/attachments", server.userAccess(http.HandlerFunc(server.createAttachment)))
	mux.Handle("DELETE /api/v1/attachments/{id}", server.userAccess(http.HandlerFunc(server.deleteAttachment)))
	mux.Handle("GET /api/v1/content/status", server.userAccess(http.HandlerFunc(server.contentStatus)))
	mux.Handle("GET /api/v1/news", server.userAccess(http.HandlerFunc(server.news)))
	mux.Handle("POST /api/v1/news/refresh", server.userAccess(http.HandlerFunc(server.refreshNews)))
	mux.Handle("POST /api/v1/news/summaries", server.userAccess(http.HandlerFunc(server.summarizeNews)))
	mux.Handle("PUT /api/v1/news/{id}/favorite", server.userAccess(http.HandlerFunc(server.favoriteNews)))
	mux.Handle("GET /api/v1/frontier", server.userAccess(http.HandlerFunc(server.frontierProjects)))
	return &http.Server{Addr: address, Handler: server.middleware(mux), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 120 * time.Second}
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("Cache-Control", "no-store")
		origin := request.Header.Get("Origin")
		if origin != "" && s.allowedOrigins[origin] {
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Vary", "Origin")
			writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-AI-Workbench-Device-ID")
			writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if request.Method == http.MethodOptions {
			if origin != "" && !s.allowedOrigins[origin] {
				writer.WriteHeader(http.StatusForbidden)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := bearer(request.Header.Get("Authorization"))
		actor, err := s.auth.Authenticate(request.Context(), token)
		if err != nil {
			fail(writer, http.StatusUnauthorized, "UNAUTHORIZED", "登录已过期，请重新登录")
			return
		}
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), actorContextKey{}, *actor)))
	})
}

func (s *Server) userAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.TrimSpace(request.Header.Get("Authorization")) != "" {
			actor, err := s.auth.Authenticate(request.Context(), bearer(request.Header.Get("Authorization")))
			if err != nil {
				fail(writer, http.StatusUnauthorized, "UNAUTHORIZED", "登录已过期，请重新登录")
				return
			}
			next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), actorContextKey{}, *actor)))
			return
		}
		actor, ok := anonymousActor(request.Header.Get("X-AI-Workbench-Device-ID"))
		if !ok {
			fail(writer, http.StatusUnauthorized, "DEVICE_ID_REQUIRED", "缺少设备标识，请刷新后重试")
			return
		}
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), actorContextKey{}, actor)))
	})
}

func anonymousActor(deviceID string) (identity.Actor, bool) {
	deviceID = strings.TrimSpace(deviceID)
	if len(deviceID) < 20 || len(deviceID) > 128 {
		return identity.Actor{}, false
	}
	for _, character := range deviceID {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("._~-", character)) {
			return identity.Actor{}, false
		}
	}
	digest := sha256.Sum256([]byte(deviceID))
	id := "anonymous:" + hex.EncodeToString(digest[:])
	return identity.Actor{ID: id, Username: id, DisplayName: "访客", Source: "anonymous", Role: identity.RoleUser}, true
}

func (s *Server) health(writer http.ResponseWriter, _ *http.Request) {
	write(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) oauthURL(writer http.ResponseWriter, request *http.Request) {
	target, err := s.auth.AuthorizationURL(request.URL.Query().Get("redirect_uri"))
	if err != nil {
		fail(writer, http.StatusBadRequest, "INVALID_REDIRECT_URI", "OAuth 回调地址无效")
		return
	}
	write(writer, http.StatusOK, map[string]string{"url": target})
}

func (s *Server) oauthCallback(writer http.ResponseWriter, request *http.Request) {
	var input struct{ Code, State, RedirectURI string }
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.auth.Exchange(request.Context(), input.Code, input.State, input.RedirectURI)
	if err != nil {
		fail(writer, http.StatusUnauthorized, "OAUTH_FAILED", "People OAuth 登录失败")
		return
	}
	write(writer, http.StatusOK, result)
}

func (s *Server) internalLogin(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.auth.InternalLogin(request.Context(), input.Username, input.Password)
	if err != nil {
		if errors.Is(err, identity.ErrUnauthorized) {
			fail(writer, http.StatusUnauthorized, "INVALID_CREDENTIALS", "用户名或密码错误")
			return
		}
		failError(writer, err)
		return
	}
	write(writer, http.StatusOK, result)
}

func (s *Server) me(writer http.ResponseWriter, request *http.Request) {
	write(writer, http.StatusOK, map[string]any{"user": actor(request)})
}

func (s *Server) logout(writer http.ResponseWriter, request *http.Request) {
	if err := s.auth.Logout(bearer(request.Header.Get("Authorization"))); err != nil {
		failError(writer, err)
		return
	}
	write(writer, http.StatusOK, map[string]bool{"loggedOut": true})
}

func (s *Server) users(writer http.ResponseWriter, request *http.Request) {
	result, err := s.auth.Users(actor(request))
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) createUser(writer http.ResponseWriter, request *http.Request) {
	var input identity.UserInput
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.auth.CreateUser(actor(request), input)
	respond(writer, result, err, http.StatusCreated)
}

func (s *Server) updateUser(writer http.ResponseWriter, request *http.Request) {
	var input identity.UserPatch
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.auth.UpdateUser(actor(request), request.PathValue("username"), input)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) deleteUser(writer http.ResponseWriter, request *http.Request) {
	err := s.auth.DeleteUser(actor(request), request.PathValue("username"))
	respond(writer, map[string]bool{"deleted": err == nil}, err, http.StatusOK)
}

func (s *Server) dashboard(writer http.ResponseWriter, request *http.Request) {
	result, err := s.workbench.Dashboard(actor(request))
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) models(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Query().Get("refresh") == "true" {
		if err := s.workbench.RefreshAvailableModels(request.Context(), actor(request)); err != nil {
			respond(writer, nil, err, http.StatusOK)
			return
		}
	}
	result, err := s.workbench.AvailableModels(actor(request))
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) providers(writer http.ResponseWriter, request *http.Request) {
	result, err := s.workbench.Providers(actor(request))
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) createProvider(writer http.ResponseWriter, request *http.Request) {
	var input workbench.ProviderInput
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.workbench.CreateProvider(actor(request), input)
	respond(writer, result, err, http.StatusCreated)
}

func (s *Server) updateProvider(writer http.ResponseWriter, request *http.Request) {
	var input workbench.ProviderInput
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.workbench.UpdateProvider(actor(request), request.PathValue("id"), input)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) deleteProvider(writer http.ResponseWriter, request *http.Request) {
	err := s.workbench.DeleteProvider(actor(request), request.PathValue("id"))
	respond(writer, map[string]bool{"deleted": err == nil}, err, http.StatusOK)
}

func (s *Server) testProvider(writer http.ResponseWriter, request *http.Request) {
	result, err := s.workbench.TestProvider(request.Context(), actor(request), request.PathValue("id"))
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) prompts(writer http.ResponseWriter, request *http.Request) {
	result, err := s.workbench.Prompts(actor(request), request.URL.Query().Get("search"))
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) createPrompt(writer http.ResponseWriter, request *http.Request) {
	var input workbench.PromptInput
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.workbench.CreatePrompt(actor(request), input)
	respond(writer, result, err, http.StatusCreated)
}

func (s *Server) updatePrompt(writer http.ResponseWriter, request *http.Request) {
	var input workbench.PromptInput
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.workbench.UpdatePrompt(actor(request), request.PathValue("id"), input)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) deletePrompt(writer http.ResponseWriter, request *http.Request) {
	err := s.workbench.DeletePrompt(actor(request), request.PathValue("id"))
	respond(writer, map[string]bool{"deleted": err == nil}, err, http.StatusOK)
}

func (s *Server) usePrompt(writer http.ResponseWriter, request *http.Request) {
	result, err := s.workbench.UsePrompt(actor(request), request.PathValue("id"))
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) conversations(writer http.ResponseWriter, request *http.Request) {
	result, err := s.workbench.Conversations(actor(request), request.URL.Query().Get("search"))
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) createConversation(writer http.ResponseWriter, request *http.Request) {
	var input workbench.ConversationInput
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.workbench.CreateConversation(actor(request), input)
	respond(writer, result, err, http.StatusCreated)
}

func (s *Server) conversation(writer http.ResponseWriter, request *http.Request) {
	result, err := s.workbench.Conversation(actor(request), request.PathValue("id"))
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) updateConversation(writer http.ResponseWriter, request *http.Request) {
	var input workbench.ConversationPatch
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.workbench.UpdateConversation(actor(request), request.PathValue("id"), input)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) deleteConversation(writer http.ResponseWriter, request *http.Request) {
	err := s.workbench.DeleteConversation(actor(request), request.PathValue("id"))
	respond(writer, map[string]bool{"deleted": err == nil}, err, http.StatusOK)
}

func (s *Server) sendMessage(writer http.ResponseWriter, request *http.Request) {
	var input workbench.MessageInput
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.workbench.SendMessage(request.Context(), actor(request), request.PathValue("id"), input)
	respond(writer, result, err, http.StatusCreated)
}

func (s *Server) queueMessage(writer http.ResponseWriter, request *http.Request) {
	var input workbench.MessageInput
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.workbench.QueueMessage(actor(request), request.PathValue("id"), input)
	respond(writer, result, err, http.StatusAccepted)
}

func (s *Server) stopGeneration(writer http.ResponseWriter, request *http.Request) {
	stopped, err := s.workbench.StopGeneration(actor(request), request.PathValue("id"))
	respond(writer, map[string]bool{"stopped": stopped}, err, http.StatusOK)
}

func (s *Server) createAttachment(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, (8<<20)+(1<<20))
	if err := request.ParseMultipartForm(8 << 20); err != nil {
		fail(writer, http.StatusBadRequest, "INVALID_ATTACHMENT", "附件无效或超过 8 MiB")
		return
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		fail(writer, http.StatusBadRequest, "INVALID_ATTACHMENT", "请选择要上传的附件")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		fail(writer, http.StatusBadRequest, "INVALID_ATTACHMENT", "附件无效或超过 8 MiB")
		return
	}
	result, err := s.workbench.CreateAttachment(actor(request), header.Filename, header.Header.Get("Content-Type"), data)
	respond(writer, result, err, http.StatusCreated)
}

func (s *Server) deleteAttachment(writer http.ResponseWriter, request *http.Request) {
	err := s.workbench.DeleteAttachment(actor(request), request.PathValue("id"))
	respond(writer, map[string]bool{"deleted": err == nil}, err, http.StatusOK)
}

func (s *Server) contentStatus(writer http.ResponseWriter, request *http.Request) {
	write(writer, http.StatusOK, s.content.Overview(actor(request)))
}

func (s *Server) news(writer http.ResponseWriter, request *http.Request) {
	result, err := s.content.News(
		actor(request), request.URL.Query().Get("search"), request.URL.Query().Get("source"), request.URL.Query().Get("favorite") == "true",
	)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) refreshNews(writer http.ResponseWriter, request *http.Request) {
	result, err := s.content.RefreshNews(request.Context())
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) summarizeNews(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		ArticleIDs []string `json:"articleIds"`
	}
	if !decode(writer, request, &input) {
		return
	}
	result, err := s.workbench.SummarizeNews(request.Context(), actor(request), input.ArticleIDs)
	respond(writer, result, err, http.StatusOK)
}

func (s *Server) favoriteNews(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Favorite bool `json:"favorite"`
	}
	if !decode(writer, request, &input) {
		return
	}
	err := s.content.FavoriteNews(actor(request), request.PathValue("id"), input.Favorite)
	respond(writer, map[string]bool{"favorite": input.Favorite}, err, http.StatusOK)
}

func (s *Server) frontierProjects(writer http.ResponseWriter, request *http.Request) {
	result, err := s.frontier.Discover(request.Context(), frontier.Query{
		Search: request.URL.Query().Get("search"), Category: request.URL.Query().Get("category"),
		Language: request.URL.Query().Get("language"), Period: request.URL.Query().Get("period"), Sort: request.URL.Query().Get("sort"),
	})
	respond(writer, result, err, http.StatusOK)
}

func actor(request *http.Request) identity.Actor {
	return request.Context().Value(actorContextKey{}).(identity.Actor)
}

func bearer(value string) string {
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
}

func decode(writer http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(writer, http.StatusBadRequest, "INVALID_REQUEST", "请求内容无效")
		return false
	}
	return true
}

func respond(writer http.ResponseWriter, data any, err error, successStatus int) {
	if err != nil {
		failError(writer, err)
		return
	}
	write(writer, successStatus, data)
}

func failError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrUnauthorized):
		fail(writer, http.StatusUnauthorized, "UNAUTHORIZED", "登录已过期，请重新登录")
	case errors.Is(err, identity.ErrForbidden), errors.Is(err, workbench.ErrForbidden):
		fail(writer, http.StatusForbidden, "FORBIDDEN", "当前账号没有权限执行此操作")
	case errors.Is(err, identity.ErrInvalid):
		fail(writer, http.StatusBadRequest, "INVALID_REQUEST", "账号信息或密码格式无效")
	case errors.Is(err, identity.ErrNotFound):
		fail(writer, http.StatusNotFound, "NOT_FOUND", "用户不存在")
	case errors.Is(err, identity.ErrConflict):
		fail(writer, http.StatusConflict, "USER_CONFLICT", "用户名已存在或管理员账号不能执行此操作")
	case errors.Is(err, content.ErrInvalid):
		fail(writer, http.StatusBadRequest, "INVALID_REQUEST", "X 用户名格式无效或已关注")
	case errors.Is(err, content.ErrNotFound):
		fail(writer, http.StatusNotFound, "NOT_FOUND", "内容不存在")
	case errors.Is(err, content.ErrXNotConfigured):
		fail(writer, http.StatusServiceUnavailable, "X_NOT_CONFIGURED", "尚未配置 X API Bearer Token")
	case errors.Is(err, content.ErrUpstream):
		fail(writer, http.StatusBadGateway, "CONTENT_SOURCE_UNAVAILABLE", "内容源暂时不可用，请稍后重试")
	case errors.Is(err, frontier.ErrInvalid):
		fail(writer, http.StatusBadRequest, "INVALID_FRONTIER_QUERY", "前沿项目筛选条件无效")
	case errors.Is(err, frontier.ErrRateLimited):
		fail(writer, http.StatusTooManyRequests, "GITHUB_RATE_LIMITED", "GitHub 搜索额度已用完，请稍后重试或配置访问令牌")
	case errors.Is(err, frontier.ErrUnavailable):
		fail(writer, http.StatusBadGateway, "GITHUB_UNAVAILABLE", "GitHub 暂时不可用，请稍后重试")
	case errors.Is(err, workbench.ErrInvalid):
		fail(writer, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
	case errors.Is(err, workbench.ErrNotFound):
		fail(writer, http.StatusNotFound, "NOT_FOUND", "资源不存在")
	case errors.Is(err, workbench.ErrConflict):
		fail(writer, http.StatusConflict, "RESOURCE_CONFLICT", "资源正在使用或已有生成任务正在运行")
	case errors.Is(err, workbench.ErrCanceled):
		fail(writer, http.StatusConflict, "GENERATION_STOPPED", "生成已停止")
	case errors.Is(err, workbench.ErrProvider):
		fail(writer, http.StatusBadGateway, "MODEL_UNAVAILABLE", strings.TrimPrefix(err.Error(), workbench.ErrProvider.Error()+": "))
	case errors.Is(err, workbench.ErrNoProvider):
		fail(writer, http.StatusConflict, "MODEL_NOT_CONFIGURED", "请先配置并启用一个模型连接")
	default:
		log.Printf("AI Workbench request failed: %v", err)
		fail(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用")
	}
}

func write(writer http.ResponseWriter, status int, data any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"code": "OK", "message": "success", "data": data})
}

func fail(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"code": code, "message": message, "data": nil})
}
