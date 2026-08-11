package content

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"ai-workbench/internal/identity"
	"ai-workbench/internal/model"
	"ai-workbench/internal/store"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalid        = errors.New("invalid content request")
	ErrNotFound       = errors.New("content not found")
	ErrXNotConfigured = errors.New("x api is not configured")
	ErrUpstream       = errors.New("content upstream unavailable")
)

var handlePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)

type Source struct {
	Code string `json:"code"`
	Name string `json:"name"`
	URL  string `json:"-"`
}

var DefaultSources = []Source{
	{Code: "openai", Name: "OpenAI News", URL: "https://openai.com/news/rss.xml"},
	{Code: "google-ai", Name: "Google AI", URL: "https://blog.google/technology/ai/rss/"},
	{Code: "hugging-face", Name: "Hugging Face", URL: "https://huggingface.co/blog/feed.xml"},
	{Code: "arxiv-ai", Name: "arXiv cs.AI", URL: "https://export.arxiv.org/rss/cs.AI"},
}

var defaultPeople = []struct{ Handle, Name string }{
	{Handle: "btibor91", Name: "Tibor Blaho"},
}

type NewsResult struct {
	Items         []model.NewsArticle `json:"items"`
	Sources       []Source            `json:"sources"`
	LastSuccessAt *time.Time          `json:"lastSuccessAt"`
	LastError     string              `json:"lastError"`
}

type PeopleResult struct {
	People        []model.TrackedPerson `json:"people"`
	XConfigured   bool                  `json:"xConfigured"`
	LastSuccessAt *time.Time            `json:"lastSuccessAt"`
	LastError     string                `json:"lastError"`
}

type Status struct {
	XConfigured       bool       `json:"xConfigured"`
	RefreshHours      int        `json:"refreshHours"`
	NewsLastSuccess   *time.Time `json:"newsLastSuccessAt"`
	NewsLastError     string     `json:"newsLastError"`
	PeopleLastSuccess *time.Time `json:"peopleLastSuccessAt"`
	PeopleLastError   string     `json:"peopleLastError"`
}

type Service struct {
	database      *store.Store
	feeds         *feedClient
	x             *xClient
	sources       []Source
	refreshPeriod time.Duration
	newsMu        sync.Mutex
	peopleMu      sync.Mutex
}

func New(database *store.Store, sources []Source, xBaseURL, xBearerToken string, refreshPeriod time.Duration) *Service {
	if len(sources) == 0 {
		sources = DefaultSources
	}
	if refreshPeriod < time.Hour {
		refreshPeriod = 24 * time.Hour
	}
	return &Service{
		database: database, feeds: newFeedClient(), x: newXClient(xBaseURL, xBearerToken),
		sources: append([]Source(nil), sources...), refreshPeriod: refreshPeriod,
	}
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		s.refreshDue(ctx)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshDue(ctx)
			}
		}
	}()
}

func (s *Service) refreshDue(ctx context.Context) {
	if s.isDue("news", time.Now().UTC()) {
		if _, err := s.RefreshNews(ctx); err != nil {
			log.Printf("AI news refresh failed: %v", err)
		}
	}
	if !s.x.configured() {
		return
	}
	var owners []string
	cutoff := time.Now().UTC().Add(-s.refreshPeriod)
	if err := s.database.DB.Model(&model.TrackedPerson{}).Distinct("owner_id").Where("enabled = ? AND (last_fetched_at IS NULL OR last_fetched_at < ?)", true, cutoff).Pluck("owner_id", &owners).Error; err != nil {
		log.Printf("tracked people due query failed: %v", err)
		return
	}
	for _, owner := range owners {
		if _, err := s.refreshPeople(ctx, owner); err != nil {
			log.Printf("X refresh failed for owner %s: %v", owner, err)
		}
	}
}

func (s *Service) isDue(key string, now time.Time) bool {
	var state model.SyncState
	err := s.database.DB.First(&state, "key = ?", key).Error
	return errors.Is(err, gorm.ErrRecordNotFound) || err == nil && (state.LastSuccessAt == nil || state.LastSuccessAt.Before(now.Add(-s.refreshPeriod)))
}

func (s *Service) RefreshNews(ctx context.Context) (*model.SyncState, error) {
	s.newsMu.Lock()
	defer s.newsMu.Unlock()
	now := time.Now().UTC()
	state := model.SyncState{Key: "news"}
	if err := s.database.DB.FirstOrCreate(&state, model.SyncState{Key: "news"}).Error; err != nil {
		return nil, err
	}
	state.LastAttemptAt = &now
	if err := s.database.DB.Save(&state).Error; err != nil {
		return nil, err
	}
	count := 0
	var failures []string
	for _, source := range s.sources {
		items, err := s.feeds.fetch(ctx, source)
		if err != nil {
			failures = append(failures, source.Name+": "+err.Error())
			continue
		}
		for _, item := range items {
			if err := s.database.DB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "url"}},
				DoUpdates: clause.AssignmentColumns([]string{"source_code", "source_name", "external_id", "title", "summary", "author", "published_at", "fetched_at", "updated_at"}),
			}).Create(&item).Error; err != nil {
				failures = append(failures, source.Name+": save failed")
				continue
			}
			count++
		}
	}
	state.ItemsFetched = count
	state.LastError = strings.Join(failures, "; ")
	if count > 0 {
		state.LastSuccessAt = &now
	}
	if err := s.database.DB.Save(&state).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return &state, fmt.Errorf("%w: %s", ErrUpstream, state.LastError)
	}
	return &state, nil
}

func (s *Service) News(actor identity.Actor, search, source string, favoriteOnly bool) (*NewsResult, error) {
	var items []model.NewsArticle
	search, source = strings.TrimSpace(search), strings.TrimSpace(source)
	query := func() *gorm.DB {
		result := s.database.DB.Model(&model.NewsArticle{})
		if search != "" {
			like := "%" + search + "%"
			result = result.Where("title LIKE ? OR summary LIKE ? OR author LIKE ?", like, like, like)
		}
		if favoriteOnly {
			result = result.Where("id IN (?)", s.database.DB.Model(&model.NewsFavorite{}).Select("article_id").Where("owner_id = ?", actor.ID))
		}
		return result
	}
	if source != "" || favoriteOnly {
		if err := query().Where("source_code = ? OR ? = ''", source, source).Order("published_at DESC").Limit(100).Find(&items).Error; err != nil {
			return nil, err
		}
	} else {
		for _, item := range s.sources {
			var sourceItems []model.NewsArticle
			if err := query().Where("source_code = ?", item.Code).Order("published_at DESC").Limit(20).Find(&sourceItems).Error; err != nil {
				return nil, err
			}
			items = append(items, sourceItems...)
		}
		sort.Slice(items, func(left, right int) bool { return items[left].PublishedAt.After(items[right].PublishedAt) })
	}
	var favoriteIDs []string
	if err := s.database.DB.Model(&model.NewsFavorite{}).Where("owner_id = ?", actor.ID).Pluck("article_id", &favoriteIDs).Error; err != nil {
		return nil, err
	}
	favorites := make(map[string]bool, len(favoriteIDs))
	for _, id := range favoriteIDs {
		favorites[id] = true
	}
	for index := range items {
		items[index].Favorite = favorites[items[index].ID]
	}
	var state model.SyncState
	_ = s.database.DB.First(&state, "key = ?", "news").Error
	return &NewsResult{Items: items, Sources: s.publicSources(), LastSuccessAt: state.LastSuccessAt, LastError: state.LastError}, nil
}

func (s *Service) FavoriteNews(actor identity.Actor, articleID string, favorite bool) error {
	var count int64
	if err := s.database.DB.Model(&model.NewsArticle{}).Where("id = ?", articleID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	query := s.database.DB.Where("owner_id = ? AND article_id = ?", actor.ID, articleID)
	if !favorite {
		return query.Delete(&model.NewsFavorite{}).Error
	}
	item := model.NewsFavorite{ID: newID("nfav"), OwnerID: actor.ID, ArticleID: articleID}
	return s.database.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error
}

func (s *Service) People(actor identity.Actor) (*PeopleResult, error) {
	if err := s.ensureDefaults(actor.ID); err != nil {
		return nil, err
	}
	var people []model.TrackedPerson
	if err := s.database.DB.Where("owner_id = ?", actor.ID).Order("created_at ASC").Find(&people).Error; err != nil {
		return nil, err
	}
	var state model.SyncState
	_ = s.database.DB.First(&state, "key = ?", peopleStateKey(actor.ID)).Error
	return &PeopleResult{People: people, XConfigured: s.x.configured(), LastSuccessAt: state.LastSuccessAt, LastError: state.LastError}, nil
}

func (s *Service) AddPerson(actor identity.Actor, handle, displayName string) (*model.TrackedPerson, error) {
	handle = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@"))
	if !handlePattern.MatchString(handle) {
		return nil, ErrInvalid
	}
	if displayName = strings.TrimSpace(displayName); displayName == "" {
		displayName = "@" + handle
	}
	if len([]rune(displayName)) > 160 {
		return nil, ErrInvalid
	}
	person := &model.TrackedPerson{ID: newID("person"), OwnerID: actor.ID, Platform: "x", Handle: handle, DisplayName: displayName, Enabled: true}
	if err := s.database.DB.Create(person).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrInvalid
		}
		return nil, err
	}
	return person, nil
}

func (s *Service) DeletePerson(actor identity.Actor, personID string) error {
	return s.database.DB.Transaction(func(tx *gorm.DB) error {
		var person model.TrackedPerson
		if err := tx.Where("id = ? AND owner_id = ?", personID, actor.ID).First(&person).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		var postIDs []string
		if err := tx.Model(&model.SocialPost{}).Where("person_id = ? AND owner_id = ?", personID, actor.ID).Pluck("id", &postIDs).Error; err != nil {
			return err
		}
		if len(postIDs) > 0 {
			if err := tx.Where("owner_id = ? AND post_id IN ?", actor.ID, postIDs).Delete(&model.SocialPostFavorite{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("person_id = ? AND owner_id = ?", personID, actor.ID).Delete(&model.SocialPost{}).Error; err != nil {
			return err
		}
		return tx.Delete(&person).Error
	})
}

func (s *Service) RefreshPeople(ctx context.Context, actor identity.Actor) (*model.SyncState, error) {
	if err := s.ensureDefaults(actor.ID); err != nil {
		return nil, err
	}
	return s.refreshPeople(ctx, actor.ID)
}

func (s *Service) refreshPeople(ctx context.Context, ownerID string) (*model.SyncState, error) {
	if !s.x.configured() {
		return nil, ErrXNotConfigured
	}
	s.peopleMu.Lock()
	defer s.peopleMu.Unlock()
	now := time.Now().UTC()
	state := model.SyncState{Key: peopleStateKey(ownerID)}
	if err := s.database.DB.FirstOrCreate(&state, model.SyncState{Key: peopleStateKey(ownerID)}).Error; err != nil {
		return nil, err
	}
	state.LastAttemptAt = &now
	if err := s.database.DB.Save(&state).Error; err != nil {
		return nil, err
	}
	var people []model.TrackedPerson
	if err := s.database.DB.Where("owner_id = ? AND enabled = ?", ownerID, true).Find(&people).Error; err != nil {
		return nil, err
	}
	count := 0
	var failures []string
	for index := range people {
		posts, err := s.x.fetchPosts(ctx, &people[index])
		if err != nil {
			people[index].LastError = err.Error()
			failures = append(failures, "@"+people[index].Handle+": "+err.Error())
			_ = s.database.DB.Save(&people[index]).Error
			continue
		}
		people[index].LastFetchedAt = &now
		people[index].LastError = ""
		if err := s.database.DB.Save(&people[index]).Error; err != nil {
			return nil, err
		}
		for _, post := range posts {
			post.OwnerID, post.PersonID, post.Handle, post.DisplayName = ownerID, people[index].ID, people[index].Handle, people[index].DisplayName
			if err := s.database.DB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "owner_id"}, {Name: "external_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"person_id", "handle", "display_name", "content", "url", "published_at", "like_count", "repost_count", "reply_count", "updated_at"}),
			}).Create(&post).Error; err != nil {
				return nil, err
			}
			count++
		}
	}
	state.ItemsFetched, state.LastError = count, strings.Join(failures, "; ")
	if len(people) > 0 && len(failures) < len(people) {
		state.LastSuccessAt = &now
	}
	if err := s.database.DB.Save(&state).Error; err != nil {
		return nil, err
	}
	if len(people) > 0 && len(failures) == len(people) {
		return &state, fmt.Errorf("%w: %s", ErrUpstream, state.LastError)
	}
	return &state, nil
}

func (s *Service) Posts(actor identity.Actor, personID, search string, favoriteOnly bool) ([]model.SocialPost, error) {
	query := s.database.DB.Model(&model.SocialPost{}).Where("owner_id = ?", actor.ID)
	if personID = strings.TrimSpace(personID); personID != "" {
		query = query.Where("person_id = ?", personID)
	}
	if search = strings.TrimSpace(search); search != "" {
		query = query.Where("content LIKE ?", "%"+search+"%")
	}
	if favoriteOnly {
		query = query.Where("id IN (?)", s.database.DB.Model(&model.SocialPostFavorite{}).Select("post_id").Where("owner_id = ?", actor.ID))
	}
	var posts []model.SocialPost
	if err := query.Order("published_at DESC").Limit(300).Find(&posts).Error; err != nil {
		return nil, err
	}
	var favoriteIDs []string
	if err := s.database.DB.Model(&model.SocialPostFavorite{}).Where("owner_id = ?", actor.ID).Pluck("post_id", &favoriteIDs).Error; err != nil {
		return nil, err
	}
	favorites := make(map[string]bool, len(favoriteIDs))
	for _, id := range favoriteIDs {
		favorites[id] = true
	}
	for index := range posts {
		posts[index].Favorite = favorites[posts[index].ID]
	}
	return posts, nil
}

func (s *Service) FavoritePost(actor identity.Actor, postID string, favorite bool) error {
	var count int64
	if err := s.database.DB.Model(&model.SocialPost{}).Where("id = ? AND owner_id = ?", postID, actor.ID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	query := s.database.DB.Where("owner_id = ? AND post_id = ?", actor.ID, postID)
	if !favorite {
		return query.Delete(&model.SocialPostFavorite{}).Error
	}
	item := model.SocialPostFavorite{ID: newID("pfav"), OwnerID: actor.ID, PostID: postID}
	return s.database.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error
}

func (s *Service) Overview(actor identity.Actor) Status {
	result := Status{XConfigured: s.x.configured(), RefreshHours: int(s.refreshPeriod / time.Hour)}
	var news, people model.SyncState
	if s.database.DB.First(&news, "key = ?", "news").Error == nil {
		result.NewsLastSuccess, result.NewsLastError = news.LastSuccessAt, news.LastError
	}
	if s.database.DB.First(&people, "key = ?", peopleStateKey(actor.ID)).Error == nil {
		result.PeopleLastSuccess, result.PeopleLastError = people.LastSuccessAt, people.LastError
	}
	return result
}

func (s *Service) ensureDefaults(ownerID string) error {
	for _, item := range defaultPeople {
		person := model.TrackedPerson{ID: newID("person"), OwnerID: ownerID, Platform: "x", Handle: item.Handle, DisplayName: item.Name, Enabled: true}
		if err := s.database.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&person).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) publicSources() []Source {
	result := make([]Source, len(s.sources))
	for index, source := range s.sources {
		result[index] = Source{Code: source.Code, Name: source.Name}
	}
	return result
}

func peopleStateKey(ownerID string) string {
	digest := sha256.Sum256([]byte(ownerID))
	return "people:" + hex.EncodeToString(digest[:8])
}

func newID(prefix string) string {
	value := make([]byte, 12)
	_, _ = rand.Read(value)
	return prefix + "_" + hex.EncodeToString(value)
}
