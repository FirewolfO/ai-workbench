package frontier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid frontier request")
	ErrUnavailable = errors.New("github unavailable")
	ErrRateLimited = errors.New("github rate limited")
)

const (
	defaultBaseURL = "https://api.github.com"
	cacheTTL       = 5 * time.Minute
)

var languagePattern = regexp.MustCompile(`^[A-Za-z0-9+#. _-]{1,40}$`)

type Query struct {
	Search   string
	Category string
	Language string
	Period   string
	Sort     string
}

type Result struct {
	Items          []Repository `json:"items"`
	Total          int          `json:"total"`
	GeneratedAt    time.Time    `json:"generatedAt"`
	GitHubTokenSet bool         `json:"githubTokenSet"`
	RateLimit      RateLimit    `json:"rateLimit"`
	Stale          bool         `json:"stale"`
}

type Repository struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	FullName    string    `json:"fullName"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Homepage    string    `json:"homepage"`
	Owner       string    `json:"owner"`
	OwnerAvatar string    `json:"ownerAvatar"`
	Category    string    `json:"category"`
	Language    string    `json:"language"`
	License     string    `json:"license"`
	Topics      []string  `json:"topics"`
	Stars       int       `json:"stars"`
	Forks       int       `json:"forks"`
	OpenIssues  int       `json:"openIssues"`
	Score       int       `json:"score"`
	Signals     []string  `json:"signals"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	PushedAt    time.Time `json:"pushedAt"`
}

type RateLimit struct {
	Limit     int        `json:"limit"`
	Remaining int        `json:"remaining"`
	ResetAt   *time.Time `json:"resetAt,omitempty"`
}

type cacheEntry struct {
	result    Result
	expiresAt time.Time
}

type Service struct {
	baseURL string
	token   string
	http    *http.Client
	now     func() time.Time
	mu      sync.RWMutex
	cache   map[string]cacheEntry
}

func New(baseURL, token string) *Service {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Service{
		baseURL: baseURL,
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: 20 * time.Second},
		now:     time.Now,
		cache:   make(map[string]cacheEntry),
	}
}

func (s *Service) Discover(ctx context.Context, input Query) (*Result, error) {
	query, err := normalizeQuery(input)
	if err != nil {
		return nil, err
	}
	key := query.cacheKey()
	if cached, ok := s.cached(key, false); ok {
		return &cached, nil
	}

	result, err := s.search(ctx, query)
	if err != nil {
		if cached, ok := s.cached(key, true); ok {
			cached.Stale = true
			return &cached, nil
		}
		return nil, err
	}
	s.mu.Lock()
	s.cache[key] = cacheEntry{result: *result, expiresAt: s.now().Add(cacheTTL)}
	s.mu.Unlock()
	return result, nil
}

type normalizedQuery struct {
	search, category, language, period, sort string
}

func normalizeQuery(input Query) (normalizedQuery, error) {
	query := normalizedQuery{
		search: strings.TrimSpace(input.Search), category: strings.TrimSpace(input.Category),
		language: strings.TrimSpace(input.Language), period: strings.TrimSpace(input.Period), sort: strings.TrimSpace(input.Sort),
	}
	if query.category == "" {
		query.category = "project"
	}
	if query.period == "" {
		query.period = "90d"
	}
	if query.sort == "" {
		query.sort = "recommended"
	}
	if len([]rune(query.search)) > 80 || !oneOf(query.category, "project", "skill", "plugin") ||
		!oneOf(query.period, "30d", "90d", "180d", "1y", "all") ||
		!oneOf(query.sort, "recommended", "stars", "updated", "newest") {
		return normalizedQuery{}, ErrInvalid
	}
	if query.language != "" && !languagePattern.MatchString(query.language) {
		return normalizedQuery{}, ErrInvalid
	}
	return query, nil
}

func (q normalizedQuery) cacheKey() string {
	return strings.Join([]string{strings.ToLower(q.search), q.category, strings.ToLower(q.language), q.period, q.sort}, "\x00")
}

func (s *Service) cached(key string, allowExpired bool) (Result, bool) {
	s.mu.RLock()
	entry, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok || (!allowExpired && s.now().After(entry.expiresAt)) || (allowExpired && s.now().After(entry.expiresAt.Add(time.Hour))) {
		return Result{}, false
	}
	return entry.result, true
}

func (s *Service) search(ctx context.Context, query normalizedQuery) (*Result, error) {
	endpoint, err := url.Parse(s.baseURL + "/search/repositories")
	if err != nil {
		return nil, fmt.Errorf("%w: invalid GitHub endpoint", ErrUnavailable)
	}
	parameters := endpoint.Query()
	parameters.Set("q", buildSearchQuery(query, s.now()))
	parameters.Set("per_page", "30")
	parameters.Set("order", "desc")
	if query.sort != "recommended" {
		parameters.Set("sort", map[string]string{"stars": "stars", "updated": "updated", "newest": "created"}[query.sort])
	} else {
		parameters.Set("sort", "stars")
	}
	endpoint.RawQuery = parameters.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "AI-Workbench/1.0")
	if s.token != "" {
		request.Header.Set("Authorization", "Bearer "+s.token)
	}
	response, err := s.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer response.Body.Close()
	rate := parseRateLimit(response.Header)
	if response.StatusCode == http.StatusTooManyRequests || (response.StatusCode == http.StatusForbidden && response.Header.Get("X-RateLimit-Remaining") == "0") {
		return nil, fmt.Errorf("%w: reset at %v", ErrRateLimited, rate.ResetAt)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: GitHub returned HTTP %d", ErrUnavailable, response.StatusCode)
	}
	var payload githubSearchResult
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: invalid GitHub response: %v", ErrUnavailable, err)
	}
	now := s.now().UTC()
	items := make([]Repository, 0, len(payload.Items))
	for _, item := range payload.Items {
		repository := item.repository(query.category, now)
		if repository.FullName != "" && repository.URL != "" {
			items = append(items, repository)
		}
	}
	if query.sort == "recommended" {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Score == items[j].Score {
				return items[i].Stars > items[j].Stars
			}
			return items[i].Score > items[j].Score
		})
	}
	return &Result{Items: items, Total: payload.TotalCount, GeneratedAt: now, GitHubTokenSet: s.token != "", RateLimit: rate}, nil
}

func buildSearchQuery(query normalizedQuery, now time.Time) string {
	terms := map[string]string{
		"project": "AI OR LLM OR agent in:name,description,topics",
		"skill":   "AI skill OR agent skill OR Codex skill OR Claude skill in:name,description,topics",
		"plugin":  "AI plugin OR LLM plugin OR MCP server in:name,description,topics",
	}[query.category]
	minimumStars := map[string]int{"project": 200, "skill": 20, "plugin": 20}[query.category]
	parts := []string{terms, fmt.Sprintf("stars:>=%d", minimumStars), "archived:false", "fork:false"}
	if query.search != "" {
		parts = append([]string{strconv.Quote(query.search)}, parts...)
	}
	if query.language != "" {
		parts = append(parts, "language:"+strconv.Quote(query.language))
	}
	days := map[string]int{"30d": 30, "90d": 90, "180d": 180, "1y": 365}[query.period]
	if days > 0 {
		parts = append(parts, "pushed:>="+now.UTC().AddDate(0, 0, -days).Format("2006-01-02"))
	}
	return strings.Join(parts, " ")
}

type githubSearchResult struct {
	TotalCount int                `json:"total_count"`
	Items      []githubRepository `json:"items"`
}

type githubRepository struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	FullName       string    `json:"full_name"`
	Description    string    `json:"description"`
	HTMLURL        string    `json:"html_url"`
	Homepage       string    `json:"homepage"`
	Language       string    `json:"language"`
	Stars          int       `json:"stargazers_count"`
	Forks          int       `json:"forks_count"`
	OpenIssues     int       `json:"open_issues_count"`
	Topics         []string  `json:"topics"`
	HasDiscussions bool      `json:"has_discussions"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	PushedAt       time.Time `json:"pushed_at"`
	Owner          struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
	License *struct {
		SPDXID string `json:"spdx_id"`
	} `json:"license"`
}

func (item githubRepository) repository(category string, now time.Time) Repository {
	license := ""
	if item.License != nil && item.License.SPDXID != "NOASSERTION" {
		license = item.License.SPDXID
	}
	score, signals := quality(item, now)
	topics := item.Topics
	if topics == nil {
		topics = []string{}
	}
	return Repository{
		ID: item.ID, Name: item.Name, FullName: item.FullName, Description: item.Description, URL: item.HTMLURL,
		Homepage: item.Homepage, Owner: item.Owner.Login, OwnerAvatar: item.Owner.AvatarURL, Category: category,
		Language: item.Language, License: license, Topics: topics, Stars: item.Stars, Forks: item.Forks,
		OpenIssues: item.OpenIssues, Score: score, Signals: signals, CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt, PushedAt: item.PushedAt,
	}
}

func quality(item githubRepository, now time.Time) (int, []string) {
	starScore := math.Min(math.Log10(float64(item.Stars)+1)/5.5, 1) * 45
	forkScore := math.Min(math.Log10(float64(item.Forks)+1)/4.5, 1) * 15
	daysSincePush := math.Max(now.Sub(item.PushedAt).Hours()/24, 0)
	freshnessScore := math.Exp(-daysSincePush/180) * 20
	maturity := 0.0
	signals := make([]string, 0, 4)
	if item.License != nil && item.License.SPDXID != "" && item.License.SPDXID != "NOASSERTION" {
		maturity += 6
		signals = append(signals, "开源协议清晰")
	}
	if len(item.Topics) > 0 {
		maturity += 4
	}
	if strings.TrimSpace(item.Description) != "" {
		maturity += 4
	}
	if strings.TrimSpace(item.Homepage) != "" {
		maturity += 3
	}
	if item.HasDiscussions {
		maturity += 3
		signals = append(signals, "社区讨论开放")
	}
	if daysSincePush <= 30 {
		signals = append(signals, "近 30 天活跃")
	} else if daysSincePush <= 90 {
		signals = append(signals, "近 90 天活跃")
	}
	if item.Stars >= 10000 {
		signals = append(signals, "社区关注度高")
	} else if item.Stars >= 1000 {
		signals = append(signals, "已有社区验证")
	}
	return int(math.Round(starScore + forkScore + freshnessScore + maturity)), signals
}

func parseRateLimit(header http.Header) RateLimit {
	limit, _ := strconv.Atoi(header.Get("X-RateLimit-Limit"))
	remaining, _ := strconv.Atoi(header.Get("X-RateLimit-Remaining"))
	var resetAt *time.Time
	if reset, err := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64); err == nil && reset > 0 {
		value := time.Unix(reset, 0).UTC()
		resetAt = &value
	}
	return RateLimit{Limit: limit, Remaining: remaining, ResetAt: resetAt}
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
