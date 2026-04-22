package store

import (
	"context"
	"database/sql"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/snemc/snemc-blog/internal/ai"
)

var nicknamePrefixPool = []string{
	"漫游的", "冷静的", "轻快的", "偏执的", "明亮的", "沉稳的", "跃迁的", "稀有的",
	"安静的", "敏捷的", "理性的", "灵巧的", "超新的", "稳态的", "极夜的", "自洽的",
}

var nicknameNounPool = []string{
	"观测者", "流星", "棱镜", "航标", "极光", "鲸歌", "新星", "旅人",
	"回声", "矩阵", "潮汐", "星图", "余烬", "微光", "天穹", "锚点",
}

func (s *Store) UpsertVisitor(ctx context.Context, input VisitorInput) (VisitorProfile, error) {
	if input.VisitorID == "" {
		return VisitorProfile{}, nil
	}

	profile := VisitorProfile{VisitorID: input.VisitorID}
	err := s.db.QueryRowContext(ctx, `
SELECT display_name, contact_email
FROM visitors
WHERE visitor_id = ?
`, input.VisitorID).Scan(&profile.DisplayName, &profile.ContactEmail)
	if err != nil && !errorsIsNoRows(err) {
		return VisitorProfile{}, err
	}
	if profile.DisplayName == "" {
		profile.DisplayName = generateVisitorDisplayName(input.Fingerprint, input.VisitorID)
	}

	if errorsIsNoRows(err) {
		_, err = s.db.ExecContext(ctx, `
INSERT INTO visitors (visitor_id, fingerprint, user_agent, language, display_name, contact_email, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`, input.VisitorID, input.Fingerprint, input.UserAgent, input.Language, profile.DisplayName, profile.ContactEmail)
		return profile, err
	}

	_, err = s.db.ExecContext(ctx, `
UPDATE visitors
SET fingerprint = ?, user_agent = ?, language = ?, display_name = COALESCE(NULLIF(display_name, ''), ?), contact_email = COALESCE(NULLIF(contact_email, ''), ?), last_seen_at = CURRENT_TIMESTAMP
WHERE visitor_id = ?
`, input.Fingerprint, input.UserAgent, input.Language, profile.DisplayName, profile.ContactEmail, input.VisitorID)
	return profile, err
}

func generateVisitorDisplayName(fingerprint string, visitorID string) string {
	seed := strings.TrimSpace(fingerprint)
	if seed == "" {
		seed = visitorID
	}
	if seed == "" {
		seed = "visitor"
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(seed))
	value := hasher.Sum32()
	prefix := nicknamePrefixPool[int(value)%len(nicknamePrefixPool)]
	noun := nicknameNounPool[int(value/uint32(len(nicknamePrefixPool)))%len(nicknameNounPool)]
	return prefix + noun
}

var leadingArticleTitlePattern = regexp.MustCompile(`(?s)^\s*<h1[^>]*>.*?</h1>\s*`)

func stripLeadingArticleTitle(input string) string {
	return strings.TrimSpace(leadingArticleTitlePattern.ReplaceAllString(input, ""))
}

func (s *Store) ResolveVisitorDisplayName(ctx context.Context, visitorID string, fingerprint string) (string, error) {
	if visitorID == "" {
		return generateVisitorDisplayName(fingerprint, visitorID), nil
	}

	var displayName string
	err := s.db.QueryRowContext(ctx, `SELECT display_name FROM visitors WHERE visitor_id = ?`, visitorID).Scan(&displayName)
	if err != nil && !errorsIsNoRows(err) {
		return "", err
	}
	if displayName == "" {
		displayName = generateVisitorDisplayName(fingerprint, visitorID)
	}
	return displayName, nil
}

func (s *Store) UpdateVisitorDisplayName(ctx context.Context, visitorID string, displayName string) error {
	if visitorID == "" || strings.TrimSpace(displayName) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE visitors
SET display_name = ?
WHERE visitor_id = ?
`, strings.TrimSpace(displayName), visitorID)
	return err
}

func (s *Store) UpdateVisitorContactEmail(ctx context.Context, visitorID string, email string) error {
	if visitorID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE visitors
SET contact_email = ?, last_seen_at = CURRENT_TIMESTAMP
WHERE visitor_id = ?
`, sanitizeContactEmail(email), visitorID)
	return err
}

func (s *Store) RecordPageView(ctx context.Context, visitorID string, postID *int64, path string, referrer string) error {
	if visitorID == "" {
		return nil
	}

	var nullablePostID any
	if postID != nil {
		nullablePostID = *postID
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO page_views (post_id, visitor_id, path, referrer)
VALUES (?, ?, ?, ?)
`, nullablePostID, visitorID, path, referrer)
	return err
}

func (s *Store) RecordSearch(ctx context.Context, visitorID string, query string) error {
	query = strings.TrimSpace(query)
	if visitorID == "" || query == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO search_logs (visitor_id, query) VALUES (?, ?)`, visitorID, query)
	return err
}

func (s *Store) ListPublishedPosts(ctx context.Context, categorySlug string, tagSlug string, limit int) ([]PostSummary, error) {
	return s.listPublishedPosts(ctx, categorySlug, tagSlug, limit, 0)
}

func (s *Store) ListPublishedPostsPage(ctx context.Context, categorySlug string, tagSlug string, limit int, offset int) ([]PostSummary, error) {
	return s.listPublishedPosts(ctx, categorySlug, tagSlug, limit, offset)
}

func (s *Store) CountPublishedPosts(ctx context.Context, categorySlug string, tagSlug string) (int, error) {
	args := []any{}
	clauses := []string{"p.status = 'published'"}
	if categorySlug != "" {
		clauses = append(clauses, "c.slug = ?")
		args = append(args, categorySlug)
	}
	if tagSlug != "" {
		clauses = append(clauses, "t.slug = ?")
		args = append(args, tagSlug)
	}

	var total int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT p.id)
FROM posts p
LEFT JOIN categories c ON c.id = p.category_id
LEFT JOIN post_tags pt ON pt.post_id = p.id
LEFT JOIN tags t ON t.id = pt.tag_id
WHERE `+strings.Join(clauses, " AND ")).Scan(&total)
	return total, err
}

func (s *Store) listPublishedPosts(ctx context.Context, categorySlug string, tagSlug string, limit int, offset int) ([]PostSummary, error) {
	if limit <= 0 {
		limit = 12
	}
	if offset < 0 {
		offset = 0
	}

	args := []any{}
	clauses := []string{"p.status = 'published'"}
	if categorySlug != "" {
		clauses = append(clauses, "c.slug = ?")
		args = append(args, categorySlug)
	}
	if tagSlug != "" {
		clauses = append(clauses, "t.slug = ?")
		args = append(args, tagSlug)
	}
	args = append(args, limit, offset)

	query := `
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
LEFT JOIN post_tags pt ON pt.post_id = p.id
LEFT JOIN tags t ON t.id = pt.tag_id
LEFT JOIN (
	SELECT post_id, COUNT(*) AS views
	FROM page_views
	GROUP BY post_id
) v ON v.post_id = p.id
LEFT JOIN (
	SELECT post_id, COUNT(*) AS likes
	FROM post_likes
	GROUP BY post_id
) l ON l.post_id = p.id
WHERE ` + strings.Join(clauses, " AND ") + `
GROUP BY p.id
ORDER BY datetime(p.published_at) DESC, p.id DESC
LIMIT ? OFFSET ?`

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *Store) SearchPublishedPosts(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	return s.SearchPublishedPostsKeyword(ctx, query, limit)
}

func (s *Store) GetPublishedPost(ctx context.Context, slug string, visitorID string) (PostDetail, error) {
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
	COALESCE(l.likes, 0),
	EXISTS(SELECT 1 FROM post_likes pl WHERE pl.post_id = p.id AND pl.visitor_id = ?)
FROM posts p
LEFT JOIN categories c ON c.id = p.category_id
LEFT JOIN (SELECT post_id, COUNT(*) AS views FROM page_views GROUP BY post_id) v ON v.post_id = p.id
LEFT JOIN (SELECT post_id, COUNT(*) AS likes FROM post_likes GROUP BY post_id) l ON l.post_id = p.id
WHERE p.slug = ? AND p.status = 'published'
`, visitorID, slug).Scan(
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
		&post.LikedByVisitor,
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
	post.RenderedHTML = stripLeadingArticleTitle(post.RenderedHTML)
	post.PublishedAt = parseDBTime(publishedAt)
	post.UpdatedAt = parseDBTime(updatedAt)
	withTags, err := s.attachTags(ctx, []PostSummary{post.PostSummary})
	if err != nil {
		return PostDetail{}, err
	}
	post.PostSummary = withTags[0]
	return post, nil
}

func (s *Store) ListComments(ctx context.Context, postID int64, visitorID string) ([]*Comment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
	c.id,
	c.post_id,
	c.parent_id,
	c.visitor_id,
	COALESCE(
		CASE
			WHEN TRIM(c.author_name) IN ('', '匿名访客', '匿名读者', '匿名身份') THEN NULL
			ELSE c.author_name
		END,
		NULLIF(v.display_name, ''),
		'匿名身份'
	),
	c.email,
	c.content,
	c.status,
	c.ai_review_status,
	c.ai_review_reason,
	COALESCE(l.likes, 0),
	EXISTS(SELECT 1 FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.visitor_id = ?),
	c.created_at
FROM comments c
LEFT JOIN visitors v ON v.visitor_id = c.visitor_id
LEFT JOIN (
	SELECT comment_id, COUNT(*) AS likes
	FROM comment_likes
	GROUP BY comment_id
) l ON l.comment_id = c.id
WHERE c.post_id = ? AND c.status = 'approved'
ORDER BY datetime(c.created_at) ASC
`, visitorID, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := map[int64]*Comment{}
	root := []*Comment{}
	for rows.Next() {
		comment := &Comment{}
		var parentID sql.NullInt64
		var createdAt sql.NullString
		if err := rows.Scan(
			&comment.ID,
			&comment.PostID,
			&parentID,
			&comment.VisitorID,
			&comment.AuthorName,
			&comment.Email,
			&comment.Content,
			&comment.Status,
			&comment.AIReviewStatus,
			&comment.AIReviewReason,
			&comment.Likes,
			&comment.LikedByVisitor,
			&createdAt,
		); err != nil {
			return nil, err
		}
		if parentID.Valid {
			comment.ParentID = &parentID.Int64
		}
		comment.CreatedAt = parseDBTime(createdAt)
		byID[comment.ID] = comment
		if comment.ParentID == nil {
			root = append(root, comment)
			continue
		}
		if parent, ok := byID[*comment.ParentID]; ok {
			parent.Replies = append(parent.Replies, comment)
		} else {
			root = append(root, comment)
		}
	}
	return root, rows.Err()
}

func (s *Store) CreateComment(ctx context.Context, input CommentInput, cooldownSeconds int, reviewMode string) (Comment, error) {
	input.AuthorName = strings.TrimSpace(input.AuthorName)
	if input.AuthorName == "" {
		resolved, err := s.ResolveVisitorDisplayName(ctx, input.VisitorID, "")
		if err != nil {
			return Comment{}, err
		}
		input.AuthorName = resolved
	}
	input.AuthorName = sanitizeCommentAuthor(input.AuthorName)
	input.Email = sanitizeContactEmail(input.Email)
	input.Content = sanitizeCommentContent(input.Content)
	if input.Content == "" {
		return Comment{}, ErrInvalidInput
	}

	ipHash := hashIP(input.IP)
	var recentCount int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM comments
WHERE post_id = ? AND visitor_id = ? AND ip_hash = ? AND datetime(created_at) >= datetime('now', ?)
`, input.PostID, input.VisitorID, ipHash, "-"+intToString(cooldownSeconds)+" seconds").Scan(&recentCount); err != nil {
		return Comment{}, err
	}
	if recentCount > 0 {
		return Comment{}, ErrRateLimited
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Comment{}, err
	}
	defer tx.Rollback()

	decision, err := s.reviewer.ReviewComment(ctx, ai.Input{
		PostTitle: input.PostTitle,
		Content:   input.Content,
		VisitorID: input.VisitorID,
	})
	if err != nil {
		decision = ai.Decision{Status: "pending", Reason: "reviewer-error"}
	}
	status := resolveCommentStatus(reviewMode, decision)

	res, err := tx.ExecContext(ctx, `
INSERT INTO comments (
	post_id, parent_id, visitor_id, author_name, email, content, status,
	ai_review_status, ai_review_reason, ip_hash
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, input.PostID, nullableInt64(input.ParentID), input.VisitorID, input.AuthorName, input.Email, input.Content, status, decision.Status, decision.Reason, ipHash)
	if err != nil {
		return Comment{}, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return Comment{}, err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO comment_notifications (comment_id, delivery_status, error_message)
VALUES (?, 'queued', '')
`, id); err != nil {
		return Comment{}, err
	}

	if err := tx.Commit(); err != nil {
		return Comment{}, err
	}

	_ = s.UpdateVisitorDisplayName(ctx, input.VisitorID, input.AuthorName)
	if input.Email != "" {
		_ = s.UpdateVisitorContactEmail(ctx, input.VisitorID, input.Email)
	}

	return Comment{
		ID:             id,
		PostID:         input.PostID,
		ParentID:       input.ParentID,
		VisitorID:      input.VisitorID,
		AuthorName:     input.AuthorName,
		Email:          input.Email,
		Content:        input.Content,
		Status:         status,
		AIReviewStatus: decision.Status,
		AIReviewReason: decision.Reason,
		NotifyStatus:   "queued",
		CreatedAt:      time.Now(),
	}, nil
}

func (s *Store) MarkCommentNotification(ctx context.Context, commentID int64, status string, errorMessage string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE comment_notifications
SET delivery_status = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
WHERE comment_id = ?
`, status, errorMessage, commentID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected > 0 {
		return nil
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO comment_notifications (comment_id, delivery_status, error_message)
VALUES (?, ?, ?)
`, commentID, status, errorMessage)
	return err
}

func (s *Store) LikePost(ctx context.Context, slug string, visitorID string) (LikeResult, error) {
	if visitorID == "" {
		return LikeResult{}, ErrNotFound
	}
	var postID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM posts WHERE slug = ?`, slug).Scan(&postID); err != nil {
		return LikeResult{}, err
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO post_likes (post_id, visitor_id) VALUES (?, ?)`, postID, visitorID)
	if err != nil {
		return LikeResult{}, err
	}
	return s.postLikeResult(ctx, postID, visitorID)
}

func (s *Store) LikeComment(ctx context.Context, commentID int64, visitorID string) (LikeResult, error) {
	if visitorID == "" {
		return LikeResult{}, ErrNotFound
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO comment_likes (comment_id, visitor_id) VALUES (?, ?)`, commentID, visitorID); err != nil {
		return LikeResult{}, err
	}
	return s.commentLikeResult(ctx, commentID, visitorID)
}

func (s *Store) postLikeResult(ctx context.Context, postID int64, visitorID string) (LikeResult, error) {
	result := LikeResult{}
	err := s.db.QueryRowContext(ctx, `
SELECT
	(SELECT COUNT(*) FROM post_likes WHERE post_id = ?),
	EXISTS(SELECT 1 FROM post_likes WHERE post_id = ? AND visitor_id = ?)
`, postID, postID, visitorID).Scan(&result.Likes, &result.Liked)
	return result, err
}

func (s *Store) commentLikeResult(ctx context.Context, commentID int64, visitorID string) (LikeResult, error) {
	result := LikeResult{}
	err := s.db.QueryRowContext(ctx, `
SELECT
	(SELECT COUNT(*) FROM comment_likes WHERE comment_id = ?),
	EXISTS(SELECT 1 FROM comment_likes WHERE comment_id = ? AND visitor_id = ?)
`, commentID, commentID, visitorID).Scan(&result.Likes, &result.Liked)
	return result, err
}

func (s *Store) GetRecommendations(ctx context.Context, visitorID string, currentPostID int64, limit int) ([]Recommendation, error) {
	if limit <= 0 {
		limit = 4
	}
	rows, err := s.db.QueryContext(ctx, `
WITH recent_tags AS (
	SELECT pt.tag_id, COUNT(*) AS weight
	FROM page_views pv
	JOIN post_tags pt ON pt.post_id = pv.post_id
	WHERE pv.visitor_id = ? AND datetime(pv.created_at) >= datetime('now', '-30 day')
	GROUP BY pt.tag_id
	ORDER BY weight DESC
	LIMIT 5
),
views AS (
	SELECT post_id, COUNT(*) AS view_count
	FROM page_views
	GROUP BY post_id
)
SELECT
	p.title,
	p.slug,
	COALESCE(c.name, ''),
	p.cover_image,
	CASE
		WHEN EXISTS(SELECT 1 FROM recent_tags) THEN '基于最近阅读偏好'
		ELSE '最近热门内容'
	END AS reason
FROM posts p
LEFT JOIN categories c ON c.id = p.category_id
LEFT JOIN post_tags pt ON pt.post_id = p.id
LEFT JOIN recent_tags rt ON rt.tag_id = pt.tag_id
LEFT JOIN views v ON v.post_id = p.id
WHERE p.status = 'published' AND (? = 0 OR p.id <> ?)
GROUP BY p.id
ORDER BY SUM(COALESCE(rt.weight, 0)) DESC, COALESCE(v.view_count, 0) DESC, datetime(p.published_at) DESC
LIMIT ?
`, visitorID, currentPostID, currentPostID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []Recommendation{}
	for rows.Next() {
		var item Recommendation
		if err := rows.Scan(&item.Title, &item.Slug, &item.Category, &item.CoverImage, &item.Reason); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func intToString(value int) string {
	return strconv.Itoa(value)
}
