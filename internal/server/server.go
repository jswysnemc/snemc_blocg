package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	blogembed "github.com/snemc/snemc-blog"

	"github.com/snemc/snemc-blog/internal/ai"
	"github.com/snemc/snemc-blog/internal/auth"
	"github.com/snemc/snemc-blog/internal/cache"
	"github.com/snemc/snemc-blog/internal/config"
	"github.com/snemc/snemc-blog/internal/data"
	"github.com/snemc/snemc-blog/internal/email"
	"github.com/snemc/snemc-blog/internal/embedding"
	"github.com/snemc/snemc-blog/internal/render"
	"github.com/snemc/snemc-blog/internal/store"
)

type App struct {
	cfg                    config.Config
	store                  *store.Store
	mailer                 *email.Mailer
	reviewer               *ai.OpenAICompatibleReviewer
	embedder               *embedding.OpenAICompatibleEmbedder
	templates              *template.Template
	postCache              *cache.TTLCache[[]store.PostSummary]
	searchCache            *cache.TTLCache[searchOutcome]
	taxonomyCache          *cache.TTLCache[store.TaxonomyBundle]
	settingsCache          *cache.TTLCache[store.AppSettings]
	semanticBackfillMu     sync.Mutex
	semanticBackfillActive bool
	publicAssetsFS         fs.FS
	frontendDistFS         fs.FS
	publicJSPath           string
	publicCSSPath          string
}

type searchOutcome struct {
	RequestedMode string
	ExecutedMode  string
	Notice        string
	Results       []store.SearchResult
}

type PageMeta struct {
	SiteName      string
	SiteURL       string
	Title         string
	Description   string
	Canonical     string
	CurrentPath   string
	SearchQuery   string
	PublicJSPath  string
	PublicCSSPath string
}

type HomePage struct {
	PageMeta
	Featured        *store.PostSummary
	Posts           []store.PostSummary
	Categories      []store.Category
	Tags            []store.Tag
	Recommendations []store.Recommendation
}

type ListPage struct {
	PageMeta
	Heading    string
	Subheading string
	Posts      []store.PostSummary
	Categories []store.Category
	Tags       []store.Tag
}

type ArchivePage struct {
	PageMeta
	Posts      []store.PostSummary
	TotalPosts int
	PageSize   int
	HasMore    bool
}

type SearchPage struct {
	PageMeta
	Query         string
	RequestedMode string
	ExecutedMode  string
	Notice        string
	Results       []store.SearchResult
	Categories    []store.Category
	Tags          []store.Tag
	ResultCount   int
}

type AboutFriend struct {
	Name        string
	Description string
	URL         string
	Accent      string
}

type AboutStat struct {
	Label string
	Value string
}

type AboutProfile struct {
	Name       string
	Tagline    string
	AvatarURL  string
	AvatarText string
	Email      string
	GitHubURL  string
	Bio        string
	Stats      []AboutStat
	Friends    []AboutFriend
}

type AboutPage struct {
	PageMeta
	Profile AboutProfile
}

type PostPage struct {
	PageMeta
	Post            store.PostDetail
	Recommendations []store.Recommendation
}

func Run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg := config.Load(root)
	if err := os.MkdirAll(cfg.MediaDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.UploadsDir, 0o755); err != nil {
		return err
	}

	db, err := data.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}

	renderer := render.NewRenderer()
	reviewer := ai.NewReviewer(ai.RuntimeConfig{
		BaseURL:      cfg.LLMBaseURL,
		APIKey:       cfg.LLMAPIKey,
		Model:        cfg.LLMModel,
		SystemPrompt: cfg.LLMSystemPrompt,
	})
	embedder := embedding.New(runtimeEmbeddingConfig(store.AppSettings{
		EmbeddingBaseURL:      cfg.EmbeddingBaseURL,
		EmbeddingAPIKey:       cfg.EmbeddingAPIKey,
		EmbeddingModel:        cfg.EmbeddingModel,
		EmbeddingDimensions:   cfg.EmbeddingDimensions,
		EmbeddingTimeoutMS:    cfg.EmbeddingTimeoutMS,
		SemanticSearchEnabled: cfg.SemanticSearchEnabled,
	}))
	st := store.New(db, renderer, reviewer)
	if err := st.Bootstrap(context.Background(), cfg); err != nil {
		return err
	}
	settings, err := st.GetAppSettings(context.Background())
	if err != nil {
		return err
	}
	settings = mergeRuntimeOverrides(settings, cfg.RuntimeOverrides)
	reviewer.Update(runtimeAIConfig(settings))
	embedder.Update(runtimeEmbeddingConfig(settings))
	if settings.SemanticSearchEnabled && settings.EmbeddingDimensions > 0 {
		if _, err := st.EnsureSemanticVectorTable(context.Background(), settings.EmbeddingDimensions); err != nil && err != store.ErrInvalidInput {
			log.Printf("semantic vector table setup failed: %v", err)
		}
	}

	funcs := template.FuncMap{
		"safeHTML": func(input string) template.HTML {
			return template.HTML(input)
		},
	}
	templates, err := template.New("").Funcs(funcs).ParseFS(blogembed.TemplatesFS, "*.gohtml")
	if err != nil {
		return err
	}

	app := &App{
		cfg:            cfg,
		store:          st,
		mailer:         email.New(runtimeMailConfig(settings)),
		reviewer:       reviewer,
		embedder:       embedder,
		templates:      templates,
		postCache:      cache.New[[]store.PostSummary](),
		searchCache:    cache.New[searchOutcome](),
		taxonomyCache:  cache.New[store.TaxonomyBundle](),
		settingsCache:  cache.New[store.AppSettings](),
		publicAssetsFS: blogembed.PublicAssetsFS,
		frontendDistFS: blogembed.FrontendDistFS,
	}
	app.loadEnhancementAssets()
	app.scheduleSemanticBackfill()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("blog server listening on %s", cfg.Addr)
	return srv.ListenAndServe()
}

func (a *App) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	r.Handle("/assets/*", http.StripPrefix("/assets/", http.FileServer(http.FS(a.publicAssetsFS))))
	r.Handle("/media/*", immutableFileServer("/media/", a.cfg.MediaDir))
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir(a.cfg.UploadsDir))))
	r.Handle("/front/*", http.StripPrefix("/front/", http.FileServer(http.FS(a.frontendDistFS))))

	r.Get("/h/{route_id}", a.handleStaticSite)
	r.Get("/h/{route_id}/*", a.handleStaticSite)

	r.Get("/", a.handleHome)
	r.Get("/about", a.handleAboutPage)
	r.Get("/archive", a.handleArchivePage)
	r.Get("/search", a.handleSearchPage)
	r.Get("/category/{slug}", a.handleCategoryPage)
	r.Get("/tag/{slug}", a.handleTagPage)
	r.Get("/posts/{slug}", a.handlePostPage)
	r.Get("/review/comment", a.handleCommentReviewAction)
	r.Get("/robots.txt", a.handleRobots)
	r.Get("/sitemap.xml", a.handleSitemap)

	r.Post("/api/visitor/init", a.handleVisitorInit)
	r.Post("/api/track/pageview", a.handleTrackPageView)
	r.Get("/api/search", a.handleSearchAPI)
	r.Get("/api/archive/posts", a.handleArchivePosts)
	r.Get("/api/posts/{slug}/comments", a.handleCommentsList)
	r.Post("/api/posts/{slug}/comments", a.handleCreateComment)
	r.Post("/api/posts/{slug}/like", a.handlePostLike)
	r.Post("/api/comments/{id}/like", a.handleCommentLike)
	r.Get("/api/posts/{slug}/share-poster.svg", a.handleSharePoster)

	r.Group(func(admin chi.Router) {
		admin.Post("/api/admin/login", a.handleAdminLogin)
		admin.With(a.requireAdmin).Get("/api/admin/me", a.handleAdminMe)
		admin.With(a.requireAdmin).Get("/api/admin/dashboard", a.handleAdminDashboard)
		admin.With(a.requireAdmin).Get("/api/admin/posts", a.handleAdminPosts)
		admin.With(a.requireAdmin).Post("/api/admin/posts", a.handleAdminSavePost)
		admin.With(a.requireAdmin).Get("/api/admin/posts/{id}", a.handleAdminGetPost)
		admin.With(a.requireAdmin).Put("/api/admin/posts/{id}", a.handleAdminSavePost)
		admin.With(a.requireAdmin).Delete("/api/admin/posts/{id}", a.handleAdminDeletePost)
		admin.With(a.requireAdmin).Get("/api/admin/taxonomies", a.handleAdminTaxonomies)
		admin.With(a.requireAdmin).Post("/api/admin/categories", a.handleAdminSaveCategory)
		admin.With(a.requireAdmin).Delete("/api/admin/categories/{id}", a.handleAdminDeleteCategory)
		admin.With(a.requireAdmin).Post("/api/admin/tags", a.handleAdminSaveTag)
		admin.With(a.requireAdmin).Delete("/api/admin/tags/{id}", a.handleAdminDeleteTag)
		admin.With(a.requireAdmin).Get("/api/admin/comments", a.handleAdminComments)
		admin.With(a.requireAdmin).Post("/api/admin/comments/{id}/review", a.handleAdminReviewComment)
		admin.With(a.requireAdmin).Post("/api/admin/comments/{id}/ai-review", a.handleAdminRerunAIReview)
		admin.With(a.requireAdmin).Delete("/api/admin/comments/{id}", a.handleAdminDeleteComment)
		admin.With(a.requireAdmin).Get("/api/admin/settings", a.handleAdminGetSettings)
		admin.With(a.requireAdmin).Put("/api/admin/settings", a.handleAdminSaveSettings)
		admin.With(a.requireAdmin).Get("/api/admin/agent-keys", a.handleAdminAgentKeys)
		admin.With(a.requireAdmin).Post("/api/admin/agent-keys", a.handleAdminCreateAgentKey)
		admin.With(a.requireAdmin).Delete("/api/admin/agent-keys/{id}", a.handleAdminRevokeAgentKey)
		admin.With(a.requireAdmin).Get("/api/admin/media/assets", a.handleAdminMediaAssets)
		admin.With(a.requireAdmin).Delete("/api/admin/media/assets", a.handleAdminMediaAssetDelete)
		admin.With(a.requireAdmin).Get("/api/admin/static-sites", a.handleAdminStaticSites)
		admin.With(a.requireAdmin).Post("/api/admin/static-sites", a.handleAdminCreateStaticSite)
		admin.With(a.requireAdmin).Post("/api/admin/static-sites/{id}/upload", a.handleAdminUploadStaticSite)
		admin.With(a.requireAdmin).Get("/api/admin/static-sites/{id}/download", a.handleAdminDownloadStaticSite)
		admin.With(a.requireAdmin).Delete("/api/admin/static-sites/{id}", a.handleAdminDeleteStaticSite)
		admin.With(a.requireAdmin).Post("/api/admin/media/images", a.handleAdminMediaImageUpload)
		admin.With(a.requireAdmin).Post("/api/admin/media/import", a.handleAdminMediaImport)
		admin.With(a.requireAdmin).Post("/api/admin/upload/image", a.handleAdminImageUpload)
	})

	r.Get("/api/agent/skills", a.handleAgentSkills)
	r.Get("/api/agent/skills.md", a.handleAgentSkills)
	r.Group(func(agent chi.Router) {
		agent.With(a.requireAgentKey).Get("/api/agent/posts", a.handleAgentPosts)
		agent.With(a.requireAgentKey).Post("/api/agent/posts", a.handleAgentCreatePost)
		agent.With(a.requireAgentKey).Get("/api/agent/posts/{id}", a.handleAgentGetPost)
		agent.With(a.requireAgentKey).Put("/api/agent/posts/{id}", a.handleAgentUpdatePost)
		agent.With(a.requireAgentKey).Patch("/api/agent/posts/{id}", a.handleAgentPatchPost)
		agent.With(a.requireAgentKey).Get("/api/agent/stats", a.handleAgentStats)
		agent.With(a.requireAgentKey).Get("/api/agent/taxonomies", a.handleAgentTaxonomies)
		agent.With(a.requireAgentKey).Post("/api/agent/categories", a.handleAgentCreateCategory)
		agent.With(a.requireAgentKey).Put("/api/agent/categories/{id}", a.handleAgentUpdateCategory)
		agent.With(a.requireAgentKey).Delete("/api/agent/categories/{id}", a.handleAgentDeleteCategory)
		agent.With(a.requireAgentKey).Post("/api/agent/tags", a.handleAgentCreateTag)
		agent.With(a.requireAgentKey).Put("/api/agent/tags/{id}", a.handleAgentUpdateTag)
		agent.With(a.requireAgentKey).Delete("/api/agent/tags/{id}", a.handleAgentDeleteTag)
	})

	r.Get("/admin", a.handleAdminApp)
	r.Get("/admin/*", a.handleAdminApp)

	return r
}

func (a *App) loadEnhancementAssets() {
	if _, err := fs.Stat(a.frontendDistFS, "assets/public.js"); err == nil {
		a.publicJSPath = "/front/assets/public.js"
	}
	if _, err := fs.Stat(a.frontendDistFS, "assets/public-enhance.css"); err == nil {
		a.publicCSSPath = "/front/assets/public-enhance.css"
	}
}

func (a *App) taxonomyBundle(ctx context.Context) (store.TaxonomyBundle, error) {
	if bundle, ok := a.taxonomyCache.Get("all"); ok {
		return bundle, nil
	}
	bundle, err := a.store.GetTaxonomies(ctx)
	if err != nil {
		return store.TaxonomyBundle{}, err
	}
	a.taxonomyCache.Set("all", bundle, 10*time.Minute)
	return bundle, nil
}

func (a *App) appSettings(ctx context.Context) (store.AppSettings, error) {
	if settings, ok := a.settingsCache.Get("current"); ok {
		return settings, nil
	}
	settings, err := a.store.GetAppSettings(ctx)
	if err != nil {
		return store.AppSettings{}, err
	}
	settings = mergeRuntimeOverrides(settings, a.cfg.RuntimeOverrides)
	a.settingsCache.Set("current", settings, time.Minute)
	return settings, nil
}

func runtimeMailConfig(settings store.AppSettings) email.RuntimeConfig {
	return email.RuntimeConfig{
		SMTPHost:     settings.SMTPHost,
		SMTPPort:     settings.SMTPPort,
		SMTPUsername: settings.SMTPUsername,
		SMTPPassword: settings.SMTPPassword,
		SMTPFrom:     settings.SMTPFrom,
	}
}

func runtimeAIConfig(settings store.AppSettings) ai.RuntimeConfig {
	return ai.RuntimeConfig{
		BaseURL:      settings.LLMBaseURL,
		APIKey:       settings.LLMAPIKey,
		Model:        settings.LLMModel,
		SystemPrompt: settings.LLMSystemPrompt,
	}
}

func runtimeEmbeddingConfig(settings store.AppSettings) embedding.RuntimeConfig {
	timeout := time.Duration(settings.EmbeddingTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return embedding.RuntimeConfig{
		Enabled:    settings.SemanticSearchEnabled,
		BaseURL:    settings.EmbeddingBaseURL,
		APIKey:     settings.EmbeddingAPIKey,
		Model:      settings.EmbeddingModel,
		Dimensions: settings.EmbeddingDimensions,
		Timeout:    timeout,
	}
}

func mergeRuntimeOverrides(settings store.AppSettings, overrides config.RuntimeSettingsOverrides) store.AppSettings {
	if overrides.SMTPHost != nil {
		settings.SMTPHost = *overrides.SMTPHost
	}
	if overrides.SMTPPort != nil {
		settings.SMTPPort = *overrides.SMTPPort
	}
	if overrides.SMTPUsername != nil {
		settings.SMTPUsername = *overrides.SMTPUsername
	}
	if overrides.SMTPPassword != nil {
		settings.SMTPPassword = *overrides.SMTPPassword
	}
	if overrides.SMTPFrom != nil {
		settings.SMTPFrom = *overrides.SMTPFrom
	}
	if overrides.AdminNotifyEmail != nil {
		settings.AdminNotifyEmail = *overrides.AdminNotifyEmail
	}
	if overrides.LLMBaseURL != nil {
		settings.LLMBaseURL = *overrides.LLMBaseURL
	}
	if overrides.LLMAPIKey != nil {
		settings.LLMAPIKey = *overrides.LLMAPIKey
	}
	if overrides.LLMModel != nil {
		settings.LLMModel = *overrides.LLMModel
	}
	if overrides.LLMSystemPrompt != nil {
		settings.LLMSystemPrompt = *overrides.LLMSystemPrompt
	}
	if overrides.EmbeddingBaseURL != nil {
		settings.EmbeddingBaseURL = *overrides.EmbeddingBaseURL
	}
	if overrides.EmbeddingAPIKey != nil {
		settings.EmbeddingAPIKey = *overrides.EmbeddingAPIKey
	}
	if overrides.EmbeddingModel != nil {
		settings.EmbeddingModel = *overrides.EmbeddingModel
	}
	if overrides.EmbeddingDimensions != nil {
		settings.EmbeddingDimensions = *overrides.EmbeddingDimensions
	}
	if overrides.EmbeddingTimeoutMS != nil {
		settings.EmbeddingTimeoutMS = *overrides.EmbeddingTimeoutMS
	}
	if overrides.SemanticSearchEnabled != nil {
		settings.SemanticSearchEnabled = *overrides.SemanticSearchEnabled
	}
	return settings
}

func (a *App) renderTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *App) pageMeta(r *http.Request, title string, description string) PageMeta {
	return PageMeta{
		SiteName:      a.cfg.SiteName,
		SiteURL:       a.cfg.SiteURL,
		Title:         title,
		Description:   description,
		Canonical:     strings.TrimRight(a.cfg.SiteURL, "/") + r.URL.Path,
		CurrentPath:   r.URL.Path,
		SearchQuery:   strings.TrimSpace(r.URL.Query().Get("q")),
		PublicJSPath:  a.publicJSPath,
		PublicCSSPath: a.publicCSSPath,
	}
}

func (a *App) respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (a *App) decodeJSON(r *http.Request, dest any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dest)
}

func (a *App) visitorID(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie("visitor_id"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	value := randomID()
	http.SetCookie(w, &http.Cookie{
		Name:     "visitor_id",
		Value:    value,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	return value
}

func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf)
}

func (a *App) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(authz, "Bearer ") {
			a.respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
			return
		}
		token := strings.TrimPrefix(authz, "Bearer ")
		claims, err := auth.Parse(a.cfg.JWTSecret, token)
		if err != nil {
			a.respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		ctx := context.WithValue(r.Context(), adminClaimsKey{}, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type adminClaimsKey struct{}

func adminClaims(ctx context.Context) (auth.Claims, error) {
	claims, ok := ctx.Value(adminClaimsKey{}).(auth.Claims)
	if !ok {
		return auth.Claims{}, errors.New("missing admin claims")
	}
	return claims, nil
}

func (a *App) requireAgentKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(authz, "Bearer ") {
			a.respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
			return
		}
		token := strings.TrimPrefix(authz, "Bearer ")
		key, err := a.store.AuthenticateAgentAPIKey(r.Context(), token)
		if err == store.ErrInvalidAgentKey {
			a.respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid agent key"})
			return
		}
		if err != nil {
			a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent auth failed"})
			return
		}
		ctx := context.WithValue(r.Context(), agentKeyContextKey{}, key)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type agentKeyContextKey struct{}

func agentKeyFromContext(ctx context.Context) (store.AgentAPIKey, error) {
	key, ok := ctx.Value(agentKeyContextKey{}).(store.AgentAPIKey)
	if !ok {
		return store.AgentAPIKey{}, errors.New("missing agent key")
	}
	return key, nil
}
