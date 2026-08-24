package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"ai-workbench/internal/model"
	"ai-workbench/internal/store"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalid      = errors.New("invalid input")
	ErrConflict     = errors.New("conflict")
	ErrNotFound     = errors.New("not found")
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{2,39}$`)

type Actor struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Source      string `json:"source"`
	Role        string `json:"role"`
}

func (actor Actor) IsAdmin() bool { return actor.Source == "internal" && actor.Role == RoleAdmin }

type UserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type UserPatch struct {
	DisplayName *string `json:"displayName,omitempty"`
	Password    string  `json:"password"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

type InternalUserView struct {
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Role        string    `json:"role"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreatedUser struct {
	User            InternalUserView `json:"user"`
	InitialPassword string           `json:"initialPassword"`
}

type SessionResult struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
	User        Actor     `json:"user"`
}

type Client struct {
	database        *store.Store
	permissionBase  string
	peopleBase      string
	peopleAuthorize string
	clientID        string
	clientSecret    string
	redirectURIs    map[string]bool
	httpClient      *http.Client
}

func New(database *store.Store, permissionBase, peopleBase, peopleAuthorize, clientID, clientSecret string, redirectURIs []string) *Client {
	allowed := make(map[string]bool, len(redirectURIs))
	for _, item := range redirectURIs {
		allowed[item] = true
	}
	return &Client{
		database: database, permissionBase: strings.TrimRight(permissionBase, "/"), peopleBase: strings.TrimRight(peopleBase, "/"),
		peopleAuthorize: peopleAuthorize, clientID: clientID, clientSecret: clientSecret, redirectURIs: allowed,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) AuthorizationURL(redirectURI string) (string, error) {
	if !c.redirectURIs[redirectURI] {
		return "", fmt.Errorf("redirect URI is not allowed")
	}
	state, err := randomToken(24)
	if err != nil {
		return "", err
	}
	if err := c.database.DB.Create(&model.OAuthState{StateHash: hash(state), RedirectURI: redirectURI, ExpiresAt: time.Now().UTC().Add(10 * time.Minute)}).Error; err != nil {
		return "", err
	}
	target, err := url.Parse(c.peopleAuthorize)
	if err != nil || !target.IsAbs() {
		return "", fmt.Errorf("invalid People authorize URL")
	}
	target.RawQuery = url.Values{
		"client_id": {c.clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"},
		"scope": {"openid profile"}, "state": {state},
	}.Encode()
	return target.String(), nil
}

func (c *Client) Exchange(ctx context.Context, code, state, redirectURI string) (*SessionResult, error) {
	var saved model.OAuthState
	err := c.database.DB.Where("state_hash = ? AND redirect_uri = ? AND expires_at > ?", hash(state), redirectURI, time.Now().UTC()).First(&saved).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if err := c.database.DB.Delete(&saved).Error; err != nil {
		return nil, err
	}
	actor, err := c.exchangePeople(ctx, strings.TrimSpace(code), redirectURI)
	if err != nil {
		return nil, err
	}
	return c.createSession(*actor)
}

func (c *Client) InternalLogin(_ context.Context, username, password string) (*SessionResult, error) {
	username = strings.TrimSpace(username)
	var user model.InternalUser
	if err := c.database.DB.Where("username = ? AND enabled = ?", username, true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrUnauthorized
	}
	return c.createSession(Actor{ID: "internal:" + user.Username, Username: user.Username, DisplayName: user.DisplayName, Source: "internal", Role: user.Role})
}

func (c *Client) createSession(actor Actor) (*SessionResult, error) {
	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().UTC().Add(12 * time.Hour)
	if actor.Role == "" {
		actor.Role = RoleUser
	}
	if err := c.database.DB.Create(&model.Session{TokenHash: hash(token), Username: actor.Username, DisplayName: actor.DisplayName, Source: actor.Source, Role: actor.Role, ExpiresAt: expiresAt}).Error; err != nil {
		return nil, err
	}
	return &SessionResult{AccessToken: token, ExpiresAt: expiresAt, User: actor}, nil
}

func (c *Client) Authenticate(ctx context.Context, token string) (*Actor, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrUnauthorized
	}
	var session model.Session
	if err := c.database.DB.Where("token_hash = ? AND expires_at > ?", hash(token), time.Now().UTC()).First(&session).Error; err == nil {
		source, role := session.Source, session.Role
		if source == "" {
			source = "people"
		}
		if role == "" {
			role = RoleUser
		}
		if source == "internal" {
			var user model.InternalUser
			if err := c.database.DB.Where("username = ? AND enabled = ?", session.Username, true).First(&user).Error; err != nil {
				return nil, ErrUnauthorized
			}
			return &Actor{ID: "internal:" + user.Username, Username: user.Username, DisplayName: user.DisplayName, Source: source, Role: user.Role}, nil
		}
		return &Actor{ID: session.Username, Username: session.Username, DisplayName: session.DisplayName, Source: source, Role: role}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return c.permissionIdentity(ctx, token)
}

func (c *Client) Users(actor Actor) ([]InternalUserView, error) {
	if !actor.IsAdmin() {
		return nil, ErrForbidden
	}
	var users []model.InternalUser
	if err := c.database.DB.Order("CASE WHEN role = 'admin' THEN 0 ELSE 1 END, created_at ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	result := make([]InternalUserView, 0, len(users))
	for _, user := range users {
		result = append(result, userView(user))
	}
	return result, nil
}

func (c *Client) CreateUser(actor Actor, input UserInput) (*CreatedUser, error) {
	if !actor.IsAdmin() {
		return nil, ErrForbidden
	}
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !usernamePattern.MatchString(input.Username) || len([]rune(input.DisplayName)) > 120 {
		return nil, ErrInvalid
	}
	if input.DisplayName == "" {
		input.DisplayName = input.Username
	}
	initialPassword := input.Password
	if initialPassword == "" {
		initialPassword = input.Username + "@123"
	}
	if len(initialPassword) < 8 || len(initialPassword) > 128 {
		return nil, ErrInvalid
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(initialPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := model.InternalUser{Username: input.Username, DisplayName: input.DisplayName, PasswordHash: string(passwordHash), Role: RoleUser, Enabled: true}
	if err := c.database.DB.Create(&user).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrConflict
		}
		return nil, err
	}
	return &CreatedUser{User: userView(user), InitialPassword: initialPassword}, nil
}

func (c *Client) UpdateUser(actor Actor, username string, patch UserPatch) (*InternalUserView, error) {
	if !actor.IsAdmin() {
		return nil, ErrForbidden
	}
	username = strings.TrimSpace(username)
	var user model.InternalUser
	if err := c.database.DB.First(&user, "username = ?", username).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if patch.DisplayName != nil {
		name := strings.TrimSpace(*patch.DisplayName)
		if name == "" || len([]rune(name)) > 120 {
			return nil, ErrInvalid
		}
		user.DisplayName = name
	}
	if patch.Enabled != nil {
		if user.Role == RoleAdmin && !*patch.Enabled {
			return nil, ErrConflict
		}
		user.Enabled = *patch.Enabled
	}
	passwordChanged := false
	if patch.Password != "" {
		if len(patch.Password) < 8 || len(patch.Password) > 128 {
			return nil, ErrInvalid
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(patch.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		user.PasswordHash = string(passwordHash)
		passwordChanged = true
	}
	if err := c.database.DB.Save(&user).Error; err != nil {
		return nil, err
	}
	if passwordChanged || (patch.Enabled != nil && !*patch.Enabled) {
		if err := c.database.DB.Where("username = ? AND source = ?", user.Username, "internal").Delete(&model.Session{}).Error; err != nil {
			return nil, err
		}
	}
	view := userView(user)
	return &view, nil
}

func (c *Client) DeleteUser(actor Actor, username string) error {
	if !actor.IsAdmin() {
		return ErrForbidden
	}
	username = strings.TrimSpace(username)
	var user model.InternalUser
	if err := c.database.DB.First(&user, "username = ?", username).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if user.Role == RoleAdmin {
		return ErrConflict
	}
	return c.database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("username = ? AND source = ?", user.Username, "internal").Delete(&model.Session{}).Error; err != nil {
			return err
		}
		return tx.Delete(&user).Error
	})
}

func userView(user model.InternalUser) InternalUserView {
	return InternalUserView{Username: user.Username, DisplayName: user.DisplayName, Role: user.Role, Enabled: user.Enabled, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}
}

func (c *Client) Logout(token string) error {
	return c.database.DB.Where("token_hash = ?", hash(strings.TrimSpace(token))).Delete(&model.Session{}).Error
}

func (c *Client) exchangePeople(ctx context.Context, code, redirectURI string) (*Actor, error) {
	values := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.peopleBase+"/oauth/token", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(c.clientID, c.clientSecret)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("People token request: %w", err)
	}
	defer response.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil || response.StatusCode != http.StatusOK || token.AccessToken == "" {
		return nil, ErrUnauthorized
	}
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, c.peopleBase+"/oauth/userinfo", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	response, err = c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("People userinfo request: %w", err)
	}
	defer response.Body.Close()
	var employee struct{ ID, Username, DisplayName, Status string }
	if err := json.NewDecoder(response.Body).Decode(&employee); err != nil || response.StatusCode != http.StatusOK || employee.Username == "" || employee.Status != "enabled" {
		return nil, ErrUnauthorized
	}
	return &Actor{ID: employee.Username, Username: employee.Username, DisplayName: employee.DisplayName, Source: "people", Role: RoleUser}, nil
}

func (c *Client) permissionIdentity(ctx context.Context, token string) (*Actor, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.permissionBase+"/auth/me", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Permission identity request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrUnauthorized
	}
	var payload struct {
		Data struct {
			User struct{ ID, Username, DisplayName string } `json:"user"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload.Data.User.Username == "" {
		return nil, ErrUnauthorized
	}
	user := payload.Data.User
	return &Actor{ID: user.Username, Username: user.Username, DisplayName: user.DisplayName, Source: "permission", Role: RoleUser}, nil
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
