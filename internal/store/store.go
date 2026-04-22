package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/snemc/snemc-blog/internal/ai"
	"github.com/snemc/snemc-blog/internal/config"
	"github.com/snemc/snemc-blog/internal/render"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials        = errors.New("invalid credentials")
	ErrNotFound                  = errors.New("not found")
	ErrRateLimited               = errors.New("too many comments in a short time")
	ErrInvalidInput              = errors.New("invalid input")
	ErrExpiredToken              = errors.New("expired token")
	ErrUsedToken                 = errors.New("used token")
	ErrInvalidAgentKey           = errors.New("invalid agent key")
	ErrSemanticSearchUnavailable = errors.New("semantic search unavailable")
)

const CurrentRenderVersion = 4
const routeIDAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"
const (
	CommentReviewModeManualAll           = "manual_all"
	CommentReviewModeAutoApproveAIPassed = "auto_approve_ai_passed"
)

var routeIDPattern = regexp.MustCompile(`^[23456789abcdefghjkmnpqrstuvwxyz]{10}$`)

type Store struct {
	db       *sql.DB
	renderer *render.Renderer
	reviewer ai.Reviewer
}

func New(db *sql.DB, renderer *render.Renderer, reviewer ai.Reviewer) *Store {
	return &Store{
		db:       db,
		renderer: renderer,
		reviewer: reviewer,
	}
}

func (s *Store) Bootstrap(ctx context.Context, cfg config.Config) error {
	if err := s.ensureAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		return err
	}
	if err := s.ensureAppSettings(ctx, cfg); err != nil {
		return err
	}
	if err := s.seedDemoContent(ctx); err != nil {
		return err
	}
	if err := s.ensurePostRouteIDs(ctx); err != nil {
		return err
	}
	if err := s.ensureVisitorDisplayNames(ctx); err != nil {
		return err
	}
	if err := s.normalizeLegacyAnonymousAuthors(ctx); err != nil {
		return err
	}
	return s.refreshRenderedPosts(ctx)
}

func (s *Store) ensureAppSettings(ctx context.Context, cfg config.Config) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO app_settings (
	id, smtp_host, smtp_port, smtp_username, smtp_password, smtp_from,
	admin_notify_email, llm_base_url, llm_api_key, llm_model, llm_system_prompt,
	embedding_base_url, embedding_api_key, embedding_model, embedding_dimensions,
	embedding_timeout_ms, semantic_search_enabled, comment_review_mode, updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO NOTHING
`, cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom, cfg.CommentNotifyTo, cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMSystemPrompt, cfg.EmbeddingBaseURL, cfg.EmbeddingAPIKey, cfg.EmbeddingModel, cfg.EmbeddingDimensions, cfg.EmbeddingTimeoutMS, boolToInt(cfg.SemanticSearchEnabled), CommentReviewModeManualAll)
	return err
}

func (s *Store) ensureAdmin(ctx context.Context, username string, password string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users WHERE username = ?`, username).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO admin_users (username, password_hash) VALUES (?, ?)`, username, string(hash))
	return err
}

func (s *Store) seedDemoContent(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM posts`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	input := PostInput{
		Title:        "构建高性能技术博客的渲染链路",
		Slug:         "build-a-fast-go-vue-blog",
		Summary:      "用 Go 预编译 Markdown，前台只做必要增强，把正文渲染成本前移到内容写入链路。",
		CoverImage:   "",
		Status:       "published",
		CategoryName: "Engineering",
		Tags:         []string{"Go", "Vue3", "SQLite", "Markdown"},
		Markdown: strings.Join([]string{
			"# 构建高性能技术博客的渲染链路",
			"",
			"这个示例文章用于验证：",
			"",
			"- Markdown 正文预渲染",
			"- 代码块高亮",
			"- 数学公式展示",
			"- Mermaid 图表渲染",
			"",
			"## 为什么不要把所有工作放到浏览器",
			"",
			"博客是读多写少场景，最适合把重计算移到保存时处理。",
			"",
			"```go",
			"func RenderPipeline(input string) string {",
			`	return "rendered-before-request"`,
			"}",
			"```",
			"",
			"## 数学公式",
			"",
			"行内公式示例：$E = mc^2$",
			"",
			"块级公式：",
			"",
			"$$",
			`\\int_0^1 x^2 dx = \\frac{1}{3}`,
			"$$",
			"",
			"## Mermaid",
			"",
			"```mermaid",
			"flowchart LR",
			"  Author[Admin Editor] --> Save[Save Markdown]",
			"  Save --> Render[Render HTML]",
			"  Render --> Search[Update Search Index]",
			"  Search --> Publish[Serve Fast Page]",
			"```",
			"",
			"## 搜索与缓存",
			"",
			"SQLite FTS5 可以满足中小型博客的全文检索。",
		}, "\n"),
	}

	_, err := s.SavePost(ctx, input)
	return err
}

func (s *Store) refreshRenderedPosts(ctx context.Context) error {
	type renderTarget struct {
		ID       int64
		Title    string
		Summary  string
		Markdown string
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, title, summary, markdown_source
FROM posts
WHERE render_version < ?
`, CurrentRenderVersion)
	if err != nil {
		return err
	}
	defer rows.Close()

	targets := []renderTarget{}
	for rows.Next() {
		var item renderTarget
		if err := rows.Scan(&item.ID, &item.Title, &item.Summary, &item.Markdown); err != nil {
			return err
		}
		targets = append(targets, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range targets {
		result, err := s.renderer.Render(item.Markdown)
		if err != nil {
			return err
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
UPDATE posts
SET rendered_html = ?, toc_json = ?, excerpt = ?, word_count = ?, reading_time = ?, render_version = ?
WHERE id = ?
`, result.HTML, marshalTOC(result.TOC), result.Excerpt, result.WordCount, result.ReadingTime, CurrentRenderVersion, item.ID); err != nil {
			tx.Rollback()
			return err
		}

		if err := s.syncPostFTSTx(ctx, tx, item.ID, item.Title, item.Summary, result.PlainText); err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func normalizeSlug(value string) string {
	if strings.TrimSpace(value) == "" {
		value = "post"
	}
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "post"
	}
	return result
}

func normalizeCommentReviewMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case CommentReviewModeAutoApproveAIPassed:
		return CommentReviewModeAutoApproveAIPassed
	default:
		return CommentReviewModeManualAll
	}
}

func resolveCommentStatus(reviewMode string, decision ai.Decision) string {
	if normalizeCommentReviewMode(reviewMode) == CommentReviewModeAutoApproveAIPassed && decision.Status == "approved" {
		return "approved"
	}
	return "pending"
}

func generateRouteID() (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	var out strings.Builder
	out.Grow(len(buf))
	for _, b := range buf {
		out.WriteByte(routeIDAlphabet[int(b)%len(routeIDAlphabet)])
	}
	return out.String(), nil
}

func generateOpaqueSecret(prefix string, size int) (string, error) {
	if size <= 0 {
		size = 24
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buf), nil
}

func hashSecretValue(input string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(input)))
	return hex.EncodeToString(sum[:])
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func shouldReplaceRouteID(input string) bool {
	return !routeIDPattern.MatchString(strings.TrimSpace(strings.ToLower(input)))
}

func (s *Store) uniqueRouteIDTx(ctx context.Context, tx *sql.Tx, excludeID int64) (string, error) {
	for i := 0; i < 8; i++ {
		candidate, err := generateRouteID()
		if err != nil {
			return "", err
		}

		var count int
		query := `SELECT COUNT(*) FROM posts WHERE slug = ?`
		args := []any{candidate}
		if excludeID > 0 {
			query += ` AND id <> ?`
			args = append(args, excludeID)
		}
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
	}
	return "", errors.New("unable to generate unique route id")
}

func (s *Store) ensurePostRouteIDs(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, slug FROM posts`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type routeTarget struct {
		ID   int64
		Slug string
	}

	targets := []routeTarget{}
	for rows.Next() {
		var item routeTarget
		if err := rows.Scan(&item.ID, &item.Slug); err != nil {
			return err
		}
		if shouldReplaceRouteID(item.Slug) {
			targets = append(targets, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range targets {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		routeID, err := s.uniqueRouteIDTx(ctx, tx, item.ID)
		if err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE posts SET slug = ? WHERE id = ?`, routeID, item.ID); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func normalizeTagNames(values []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result
}

func isLegacyAnonymousAuthorName(input string) bool {
	normalized := strings.TrimSpace(input)
	switch normalized {
	case "", "匿名访客", "匿名读者", "匿名身份":
		return true
	default:
		return false
	}
}

func (s *Store) ensureVisitorDisplayNames(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
SELECT visitor_id, fingerprint, display_name
FROM visitors
`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type visitorRow struct {
		VisitorID   string
		Fingerprint string
		DisplayName string
	}

	items := []visitorRow{}
	for rows.Next() {
		var item visitorRow
		if err := rows.Scan(&item.VisitorID, &item.Fingerprint, &item.DisplayName); err != nil {
			return err
		}
		if strings.TrimSpace(item.DisplayName) == "" {
			items = append(items, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range items {
		displayName := generateVisitorDisplayName(item.Fingerprint, item.VisitorID)
		if _, err := s.db.ExecContext(ctx, `
UPDATE visitors
SET display_name = ?
WHERE visitor_id = ?
`, displayName, item.VisitorID); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) normalizeLegacyAnonymousAuthors(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE comments
SET author_name = ''
WHERE TRIM(author_name) IN ('匿名访客', '匿名读者', '匿名身份')
`)
	return err
}

func parseDBTime(input sql.NullString) time.Time {
	if !input.Valid || input.String == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, input.String, time.Local); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func hashIP(ip string) string {
	return hashSecretValue(ip)
}

func sanitizeCommentAuthor(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "匿名读者"
	}
	input = regexp.MustCompile(`\s+`).ReplaceAllString(input, " ")
	runes := []rune(input)
	if len(runes) > 24 {
		return string(runes[:24])
	}
	return input
}

func sanitizeCommentContent(input string) string {
	input = strings.TrimSpace(input)
	input = regexp.MustCompile(`\r\n?`).ReplaceAllString(input, "\n")
	runes := []rune(input)
	if len(runes) > 1200 {
		input = string(runes[:1200])
	}
	return input
}

func sanitizeContactEmail(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	if len(input) > 320 {
		return input[:320]
	}
	return input
}

func marshalTOC(headings []render.Heading) string {
	body, err := json.Marshal(headings)
	if err != nil {
		return "[]"
	}
	return string(body)
}

func (s *Store) ensureCategoryTx(ctx context.Context, tx *sql.Tx, name string) (Category, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "General"
	}
	slug := normalizeSlug(name)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO categories (name, slug)
VALUES (?, ?)
ON CONFLICT(slug) DO UPDATE SET name = excluded.name
`, name, slug); err != nil {
		return Category{}, err
	}

	category := Category{}
	err := tx.QueryRowContext(ctx, `SELECT id, name, slug, description FROM categories WHERE slug = ?`, slug).
		Scan(&category.ID, &category.Name, &category.Slug, &category.Description)
	return category, err
}

func (s *Store) ensureTagTx(ctx context.Context, tx *sql.Tx, name string) (Tag, error) {
	name = strings.TrimSpace(name)
	slug := normalizeSlug(name)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO tags (name, slug)
VALUES (?, ?)
ON CONFLICT(slug) DO UPDATE SET name = excluded.name
`, name, slug); err != nil {
		return Tag{}, err
	}

	tag := Tag{}
	err := tx.QueryRowContext(ctx, `SELECT id, name, slug FROM tags WHERE slug = ?`, slug).
		Scan(&tag.ID, &tag.Name, &tag.Slug)
	return tag, err
}

func (s *Store) syncPostFTSTx(ctx context.Context, tx *sql.Tx, postID int64, title string, summary string, content string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM post_fts WHERE post_id = ?`, postID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO post_fts (post_id, title, summary, content) VALUES (?, ?, ?, ?)`, postID, title, summary, content)
	return err
}

func (s *Store) attachTags(ctx context.Context, posts []PostSummary) ([]PostSummary, error) {
	if len(posts) == 0 {
		return posts, nil
	}

	ids := make([]string, 0, len(posts))
	index := make(map[int64]int, len(posts))
	for i, post := range posts {
		ids = append(ids, fmt.Sprintf("%d", post.ID))
		index[post.ID] = i
	}

	query := `
SELECT pt.post_id, t.id, t.name, t.slug
FROM post_tags pt
JOIN tags t ON t.id = pt.tag_id
WHERE pt.post_id IN (` + strings.Join(ids, ",") + `)
ORDER BY t.name ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var postID int64
		var tag Tag
		if err := rows.Scan(&postID, &tag.ID, &tag.Name, &tag.Slug); err != nil {
			return nil, err
		}
		if idx, ok := index[postID]; ok {
			posts[idx].Tags = append(posts[idx].Tags, tag)
		}
	}

	return posts, rows.Err()
}
