package store

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/snemc/snemc-blog/internal/render"
)

var semanticVectorDimensionPattern = regexp.MustCompile(`float\[(\d+)\]`)

func (s *Store) ListPublishedPostIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id
FROM posts
WHERE status = 'published'
ORDER BY datetime(published_at) DESC, id DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) GetSemanticPostSource(ctx context.Context, postID int64) (SemanticPostSource, error) {
	source := SemanticPostSource{}
	err := s.db.QueryRowContext(ctx, `
SELECT
	p.id,
	p.status,
	p.title,
	p.summary,
	COALESCE(c.name, ''),
	p.rendered_html
FROM posts p
LEFT JOIN categories c ON c.id = p.category_id
WHERE p.id = ?
`, postID).Scan(
		&source.PostID,
		&source.Status,
		&source.Title,
		&source.Summary,
		&source.CategoryName,
		&source.RenderedHTML,
	)
	if errorsIsNoRows(err) {
		return SemanticPostSource{}, ErrNotFound
	}
	if err != nil {
		return SemanticPostSource{}, err
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT t.name
FROM post_tags pt
JOIN tags t ON t.id = pt.tag_id
WHERE pt.post_id = ?
ORDER BY t.name ASC
`, postID)
	if err != nil {
		return SemanticPostSource{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return SemanticPostSource{}, err
		}
		source.Tags = append(source.Tags, name)
	}
	if err := rows.Err(); err != nil {
		return SemanticPostSource{}, err
	}

	return source, nil
}

func BuildSemanticSourceText(source SemanticPostSource) string {
	parts := []string{}
	if strings.TrimSpace(source.Title) != "" {
		parts = append(parts, "标题: "+strings.TrimSpace(source.Title))
	}
	if strings.TrimSpace(source.Summary) != "" {
		parts = append(parts, "摘要: "+strings.TrimSpace(source.Summary))
	}
	if strings.TrimSpace(source.CategoryName) != "" {
		parts = append(parts, "分类: "+strings.TrimSpace(source.CategoryName))
	}
	if len(source.Tags) > 0 {
		parts = append(parts, "标签: "+strings.Join(source.Tags, "、"))
	}
	plain := strings.TrimSpace(render.PlainTextHTML(source.RenderedHTML))
	if plain != "" {
		parts = append(parts, "正文: "+plain)
	}
	return strings.Join(parts, "\n")
}

func (s *Store) GetSemanticIndexRecord(ctx context.Context, postID int64) (SemanticIndexRecord, error) {
	record := SemanticIndexRecord{}
	var updatedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT
	post_id,
	embedding_model,
	embedding_dimensions,
	content_hash,
	source_text,
	status,
	error_message,
	updated_at
FROM post_semantic_index
WHERE post_id = ?
`, postID).Scan(
		&record.PostID,
		&record.EmbeddingModel,
		&record.EmbeddingDimensions,
		&record.ContentHash,
		&record.SourceText,
		&record.Status,
		&record.ErrorMessage,
		&updatedAt,
	)
	if errorsIsNoRows(err) {
		return SemanticIndexRecord{}, ErrNotFound
	}
	if err != nil {
		return SemanticIndexRecord{}, err
	}
	record.UpdatedAt = parseDBTime(updatedAt)
	return record, nil
}

func (s *Store) CountReadySemanticIndexes(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM post_semantic_index psi
JOIN posts p ON p.id = psi.post_id
WHERE psi.status = 'ready' AND p.status = 'published'
`).Scan(&count)
	return count, err
}

func (s *Store) UpsertSemanticIndexRecord(ctx context.Context, record SemanticIndexRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO post_semantic_index (
	post_id, embedding_model, embedding_dimensions, content_hash, source_text, status, error_message, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(post_id) DO UPDATE SET
	embedding_model = excluded.embedding_model,
	embedding_dimensions = excluded.embedding_dimensions,
	content_hash = excluded.content_hash,
	source_text = excluded.source_text,
	status = excluded.status,
	error_message = excluded.error_message,
	updated_at = CURRENT_TIMESTAMP
`, record.PostID, record.EmbeddingModel, record.EmbeddingDimensions, record.ContentHash, record.SourceText, record.Status, record.ErrorMessage)
	return err
}

func (s *Store) DeleteSemanticIndex(ctx context.Context, postID int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM post_semantic_index WHERE post_id = ?`, postID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM post_semantic_vec WHERE rowid = ?`, postID); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return err
	}
	return nil
}

func (s *Store) DeleteSemanticIndexTx(ctx context.Context, tx *sql.Tx, postID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM post_semantic_index WHERE post_id = ?`, postID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM post_semantic_vec WHERE rowid = ?`, postID); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return err
	}
	return nil
}

func (s *Store) UpsertSemanticVector(ctx context.Context, postID int64, embeddingJSON string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM post_semantic_vec WHERE rowid = ?`, postID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO post_semantic_vec(rowid, embedding) VALUES (?, ?)`, postID, embeddingJSON); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) EnsureSemanticVectorTable(ctx context.Context, dimensions int) (bool, error) {
	if dimensions <= 0 {
		return false, ErrInvalidInput
	}
	current, err := s.semanticVectorTableDimensions(ctx)
	if err != nil {
		return false, err
	}
	if current == dimensions {
		return false, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	if current > 0 {
		if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS post_semantic_vec`); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM post_semantic_index`); err != nil {
			return false, err
		}
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
CREATE VIRTUAL TABLE IF NOT EXISTS post_semantic_vec USING vec0(
	embedding float[%d]
)
`, dimensions)); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return current > 0 && current != dimensions, nil
}

func (s *Store) SemanticVectorDimensions(ctx context.Context) (int, error) {
	return s.semanticVectorTableDimensions(ctx)
}

func (s *Store) semanticVectorTableDimensions(ctx context.Context) (int, error) {
	var createSQL sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT sql
FROM sqlite_master
WHERE type = 'table' AND name = 'post_semantic_vec'
`).Scan(&createSQL)
	if errorsIsNoRows(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	match := semanticVectorDimensionPattern.FindStringSubmatch(strings.ToLower(createSQL.String))
	if len(match) != 2 {
		return 0, nil
	}
	return intMust(match[1]), nil
}

func (s *Store) SearchPublishedPostsKeyword(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 8
	}
	if query == "" {
		return []SearchResult{}, nil
	}
	pattern := "%" + escapeLikePattern(query) + "%"

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
WHERE p.status = 'published'
  AND (
	p.title LIKE ? ESCAPE '\'
	OR p.summary LIKE ? ESCAPE '\'
	OR p.excerpt LIKE ? ESCAPE '\'
	OR COALESCE(c.name, '') LIKE ? ESCAPE '\'
	OR EXISTS (
		SELECT 1
		FROM post_tags pt2
		JOIN tags t2 ON t2.id = pt2.tag_id
		WHERE pt2.post_id = p.id AND t2.name LIKE ? ESCAPE '\'
	)
  )
ORDER BY
  CASE
	WHEN p.title LIKE ? ESCAPE '\' THEN 0
	WHEN EXISTS (
		SELECT 1
		FROM post_tags pt3
		JOIN tags t3 ON t3.id = pt3.tag_id
		WHERE pt3.post_id = p.id AND t3.name LIKE ? ESCAPE '\'
	) THEN 1
	WHEN COALESCE(c.name, '') LIKE ? ESCAPE '\' THEN 2
	WHEN p.summary LIKE ? ESCAPE '\' THEN 3
	WHEN p.excerpt LIKE ? ESCAPE '\' THEN 4
	ELSE 5
  END,
  datetime(p.published_at) DESC,
  p.id DESC
LIMIT ?
`, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []SearchResult{}
	postSummaries := []PostSummary{}
	for rows.Next() {
		var result SearchResult
		var publishedAt sql.NullString
		var updatedAt sql.NullString
		if err := rows.Scan(
			&result.ID,
			&result.Title,
			&result.Slug,
			&result.Summary,
			&result.Excerpt,
			&result.CoverImage,
			&result.Status,
			&result.CategoryID,
			&result.CategoryName,
			&result.CategorySlug,
			&result.WordCount,
			&result.ReadingTime,
			&publishedAt,
			&updatedAt,
			&result.Views,
			&result.Likes,
		); err != nil {
			return nil, err
		}
		result.PublishedAt = parseDBTime(publishedAt)
		result.UpdatedAt = parseDBTime(updatedAt)
		results = append(results, result)
		postSummaries = append(postSummaries, result.PostSummary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	withTags, err := s.attachTags(ctx, postSummaries)
	if err != nil {
		return nil, err
	}
	for i := range results {
		results[i].PostSummary = withTags[i]
		results[i].Snippet = buildKeywordSnippet(query, results[i].PostSummary)
	}
	return results, nil
}

func (s *Store) SearchPublishedPostsSemantic(ctx context.Context, query string, embeddingJSON string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 8
	}
	if strings.TrimSpace(embeddingJSON) == "" {
		return []SearchResult{}, nil
	}
	dimensions, err := s.semanticVectorTableDimensions(ctx)
	if err != nil {
		return nil, err
	}
	if dimensions == 0 {
		return nil, ErrSemanticSearchUnavailable
	}

	rows, err := s.db.QueryContext(ctx, `
WITH matches AS (
	SELECT rowid, distance
	FROM post_semantic_vec
	WHERE embedding MATCH ?
	ORDER BY distance ASC
	LIMIT ?
)
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
	COALESCE(vs.views, 0),
	COALESCE(ls.likes, 0),
	matches.distance
FROM matches
JOIN posts p ON p.id = matches.rowid
JOIN post_semantic_index psi ON psi.post_id = p.id
LEFT JOIN categories c ON c.id = p.category_id
LEFT JOIN (
	SELECT post_id, COUNT(*) AS views
	FROM page_views
	GROUP BY post_id
) vs ON vs.post_id = p.id
LEFT JOIN (
	SELECT post_id, COUNT(*) AS likes
	FROM post_likes
	GROUP BY post_id
) ls ON ls.post_id = p.id
WHERE p.status = 'published'
  AND psi.status = 'ready'
ORDER BY matches.distance ASC
`, embeddingJSON, limit)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, ErrSemanticSearchUnavailable
		}
		return nil, err
	}
	defer rows.Close()

	results := []SearchResult{}
	postSummaries := []PostSummary{}
	for rows.Next() {
		var result SearchResult
		var publishedAt sql.NullString
		var updatedAt sql.NullString
		if err := rows.Scan(
			&result.ID,
			&result.Title,
			&result.Slug,
			&result.Summary,
			&result.Excerpt,
			&result.CoverImage,
			&result.Status,
			&result.CategoryID,
			&result.CategoryName,
			&result.CategorySlug,
			&result.WordCount,
			&result.ReadingTime,
			&publishedAt,
			&updatedAt,
			&result.Views,
			&result.Likes,
			&result.Distance,
		); err != nil {
			return nil, err
		}
		result.PublishedAt = parseDBTime(publishedAt)
		result.UpdatedAt = parseDBTime(updatedAt)
		results = append(results, result)
		postSummaries = append(postSummaries, result.PostSummary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	withTags, err := s.attachTags(ctx, postSummaries)
	if err != nil {
		return nil, err
	}
	for i := range results {
		results[i].PostSummary = withTags[i]
		results[i].Snippet = buildSemanticSnippet(query, results[i].Summary, results[i].Excerpt)
	}
	return results, nil
}

func buildSemanticSnippet(query string, summary string, excerpt string) string {
	base := strings.TrimSpace(summary)
	if base == "" {
		base = strings.TrimSpace(excerpt)
	}
	if base == "" {
		return "基于标题、摘要和正文语义相似度匹配。"
	}
	return highlightSnippet(base, query, 120)
}

func buildKeywordSnippet(query string, post PostSummary) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return buildSemanticSnippet("", post.Summary, post.Excerpt)
	}

	if containsFold(post.Summary, query) {
		return highlightSnippet(post.Summary, query, 120)
	}
	if containsFold(post.Excerpt, query) {
		return highlightSnippet(post.Excerpt, query, 120)
	}
	if containsFold(post.Title, query) {
		return "标题匹配：" + highlightSnippet(post.Title, query, 80)
	}
	if containsFold(post.CategoryName, query) {
		return "分类匹配：" + highlightSnippet(post.CategoryName, query, 48)
	}

	matchedTags := []string{}
	for _, tag := range post.Tags {
		if containsFold(tag.Name, query) {
			matchedTags = append(matchedTags, "#"+tag.Name)
		}
	}
	if len(matchedTags) > 0 {
		escapedTags := make([]string, 0, len(matchedTags))
		for _, tag := range matchedTags {
			escapedTags = append(escapedTags, html.EscapeString(tag))
		}
		return "标签匹配：" + strings.Join(escapedTags, " ")
	}

	return buildSemanticSnippet(query, post.Summary, post.Excerpt)
}

func containsFold(text string, query string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(query))
}

func escapeLikePattern(input string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return replacer.Replace(input)
}

func highlightSnippet(text string, query string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if limit <= 0 {
		limit = 120
	}
	runes := []rune(text)
	if len(runes) > limit {
		text = string(runes[:limit]) + "..."
	}
	escaped := html.EscapeString(text)
	query = strings.TrimSpace(query)
	if query == "" {
		return escaped
	}

	lowerEscaped := strings.ToLower(escaped)
	lowerQuery := strings.ToLower(html.EscapeString(query))
	index := strings.Index(lowerEscaped, lowerQuery)
	if index < 0 {
		return escaped
	}
	end := index + len(lowerQuery)
	return escaped[:index] + "<mark>" + escaped[index:end] + "</mark>" + escaped[end:]
}

func intMust(input string) int {
	var value int
	for _, r := range input {
		value = value*10 + int(r-'0')
	}
	return value
}
