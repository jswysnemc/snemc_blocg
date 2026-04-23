package server

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/snemc/snemc-blog/internal/auth"
	"github.com/snemc/snemc-blog/internal/store"
)

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	visitorID := a.visitorID(w, r)
	posts, err := a.cachedPosts(r.Context(), "home", func(context.Context) ([]store.PostSummary, error) {
		return a.store.ListPublishedPosts(r.Context(), "", "", 8)
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	bundle, err := a.taxonomyBundle(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recommendations, _ := a.store.GetRecommendations(r.Context(), visitorID, 0, a.cfg.RecommendationN)

	page := HomePage{
		PageMeta:        a.pageMeta(r, a.cfg.SiteName+" | Editorial Engineering Blog", "Go + Vue3 + SQLite 技术博客，强调预编译渲染、紧凑界面和高性能阅读体验。"),
		Posts:           posts,
		Categories:      bundle.Categories,
		Tags:            bundle.Tags,
		Recommendations: recommendations,
	}
	if len(posts) > 0 {
		page.Featured = &posts[0]
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	a.renderTemplate(w, "home", page)
}

func (a *App) handleAboutPage(w http.ResponseWriter, r *http.Request) {
	settings, err := a.appSettings(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	page := AboutPage{
		PageMeta: a.pageMeta(
			r,
			"关于 | "+a.cfg.SiteName,
			"了解站点作者、开源偏好与常用协作链接。",
		),
		Profile: aboutProfileFromSettings(settings, a.cfg.SiteName),
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	a.renderTemplate(w, "about", page)
}

func (a *App) handleArchivePage(w http.ResponseWriter, r *http.Request) {
	const pageSize = 8

	posts, err := a.cachedPosts(r.Context(), "archive:0:8", func(context.Context) ([]store.PostSummary, error) {
		return a.store.ListPublishedPostsPage(r.Context(), "", "", pageSize, 0)
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	total, err := a.store.CountPublishedPosts(r.Context(), "", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	page := ArchivePage{
		PageMeta:   a.pageMeta(r, "归档 | "+a.cfg.SiteName, "按时间顺序浏览全部已发布文章。"),
		Posts:      posts,
		TotalPosts: total,
		PageSize:   pageSize,
		HasMore:    len(posts) < total,
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	a.renderTemplate(w, "archive", page)
}

func (a *App) handleCategoryPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	posts, err := a.cachedPosts(r.Context(), "category:"+slug, func(context.Context) ([]store.PostSummary, error) {
		return a.store.ListPublishedPosts(r.Context(), slug, "", 24)
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	bundle, err := a.taxonomyBundle(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := ListPage{
		PageMeta:   a.pageMeta(r, "分类内容 | "+a.cfg.SiteName, "按分类浏览文章内容。"),
		Heading:    "分类 / " + slug,
		Subheading: "按主题聚合的文章列表。",
		Posts:      posts,
		Categories: bundle.Categories,
		Tags:       bundle.Tags,
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	a.renderTemplate(w, "list", page)
}

func (a *App) handleTagPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	posts, err := a.cachedPosts(r.Context(), "tag:"+slug, func(context.Context) ([]store.PostSummary, error) {
		return a.store.ListPublishedPosts(r.Context(), "", slug, 24)
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	bundle, err := a.taxonomyBundle(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := ListPage{
		PageMeta:   a.pageMeta(r, "标签内容 | "+a.cfg.SiteName, "按标签浏览文章内容。"),
		Heading:    "标签 / " + slug,
		Subheading: "围绕特定技术栈和主题的文章集合。",
		Posts:      posts,
		Categories: bundle.Categories,
		Tags:       bundle.Tags,
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	a.renderTemplate(w, "list", page)
}

func (a *App) handleSearchPage(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	requestedMode := normalizeSearchMode(r.URL.Query().Get("mode"))
	outcome, err := a.executeSearch(r.Context(), query, 12, requestedMode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	visitorID := a.visitorID(w, r)
	_ = a.store.RecordSearch(r.Context(), visitorID, query)

	bundle, err := a.taxonomyBundle(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := SearchPage{
		PageMeta:      a.pageMeta(r, "搜索 | "+a.cfg.SiteName, "关键词搜索与语义搜索双模式检索。"),
		Query:         query,
		RequestedMode: outcome.RequestedMode,
		ExecutedMode:  outcome.ExecutedMode,
		Notice:        outcome.Notice,
		Results:       outcome.Results,
		ResultCount:   len(outcome.Results),
		Categories:    bundle.Categories,
		Tags:          bundle.Tags,
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	a.renderTemplate(w, "search", page)
}

func (a *App) handlePostPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	visitorID := a.visitorID(w, r)
	post, err := a.store.GetPublishedPost(r.Context(), slug, visitorID)
	if err == store.ErrNotFound {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.applyPostCacheHeaders(w, r, post)
	if w.Header().Get("X-Cache-ShortCircuit") == "304" {
		w.Header().Del("X-Cache-ShortCircuit")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	recommendations, _ := a.store.GetRecommendations(r.Context(), visitorID, post.ID, a.cfg.RecommendationN)
	page := PostPage{
		PageMeta:        a.pageMeta(r, post.Title+" | "+a.cfg.SiteName, post.Excerpt),
		Post:            post,
		Recommendations: recommendations,
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	a.renderTemplate(w, "post", page)
}

func (a *App) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml\n", strings.TrimRight(a.cfg.SiteURL, "/"))
}

func (a *App) handleSitemap(w http.ResponseWriter, r *http.Request) {
	posts, err := a.store.ListPublishedPosts(r.Context(), "", "", 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	buf.WriteString(fmt.Sprintf(`<url><loc>%s/</loc></url>`, strings.TrimRight(a.cfg.SiteURL, "/")))
	buf.WriteString(fmt.Sprintf(`<url><loc>%s/about</loc></url>`, strings.TrimRight(a.cfg.SiteURL, "/")))
	for _, post := range posts {
		buf.WriteString(fmt.Sprintf(`<url><loc>%s/posts/%s</loc></url>`, strings.TrimRight(a.cfg.SiteURL, "/"), template.HTMLEscapeString(post.Slug)))
	}
	buf.WriteString(`</urlset>`)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (a *App) handleVisitorInit(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		VisitorID   string `json:"visitor_id"`
		Fingerprint string `json:"fingerprint"`
		Language    string `json:"language"`
	}
	var req payload
	_ = a.decodeJSON(r, &req)

	visitorID := a.visitorID(w, r)
	if strings.TrimSpace(req.VisitorID) != "" {
		visitorID = req.VisitorID
		http.SetCookie(w, &http.Cookie{
			Name:     "visitor_id",
			Value:    visitorID,
			Path:     "/",
			MaxAge:   60 * 60 * 24 * 365,
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
		})
	}

	profile, err := a.store.UpsertVisitor(r.Context(), store.VisitorInput{
		VisitorID:   visitorID,
		Fingerprint: req.Fingerprint,
		UserAgent:   r.UserAgent(),
		Language:    req.Language,
	})
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "visitor init failed"})
		return
	}
	a.respondJSON(w, http.StatusOK, profile)
}

func (a *App) handleTrackPageView(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		Path     string `json:"path"`
		Slug     string `json:"slug"`
		Referrer string `json:"referrer"`
	}
	var req payload
	_ = a.decodeJSON(r, &req)
	visitorID := a.visitorID(w, r)
	var postID *int64
	if req.Slug != "" {
		if post, err := a.store.GetPublishedPost(r.Context(), req.Slug, visitorID); err == nil {
			postID = &post.ID
		}
	}
	_ = a.store.RecordPageView(r.Context(), visitorID, postID, req.Path, req.Referrer)
	a.respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleSearchAPI(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	requestedMode := normalizeSearchMode(r.URL.Query().Get("mode"))
	outcome, err := a.executeSearch(r.Context(), query, 8, requestedMode)
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	visitorID := a.visitorID(w, r)
	_ = a.store.RecordSearch(r.Context(), visitorID, query)
	a.respondJSON(w, http.StatusOK, map[string]any{
		"requested_mode": outcome.RequestedMode,
		"executed_mode":  outcome.ExecutedMode,
		"notice":         outcome.Notice,
		"results":        outcome.Results,
	})
}

func (a *App) handleArchivePosts(w http.ResponseWriter, r *http.Request) {
	limit := clampArchiveLimit(parseQueryInt(r, "limit", 8))
	offset := parseQueryInt(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}

	posts, err := a.store.ListPublishedPostsPage(r.Context(), "", "", limit, offset)
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	total, err := a.store.CountPublishedPosts(r.Context(), "", "")
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	nextOffset := offset + len(posts)
	a.respondJSON(w, http.StatusOK, map[string]any{
		"posts":       posts,
		"offset":      offset,
		"next_offset": nextOffset,
		"limit":       limit,
		"total":       total,
		"has_more":    nextOffset < total,
	})
}

func (a *App) handleCommentsList(w http.ResponseWriter, r *http.Request) {
	visitorID := a.visitorID(w, r)
	post, err := a.store.GetPublishedPost(r.Context(), chi.URLParam(r, "slug"), visitorID)
	if err != nil {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "post not found"})
		return
	}
	comments, err := a.store.ListComments(r.Context(), post.ID, visitorID)
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

func (a *App) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		ParentID   *int64 `json:"parent_id"`
		AuthorName string `json:"author_name"`
		Email      string `json:"email"`
		Content    string `json:"content"`
	}
	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	visitorID := a.visitorID(w, r)
	post, err := a.store.GetPublishedPost(r.Context(), chi.URLParam(r, "slug"), visitorID)
	if err != nil {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "post not found"})
		return
	}
	settings, err := a.appSettings(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to load settings"})
		return
	}
	comment, err := a.store.CreateComment(r.Context(), store.CommentInput{
		PostID:     post.ID,
		ParentID:   req.ParentID,
		VisitorID:  visitorID,
		AuthorName: req.AuthorName,
		Email:      req.Email,
		Content:    req.Content,
		IP:         r.RemoteAddr,
		PostTitle:  post.Title,
	}, a.cfg.CommentCooldown, settings.CommentReviewMode)
	if err == store.ErrRateLimited {
		a.respondJSON(w, http.StatusTooManyRequests, map[string]string{"error": "comment rate limited"})
		return
	}
	if err == store.ErrInvalidInput {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "comment content required"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "unable to create comment"})
		return
	}
	a.queueAdminCommentEmail(post, comment, settings.AdminNotifyEmail)
	if comment.Status == "approved" {
		a.queueMentionNotifications(comment)
	}
	message := "评论已提交，等待审核后显示。"
	if comment.Status == "approved" {
		message = "评论已发布。"
	}
	a.respondJSON(w, http.StatusCreated, map[string]any{
		"comment": comment,
		"message": message,
	})
}

func (a *App) handlePostLike(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.LikePost(r.Context(), chi.URLParam(r, "slug"), a.visitorID(w, r))
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "unable to like"})
		return
	}
	a.respondJSON(w, http.StatusOK, result)
}

func (a *App) handleCommentLike(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	result, err := a.store.LikeComment(r.Context(), id, a.visitorID(w, r))
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "unable to like"})
		return
	}
	a.respondJSON(w, http.StatusOK, result)
}

func (a *App) handleSharePoster(w http.ResponseWriter, r *http.Request) {
	post, err := a.store.GetPublishedPost(r.Context(), chi.URLParam(r, "slug"), "")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	summary := template.HTMLEscapeString(post.Excerpt)
	title := template.HTMLEscapeString(post.Title)
	url := template.HTMLEscapeString(strings.TrimRight(a.cfg.SiteURL, "/") + "/posts/" + post.Slug)
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
<defs>
<linearGradient id="bg" x1="0" x2="1" y1="0" y2="1">
<stop offset="0%%" stop-color="#f8f7f3"/>
<stop offset="100%%" stop-color="#e7e1d6"/>
</linearGradient>
</defs>
<rect width="1200" height="630" fill="url(#bg)"/>
<rect x="64" y="64" width="1072" height="502" rx="24" fill="#111111"/>
<text x="110" y="150" font-family="Georgia, serif" font-size="28" fill="#ff6ea8">%s</text>
<text x="110" y="230" font-family="Georgia, serif" font-size="62" font-weight="700" fill="#ffffff">%s</text>
<foreignObject x="110" y="280" width="980" height="180">
  <div xmlns="http://www.w3.org/1999/xhtml" style="font-family:Arial,sans-serif;font-size:26px;line-height:1.5;color:#d4d4d8;">%s</div>
</foreignObject>
<text x="110" y="520" font-family="Arial, sans-serif" font-size="24" fill="#e4e4e7">%s</text>
</svg>`, template.HTMLEscapeString(a.cfg.SiteName), title, summary, url)
}

func (a *App) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	user, err := a.store.AuthenticateAdmin(r.Context(), req.Username, req.Password)
	if err == store.ErrInvalidCredentials {
		a.respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	token, err := auth.Issue(a.cfg.JWTSecret, user.ID, user.Username)
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

func (a *App) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	claims, err := adminClaims(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":       claims.UserID,
			"username": claims.Username,
		},
	})
}

func (a *App) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.Dashboard(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, stats)
}

func (a *App) handleAdminPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := a.store.ListAdminPosts(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]any{"posts": posts})
}

func (a *App) handleAdminGetPost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	post, err := a.store.GetAdminPost(r.Context(), id)
	if err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "post not found"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, post)
}

func (a *App) handleAdminSavePost(w http.ResponseWriter, r *http.Request) {
	var req store.PostInput
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if idParam := chi.URLParam(r, "id"); idParam != "" {
		if id, err := strconv.ParseInt(idParam, 10, 64); err == nil {
			req.ID = id
		}
	}
	post, err := a.store.SavePost(r.Context(), req)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	a.invalidateContentCache(post.Slug)
	a.scheduleSemanticIndex(post.ID)
	a.respondJSON(w, http.StatusOK, post)
}

func (a *App) handleAdminDeletePost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := a.store.DeletePost(r.Context(), id); err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.invalidateContentCache("")
	a.respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAdminTaxonomies(w http.ResponseWriter, r *http.Request) {
	bundle, err := a.store.GetTaxonomies(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, bundle)
}

func (a *App) handleAdminSaveCategory(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	category, err := a.store.SaveCategory(r.Context(), req.Name, req.Description)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	a.taxonomyCache.Delete("all")
	a.respondJSON(w, http.StatusOK, category)
}

func (a *App) handleAdminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := a.store.DeleteCategory(r.Context(), id); err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.taxonomyCache.Delete("all")
	a.respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAdminSaveTag(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		Name string `json:"name"`
	}
	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	tag, err := a.store.SaveTag(r.Context(), req.Name)
	if err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	a.taxonomyCache.Delete("all")
	a.respondJSON(w, http.StatusOK, tag)
}

func (a *App) handleAdminDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := a.store.DeleteTag(r.Context(), id); err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.taxonomyCache.Delete("all")
	a.respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAdminComments(w http.ResponseWriter, r *http.Request) {
	comments, err := a.store.ListAdminComments(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

func (a *App) handleAdminReviewComment(w http.ResponseWriter, r *http.Request) {
	type payload struct {
		Status string `json:"status"`
	}
	var req payload
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := a.store.ReviewComment(r.Context(), id, req.Status); err != nil {
		if err == store.ErrNotFound {
			a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "comment not found"})
			return
		}
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if storeAction := strings.TrimSpace(strings.ToLower(req.Status)); storeAction == "approved" {
		if comment, err := a.store.GetCommentByID(r.Context(), id); err == nil {
			a.queueMentionNotifications(comment)
		}
	}
	a.respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.appSettings(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, settings)
}

func (a *App) handleAdminSaveSettings(w http.ResponseWriter, r *http.Request) {
	var req store.AppSettings
	if err := a.decodeJSON(r, &req); err != nil {
		a.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	settings, err := a.store.SaveAppSettings(r.Context(), req)
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	settings = mergeRuntimeOverrides(settings, a.cfg.RuntimeOverrides)
	a.settingsCache.Delete("current")
	a.settingsCache.Set("current", settings, time.Minute)
	a.mailer.Update(runtimeMailConfig(settings))
	a.reviewer.Update(runtimeAIConfig(settings))
	a.embedder.Update(runtimeEmbeddingConfig(settings))
	if settings.SemanticSearchEnabled && settings.EmbeddingDimensions > 0 {
		recreated, ensureErr := a.store.EnsureSemanticVectorTable(r.Context(), settings.EmbeddingDimensions)
		if ensureErr != nil && ensureErr != store.ErrInvalidInput {
			a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": ensureErr.Error()})
			return
		}
		if recreated {
			a.scheduleSemanticBackfill()
		}
	}
	if settings.SemanticSearchEnabled {
		a.scheduleSemanticBackfill()
	}
	a.respondJSON(w, http.StatusOK, settings)
}

func (a *App) handleAdminRerunAIReview(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	settings, err := a.appSettings(r.Context())
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "unable to load settings"})
		return
	}
	comment, decision, err := a.store.ReRunAIReview(r.Context(), id, settings.CommentReviewMode)
	if err == store.ErrNotFound {
		a.respondJSON(w, http.StatusNotFound, map[string]string{"error": "comment not found"})
		return
	}
	if err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if comment.Status == "approved" {
		a.queueMentionNotifications(comment)
	}
	a.respondJSON(w, http.StatusOK, map[string]any{
		"status":         decision.Status,
		"reason":         decision.Reason,
		"comment_status": comment.Status,
	})
}

func (a *App) handleAdminDeleteComment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := a.store.DeleteComment(r.Context(), id); err != nil {
		a.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	a.respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAdminApp(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(a.frontendDistFS, "admin.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("embedded frontend assets unavailable, rebuild the binary after `npm run build`"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "admin.html", time.Time{}, bytes.NewReader(content))
}

func (a *App) cachedPosts(ctx context.Context, key string, fetch func(context.Context) ([]store.PostSummary, error)) ([]store.PostSummary, error) {
	if posts, ok := a.postCache.Get(key); ok {
		return posts, nil
	}
	posts, err := fetch(ctx)
	if err != nil {
		return nil, err
	}
	a.postCache.Set(key, posts, 2*time.Minute)
	return posts, nil
}

func (a *App) cachedSearch(ctx context.Context, key string, fetch func(context.Context) (searchOutcome, error)) (searchOutcome, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return searchOutcome{RequestedMode: searchModeKeyword, ExecutedMode: searchModeKeyword, Results: []store.SearchResult{}}, nil
	}
	if results, ok := a.searchCache.Get(key); ok {
		return results, nil
	}
	results, err := fetch(ctx)
	if err != nil {
		return searchOutcome{}, err
	}
	a.searchCache.Set(key, results, time.Minute)
	return results, nil
}

func (a *App) invalidateContentCache(_ string) {
	a.postCache.DeletePrefix("")
	a.searchCache.DeletePrefix("")
	a.taxonomyCache.Delete("all")
}

func (a *App) applyPostCacheHeaders(w http.ResponseWriter, r *http.Request, post store.PostDetail) {
	modTime := post.UpdatedAt
	if modTime.IsZero() {
		modTime = post.PublishedAt
	}
	if modTime.IsZero() {
		return
	}

	modTime = modTime.UTC().Truncate(time.Second)
	etag := fmt.Sprintf(`W/"post-%d-%d"`, post.ID, modTime.Unix())
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", modTime.Format(http.TimeFormat))

	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.Header().Set("X-Cache-ShortCircuit", "304")
		return
	}
	if since := r.Header.Get("If-Modified-Since"); since != "" {
		if parsed, err := time.Parse(http.TimeFormat, since); err == nil && !modTime.After(parsed) {
			w.Header().Set("X-Cache-ShortCircuit", "304")
		}
	}
}

func aboutProfileFromSettings(settings store.AppSettings, siteName string) AboutProfile {
	name := strings.TrimSpace(settings.AboutName)
	if name == "" {
		name = siteName
	}

	tagline := strings.TrimSpace(settings.AboutTagline)
	if tagline == "" {
		tagline = "构建简洁、清晰、长期可维护的数字内容体验"
	}

	bio := strings.TrimSpace(settings.AboutBio)
	if bio == "" {
		bio = "这里可以通过后台设置页配置站点简介、联系方式、开源统计与友链。"
	}

	stats := make([]AboutStat, 0, 3)
	if value := strings.TrimSpace(settings.AboutRepoCount); value != "" {
		stats = append(stats, AboutStat{Label: "仓库", Value: value})
	}
	if value := strings.TrimSpace(settings.AboutStarCount); value != "" {
		stats = append(stats, AboutStat{Label: "Stars", Value: value})
	}
	if value := strings.TrimSpace(settings.AboutForkCount); value != "" {
		stats = append(stats, AboutStat{Label: "Forks", Value: value})
	}

	return AboutProfile{
		Name:       name,
		Tagline:    tagline,
		AvatarURL:  strings.TrimSpace(settings.AboutAvatarURL),
		AvatarText: aboutAvatarText(name),
		Email:      strings.TrimSpace(settings.AboutEmail),
		GitHubURL:  strings.TrimSpace(settings.AboutGitHubURL),
		Bio:        bio,
		Stats:      stats,
		Friends:    parseAboutFriends(settings.AboutFriendLinks),
	}
}

func aboutAvatarText(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "A"
	}

	runes := []rune(trimmed)
	if len(runes) == 1 {
		return strings.ToUpper(trimmed)
	}

	first := strings.ToUpper(string(runes[0]))
	last := strings.ToUpper(string(runes[len(runes)-1]))
	if first == last {
		return first
	}
	return first + last
}

func parseAboutFriends(raw string) []AboutFriend {
	fallbackAccents := []string{
		"linear-gradient(135deg, #5b7cfa 0%, #6fd3ff 100%)",
		"linear-gradient(135deg, #f97316 0%, #fb7185 100%)",
		"linear-gradient(135deg, #14b8a6 0%, #60a5fa 100%)",
		"linear-gradient(135deg, #a855f7 0%, #6366f1 100%)",
	}

	lines := strings.Split(raw, "\n")
	friends := make([]AboutFriend, 0, len(lines))
	for index, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		description := strings.TrimSpace(parts[1])
		link := strings.TrimSpace(parts[2])
		if name == "" || description == "" || link == "" {
			continue
		}
		if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
			continue
		}

		accent := fallbackAccents[index%len(fallbackAccents)]
		if len(parts) >= 4 {
			if parsed := parseAboutAccent(parts[3]); parsed != "" {
				accent = parsed
			}
		}

		friends = append(friends, AboutFriend{
			Name:        name,
			Description: description,
			URL:         link,
			Accent:      accent,
		})
	}

	return friends
}

func parseAboutAccent(raw string) string {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	if len(parts) != 2 {
		return ""
	}

	left := normalizeHexColor(parts[0])
	right := normalizeHexColor(parts[1])
	if left == "" || right == "" {
		return ""
	}

	return fmt.Sprintf("linear-gradient(135deg, %s 0%%, %s 100%%)", left, right)
}

func normalizeHexColor(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) != 7 || !strings.HasPrefix(value, "#") {
		return ""
	}
	for _, ch := range value[1:] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return ""
		}
	}
	return value
}

func parseQueryInt(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func clampArchiveLimit(limit int) int {
	if limit <= 0 {
		return 8
	}
	if limit > 24 {
		return 24
	}
	return limit
}
