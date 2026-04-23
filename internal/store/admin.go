package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/snemc/snemc-blog/internal/ai"
	"golang.org/x/crypto/bcrypt"
)

func (s *Store) GetAppSettings(ctx context.Context) (AppSettings, error) {
	var settings AppSettings
	err := s.db.QueryRowContext(ctx, `
	SELECT
		smtp_host,
		smtp_port,
		smtp_username,
	smtp_password,
	smtp_from,
	admin_notify_email,
	llm_base_url,
	llm_api_key,
	llm_model,
	llm_system_prompt,
	embedding_base_url,
	embedding_api_key,
	embedding_model,
		embedding_dimensions,
		embedding_timeout_ms,
		semantic_search_enabled,
		comment_review_mode,
		about_name,
		about_tagline,
		about_avatar_url,
		about_email,
		about_github_url,
		about_bio,
		about_repo_count,
		about_star_count,
		about_fork_count,
		about_friend_links
	FROM app_settings
	WHERE id = 1
	`).Scan(
		&settings.SMTPHost,
		&settings.SMTPPort,
		&settings.SMTPUsername,
		&settings.SMTPPassword,
		&settings.SMTPFrom,
		&settings.AdminNotifyEmail,
		&settings.LLMBaseURL,
		&settings.LLMAPIKey,
		&settings.LLMModel,
		&settings.LLMSystemPrompt,
		&settings.EmbeddingBaseURL,
		&settings.EmbeddingAPIKey,
		&settings.EmbeddingModel,
			&settings.EmbeddingDimensions,
			&settings.EmbeddingTimeoutMS,
			&settings.SemanticSearchEnabled,
			&settings.CommentReviewMode,
			&settings.AboutName,
			&settings.AboutTagline,
			&settings.AboutAvatarURL,
			&settings.AboutEmail,
			&settings.AboutGitHubURL,
			&settings.AboutBio,
			&settings.AboutRepoCount,
			&settings.AboutStarCount,
			&settings.AboutForkCount,
			&settings.AboutFriendLinks,
		)
	if errorsIsNoRows(err) {
		return AppSettings{}, ErrNotFound
	}
	if settings.EmbeddingTimeoutMS <= 0 {
		settings.EmbeddingTimeoutMS = 15000
	}
	settings.CommentReviewMode = normalizeCommentReviewMode(settings.CommentReviewMode)
	return settings, err
}

func (s *Store) SaveAppSettings(ctx context.Context, input AppSettings) (AppSettings, error) {
	if strings.TrimSpace(input.SMTPPort) == "" {
		input.SMTPPort = "587"
	}
	if strings.TrimSpace(input.LLMSystemPrompt) == "" {
		input.LLMSystemPrompt = "You are a blog moderation assistant."
	}
	if input.EmbeddingTimeoutMS <= 0 {
		input.EmbeddingTimeoutMS = 15000
	}
	if input.EmbeddingDimensions < 0 {
		input.EmbeddingDimensions = 0
	}
	input.CommentReviewMode = normalizeCommentReviewMode(input.CommentReviewMode)
	_, err := s.db.ExecContext(ctx, `
UPDATE app_settings
SET
	smtp_host = ?,
	smtp_port = ?,
	smtp_username = ?,
	smtp_password = ?,
	smtp_from = ?,
	admin_notify_email = ?,
	llm_base_url = ?,
	llm_api_key = ?,
	llm_model = ?,
	llm_system_prompt = ?,
	embedding_base_url = ?,
	embedding_api_key = ?,
		embedding_model = ?,
		embedding_dimensions = ?,
		embedding_timeout_ms = ?,
		semantic_search_enabled = ?,
		comment_review_mode = ?,
		about_name = ?,
		about_tagline = ?,
		about_avatar_url = ?,
		about_email = ?,
		about_github_url = ?,
		about_bio = ?,
		about_repo_count = ?,
		about_star_count = ?,
		about_fork_count = ?,
		about_friend_links = ?,
		updated_at = CURRENT_TIMESTAMP
WHERE id = 1
`, strings.TrimSpace(input.SMTPHost), strings.TrimSpace(input.SMTPPort), strings.TrimSpace(input.SMTPUsername), input.SMTPPassword, strings.TrimSpace(input.SMTPFrom), strings.TrimSpace(input.AdminNotifyEmail), strings.TrimSpace(input.LLMBaseURL), strings.TrimSpace(input.LLMAPIKey), strings.TrimSpace(input.LLMModel), input.LLMSystemPrompt, strings.TrimSpace(input.EmbeddingBaseURL), strings.TrimSpace(input.EmbeddingAPIKey), strings.TrimSpace(input.EmbeddingModel), input.EmbeddingDimensions, input.EmbeddingTimeoutMS, input.SemanticSearchEnabled, input.CommentReviewMode, strings.TrimSpace(input.AboutName), strings.TrimSpace(input.AboutTagline), strings.TrimSpace(input.AboutAvatarURL), strings.TrimSpace(input.AboutEmail), strings.TrimSpace(input.AboutGitHubURL), strings.TrimSpace(input.AboutBio), strings.TrimSpace(input.AboutRepoCount), strings.TrimSpace(input.AboutStarCount), strings.TrimSpace(input.AboutForkCount), strings.TrimSpace(input.AboutFriendLinks))
		if err != nil {
			return AppSettings{}, err
		}
	return s.GetAppSettings(ctx)
}

func (s *Store) AuthenticateAdmin(ctx context.Context, username string, password string) (AdminUser, error) {
	var user AdminUser
	var passwordHash string
	err := s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash
FROM admin_users
WHERE username = ?
`, username).Scan(&user.ID, &user.Username, &passwordHash)
	if errorsIsNoRows(err) {
		return AdminUser{}, ErrInvalidCredentials
	}
	if err != nil {
		return AdminUser{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return AdminUser{}, ErrInvalidCredentials
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE admin_users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?`, user.ID)
	return user, nil
}

func (s *Store) Dashboard(ctx context.Context) (DashboardStats, error) {
	var stats DashboardStats
	err := s.db.QueryRowContext(ctx, `
SELECT
	(SELECT COUNT(*) FROM posts WHERE status = 'published'),
	(SELECT COUNT(*) FROM posts WHERE status = 'draft'),
	(SELECT COUNT(*) FROM comments WHERE status = 'pending'),
	(SELECT COUNT(*) FROM page_views),
	(SELECT COUNT(DISTINCT visitor_id) FROM page_views WHERE datetime(created_at) >= datetime('now', '-7 day')),
	(SELECT COUNT(*) FROM search_logs WHERE datetime(created_at) >= datetime('now', '-7 day'))
`).Scan(
		&stats.PublishedPosts,
		&stats.DraftPosts,
		&stats.PendingComments,
		&stats.TotalViews,
		&stats.ActiveVisitors,
		&stats.Searches7d,
	)
	return stats, err
}

func (s *Store) ListAdminPosts(ctx context.Context) ([]PostSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
	p.id,
	p.title,
	p.slug,
	p.summary,
	p.excerpt,
	p.cover_image,
	p.status,
	COALESCE(c.id, 0),
	COALESCE(c.name, ''),
	COALESCE(c.slug, ''),
	p.word_count,
	p.reading_time,
	p.published_at,
	p.updated_at,
	COALESCE(v.views, 0),
	COALESCE(l.likes, 0)
FROM posts p
LEFT JOIN categories c ON c.id = p.category_id
LEFT JOIN (SELECT post_id, COUNT(*) AS views FROM page_views GROUP BY post_id) v ON v.post_id = p.id
LEFT JOIN (SELECT post_id, COUNT(*) AS likes FROM post_likes GROUP BY post_id) l ON l.post_id = p.id
ORDER BY datetime(p.updated_at) DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := []PostSummary{}
	for rows.Next() {
		var post PostSummary
		var publishedAt sql.NullString
		var updatedAt sql.NullString
		if err := rows.Scan(
			&post.ID,
			&post.Title,
			&post.Slug,
			&post.Summary,
			&post.Excerpt,
			&post.CoverImage,
			&post.Status,
			&post.CategoryID,
			&post.CategoryName,
			&post.CategorySlug,
			&post.WordCount,
			&post.ReadingTime,
			&publishedAt,
			&updatedAt,
			&post.Views,
			&post.Likes,
		); err != nil {
			return nil, err
		}
		post.PublishedAt = parseDBTime(publishedAt)
		post.UpdatedAt = parseDBTime(updatedAt)
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachTags(ctx, posts)
}

func (s *Store) GetAdminPost(ctx context.Context, id int64) (PostDetail, error) {
	var post PostDetail
	var publishedAt sql.NullString
	var updatedAt sql.NullString
	var categoryID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT
	p.id,
	p.title,
	p.slug,
	p.summary,
	p.excerpt,
	p.cover_image,
	p.status,
	p.category_id,
	COALESCE(c.name, ''),
	COALESCE(c.slug, ''),
	p.word_count,
	p.reading_time,
	p.published_at,
	p.updated_at,
	p.markdown_source,
	p.rendered_html,
	p.toc_json,
	COALESCE(v.views, 0),
	COALESCE(l.likes, 0)
FROM posts p
LEFT JOIN categories c ON c.id = p.category_id
LEFT JOIN (SELECT post_id, COUNT(*) AS views FROM page_views GROUP BY post_id) v ON v.post_id = p.id
LEFT JOIN (SELECT post_id, COUNT(*) AS likes FROM post_likes GROUP BY post_id) l ON l.post_id = p.id
WHERE p.id = ?
`, id).Scan(
		&post.ID,
		&post.Title,
		&post.Slug,
		&post.Summary,
		&post.Excerpt,
		&post.CoverImage,
		&post.Status,
		&categoryID,
		&post.CategoryName,
		&post.CategorySlug,
		&post.WordCount,
		&post.ReadingTime,
		&publishedAt,
		&updatedAt,
		&post.MarkdownSource,
		&post.RenderedHTML,
		&post.TOCJSON,
		&post.Views,
		&post.Likes,
	)
	if errorsIsNoRows(err) {
		return PostDetail{}, ErrNotFound
	}
	if err != nil {
		return PostDetail{}, err
	}
	if categoryID.Valid {
		post.CategoryID = categoryID.Int64
	}
	post.PublishedAt = parseDBTime(publishedAt)
	post.UpdatedAt = parseDBTime(updatedAt)
	withTags, err := s.attachTags(ctx, []PostSummary{post.PostSummary})
	if err != nil {
		return PostDetail{}, err
	}
	post.PostSummary = withTags[0]
	return post, nil
}

func (s *Store) SavePost(ctx context.Context, input PostInput) (PostDetail, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PostDetail{}, err
	}
	defer tx.Rollback()

	result, err := s.renderer.Render(input.Markdown)
	if err != nil {
		return PostDetail{}, err
	}

	if strings.TrimSpace(input.Title) == "" {
		input.Title = "Untitled Post"
	}
	if input.Status != "published" {
		input.Status = "draft"
	}

	category, err := s.ensureCategoryTx(ctx, tx, input.CategoryName)
	if err != nil {
		return PostDetail{}, err
	}

	slug := strings.TrimSpace(input.Slug)

	now := time.Now().Format("2006-01-02 15:04:05")
	if input.ID == 0 {
		routeID, err := s.uniqueRouteIDTx(ctx, tx, 0)
		if err != nil {
			return PostDetail{}, err
		}
		slug = routeID
		publishedAt := any(nil)
		if input.Status == "published" {
			publishedAt = now
		}
		res, err := tx.ExecContext(ctx, `
INSERT INTO posts (
	title, slug, summary, markdown_source, rendered_html, toc_json, excerpt,
	cover_image, status, category_id, word_count, reading_time, render_version,
	created_at, updated_at, published_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, input.Title, slug, input.Summary, input.Markdown, result.HTML, marshalTOC(result.TOC), result.Excerpt,
			input.CoverImage, input.Status, category.ID, result.WordCount, result.ReadingTime, CurrentRenderVersion, now, now, publishedAt)
		if err != nil {
			return PostDetail{}, err
		}
		input.ID, err = res.LastInsertId()
		if err != nil {
			return PostDetail{}, err
		}
	} else {
		var existingPublishedAt sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT published_at FROM posts WHERE id = ?`, input.ID).Scan(&existingPublishedAt); err != nil {
			return PostDetail{}, err
		}
		if slug == "" || shouldReplaceRouteID(slug) {
			if err := tx.QueryRowContext(ctx, `SELECT slug FROM posts WHERE id = ?`, input.ID).Scan(&slug); err != nil {
				return PostDetail{}, err
			}
			if shouldReplaceRouteID(slug) {
				slug, err = s.uniqueRouteIDTx(ctx, tx, input.ID)
				if err != nil {
					return PostDetail{}, err
				}
			}
		}
		publishedAt := any(nil)
		if existingPublishedAt.Valid {
			publishedAt = existingPublishedAt.String
		}
		if input.Status == "published" && !existingPublishedAt.Valid {
			publishedAt = now
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE posts
SET title = ?, slug = ?, summary = ?, markdown_source = ?, rendered_html = ?, toc_json = ?,
	excerpt = ?, cover_image = ?, status = ?, category_id = ?, word_count = ?,
	reading_time = ?, render_version = ?, updated_at = ?, published_at = ?
WHERE id = ?
`, input.Title, slug, input.Summary, input.Markdown, result.HTML, marshalTOC(result.TOC), result.Excerpt,
			input.CoverImage, input.Status, category.ID, result.WordCount, result.ReadingTime, CurrentRenderVersion, now, publishedAt, input.ID); err != nil {
			return PostDetail{}, err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM post_tags WHERE post_id = ?`, input.ID); err != nil {
		return PostDetail{}, err
	}
	for _, name := range normalizeTagNames(input.Tags) {
		tag, err := s.ensureTagTx(ctx, tx, name)
		if err != nil {
			return PostDetail{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO post_tags (post_id, tag_id) VALUES (?, ?)`, input.ID, tag.ID); err != nil {
			return PostDetail{}, err
		}
	}

	if err := s.syncPostFTSTx(ctx, tx, input.ID, input.Title, input.Summary, result.PlainText); err != nil {
		return PostDetail{}, err
	}
	if err := tx.Commit(); err != nil {
		return PostDetail{}, err
	}

	return s.GetAdminPost(ctx, input.ID)
}

func (s *Store) DeletePost(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM post_fts WHERE post_id = ?`, id); err != nil {
		return err
	}
	if err := s.DeleteSemanticIndexTx(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM posts WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetTaxonomies(ctx context.Context) (TaxonomyBundle, error) {
	bundle := TaxonomyBundle{}

	categoryRows, err := s.db.QueryContext(ctx, `SELECT id, name, slug, description FROM categories ORDER BY name ASC`)
	if err != nil {
		return bundle, err
	}
	defer categoryRows.Close()
	for categoryRows.Next() {
		var category Category
		if err := categoryRows.Scan(&category.ID, &category.Name, &category.Slug, &category.Description); err != nil {
			return bundle, err
		}
		bundle.Categories = append(bundle.Categories, category)
	}
	if err := categoryRows.Err(); err != nil {
		return bundle, err
	}

	tagRows, err := s.db.QueryContext(ctx, `SELECT id, name, slug FROM tags ORDER BY name ASC`)
	if err != nil {
		return bundle, err
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var tag Tag
		if err := tagRows.Scan(&tag.ID, &tag.Name, &tag.Slug); err != nil {
			return bundle, err
		}
		bundle.Tags = append(bundle.Tags, tag)
	}
	return bundle, tagRows.Err()
}

func (s *Store) SaveCategory(ctx context.Context, name string, description string) (Category, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Category{}, err
	}
	defer tx.Rollback()

	category, err := s.ensureCategoryTx(ctx, tx, name)
	if err != nil {
		return Category{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE categories SET description = ? WHERE id = ?`, description, category.ID); err != nil {
		return Category{}, err
	}
	if err := tx.Commit(); err != nil {
		return Category{}, err
	}
	category.Description = description
	return category, nil
}

func (s *Store) UpdateCategory(ctx context.Context, id int64, name string, description string) (Category, error) {
	name = strings.TrimSpace(name)
	if id == 0 || name == "" {
		return Category{}, ErrInvalidInput
	}
	slug := normalizeSlug(name)
	result, err := s.db.ExecContext(ctx, `
UPDATE categories
SET name = ?, slug = ?, description = ?
WHERE id = ?
`, name, slug, description, id)
	if err != nil {
		return Category{}, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Category{}, err
	}
	if rowsAffected == 0 {
		return Category{}, ErrNotFound
	}
	category := Category{}
	err = s.db.QueryRowContext(ctx, `SELECT id, name, slug, description FROM categories WHERE id = ?`, id).
		Scan(&category.ID, &category.Name, &category.Slug, &category.Description)
	if errorsIsNoRows(err) {
		return Category{}, ErrNotFound
	}
	return category, err
}

func (s *Store) SaveTag(ctx context.Context, name string) (Tag, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Tag{}, err
	}
	defer tx.Rollback()
	tag, err := s.ensureTagTx(ctx, tx, name)
	if err != nil {
		return Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return Tag{}, err
	}
	return tag, nil
}

func (s *Store) UpdateTag(ctx context.Context, id int64, name string) (Tag, error) {
	name = strings.TrimSpace(name)
	if id == 0 || name == "" {
		return Tag{}, ErrInvalidInput
	}
	slug := normalizeSlug(name)
	result, err := s.db.ExecContext(ctx, `
UPDATE tags
SET name = ?, slug = ?
WHERE id = ?
`, name, slug, id)
	if err != nil {
		return Tag{}, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Tag{}, err
	}
	if rowsAffected == 0 {
		return Tag{}, ErrNotFound
	}
	tag := Tag{}
	err = s.db.QueryRowContext(ctx, `SELECT id, name, slug FROM tags WHERE id = ?`, id).
		Scan(&tag.ID, &tag.Name, &tag.Slug)
	if errorsIsNoRows(err) {
		return Tag{}, ErrNotFound
	}
	return tag, err
}

func (s *Store) DeleteCategory(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM categories WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListAdminComments(ctx context.Context) ([]Comment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
	c.id,
	c.post_id,
	c.parent_id,
	p.title,
	c.visitor_id,
	c.author_name,
	c.email,
	c.content,
	c.status,
	c.ai_review_status,
	c.ai_review_reason,
	COALESCE(cn.delivery_status, ''),
	COALESCE(cn.error_message, ''),
	COALESCE(l.likes, 0),
	c.created_at
FROM comments c
JOIN posts p ON p.id = c.post_id
LEFT JOIN (SELECT comment_id, COUNT(*) AS likes FROM comment_likes GROUP BY comment_id) l ON l.comment_id = c.id
LEFT JOIN (
	SELECT cn1.comment_id, cn1.delivery_status, cn1.error_message
	FROM comment_notifications cn1
	JOIN (
		SELECT comment_id, MAX(id) AS max_id
		FROM comment_notifications
		GROUP BY comment_id
	) cn2 ON cn2.max_id = cn1.id
) cn ON cn.comment_id = c.id
ORDER BY
	CASE c.status WHEN 'pending' THEN 0 WHEN 'approved' THEN 1 ELSE 2 END,
	datetime(c.created_at) DESC
LIMIT 100
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Comment{}
	for rows.Next() {
		var comment Comment
		var parentID sql.NullInt64
		var createdAt sql.NullString
		if err := rows.Scan(
			&comment.ID,
			&comment.PostID,
			&parentID,
			&comment.PostTitle,
			&comment.VisitorID,
			&comment.AuthorName,
			&comment.Email,
			&comment.Content,
			&comment.Status,
			&comment.AIReviewStatus,
			&comment.AIReviewReason,
			&comment.NotifyStatus,
			&comment.NotifyError,
			&comment.Likes,
			&createdAt,
		); err != nil {
			return nil, err
		}
		if parentID.Valid {
			comment.ParentID = &parentID.Int64
		}
		comment.CreatedAt = parseDBTime(createdAt)
		items = append(items, comment)
	}
	return items, rows.Err()
}

func (s *Store) ReviewComment(ctx context.Context, id int64, status string) error {
	status = normalizeCommentAction(status)
	result, err := s.db.ExecContext(ctx, `UPDATE comments SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ReRunAIReview(ctx context.Context, id int64, reviewMode string) (Comment, ai.Decision, error) {
	var content string
	var visitorID string
	var postTitle string
	var currentStatus string
	err := s.db.QueryRowContext(ctx, `
SELECT c.content, c.visitor_id, p.title, c.status
FROM comments c
JOIN posts p ON p.id = c.post_id
WHERE c.id = ?
	`, id).Scan(&content, &visitorID, &postTitle, &currentStatus)
	if errorsIsNoRows(err) {
		return Comment{}, ai.Decision{}, ErrNotFound
	}
	if err != nil {
		return Comment{}, ai.Decision{}, err
	}

	decision, err := s.reviewer.ReviewComment(ctx, ai.Input{
		PostTitle: postTitle,
		Content:   content,
		VisitorID: visitorID,
	})
	if err != nil {
		return Comment{}, ai.Decision{}, err
	}

	nextStatus := currentStatus
	if currentStatus == "pending" {
		nextStatus = resolveCommentStatus(reviewMode, decision)
	}

	_, err = s.db.ExecContext(ctx, `
UPDATE comments
SET ai_review_status = ?, ai_review_reason = ?, status = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
	`, decision.Status, decision.Reason, nextStatus, id)
	if err != nil {
		return Comment{}, ai.Decision{}, err
	}

	comment, err := s.GetCommentByID(ctx, id)
	if err != nil {
		return Comment{}, ai.Decision{}, err
	}
	return comment, decision, nil
}

func (s *Store) DeleteComment(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM comments WHERE id = ?`, id)
	return err
}
