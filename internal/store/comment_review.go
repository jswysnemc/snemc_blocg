package store

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"time"
)

var mentionNamePattern = regexp.MustCompile(`(?:^|[\s\(\[\{])@([^\s@]{1,24})`)

func (s *Store) CreateCommentReviewToken(ctx context.Context, commentID int64, action string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 72 * time.Hour
	}
	action = normalizeCommentAction(action)
	token, err := generateOpaqueSecret("crv_", 24)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO comment_review_tokens (comment_id, action, token_hash, expires_at)
VALUES (?, ?, ?, ?)
`, commentID, action, hashSecretValue(token), time.Now().Add(ttl).UTC().Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) ApplyCommentReviewToken(ctx context.Context, token string) (Comment, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Comment{}, err
	}
	defer tx.Rollback()

	var recordID int64
	var commentID int64
	var action string
	var expiresAt sql.NullString
	var usedAt sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT id, comment_id, action, expires_at, used_at
FROM comment_review_tokens
WHERE token_hash = ?
`, hashSecretValue(token)).Scan(&recordID, &commentID, &action, &expiresAt, &usedAt)
	if errorsIsNoRows(err) {
		return Comment{}, ErrNotFound
	}
	if err != nil {
		return Comment{}, err
	}

	if usedAt.Valid && strings.TrimSpace(usedAt.String) != "" {
		return Comment{}, ErrUsedToken
	}

	if parsed := parseDBTime(expiresAt); !parsed.IsZero() && parsed.Before(time.Now()) {
		return Comment{}, ErrExpiredToken
	}

	action = normalizeCommentAction(action)
	if _, err := tx.ExecContext(ctx, `
UPDATE comments
SET status = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, action, commentID); err != nil {
		return Comment{}, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE comment_review_tokens
SET used_at = CURRENT_TIMESTAMP
WHERE id = ?
`, recordID); err != nil {
		return Comment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Comment{}, err
	}
	return s.GetCommentByID(ctx, commentID)
}

func (s *Store) GetCommentByID(ctx context.Context, id int64) (Comment, error) {
	var comment Comment
	var parentID sql.NullInt64
	var createdAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT
	c.id,
	c.post_id,
	c.parent_id,
	p.title,
	p.slug,
	c.visitor_id,
	c.author_name,
	c.email,
	c.content,
	c.status,
	c.ai_review_status,
	c.ai_review_reason,
	c.created_at
FROM comments c
JOIN posts p ON p.id = c.post_id
WHERE c.id = ?
`, id).Scan(
		&comment.ID,
		&comment.PostID,
		&parentID,
		&comment.PostTitle,
		&comment.PostSlug,
		&comment.VisitorID,
		&comment.AuthorName,
		&comment.Email,
		&comment.Content,
		&comment.Status,
		&comment.AIReviewStatus,
		&comment.AIReviewReason,
		&createdAt,
	)
	if errorsIsNoRows(err) {
		return Comment{}, ErrNotFound
	}
	if err != nil {
		return Comment{}, err
	}
	if parentID.Valid {
		comment.ParentID = &parentID.Int64
	}
	comment.CreatedAt = parseDBTime(createdAt)
	return comment, nil
}

func (s *Store) ResolveMentionTargets(ctx context.Context, comment Comment) ([]MentionTarget, error) {
	names := extractMentionNames(comment.Content)
	if len(names) == 0 {
		return nil, nil
	}

	holders := make([]string, 0, len(names))
	args := make([]any, 0, len(names)+1)
	for _, name := range names {
		holders = append(holders, "?")
		args = append(args, name)
	}
	args = append(args, comment.VisitorID)

	rows, err := s.db.QueryContext(ctx, `
SELECT visitor_id, display_name, contact_email
FROM visitors
WHERE display_name IN (`+strings.Join(holders, ",")+`) AND contact_email <> '' AND visitor_id <> ?
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := []MentionTarget{}
	for rows.Next() {
		var target MentionTarget
		if err := rows.Scan(&target.VisitorID, &target.DisplayName, &target.Email); err != nil {
			return nil, err
		}
		target.Email = sanitizeContactEmail(target.Email)
		if target.Email == "" {
			continue
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func (s *Store) MarkMentionNotification(ctx context.Context, commentID int64, target MentionTarget, status string, errorMessage string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO mention_notifications (
	comment_id, mentioned_visitor_id, mentioned_email, delivery_status, error_message, updated_at
) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(comment_id, mentioned_visitor_id) DO UPDATE SET
	mentioned_email = excluded.mentioned_email,
	delivery_status = excluded.delivery_status,
	error_message = excluded.error_message,
	updated_at = CURRENT_TIMESTAMP
`, commentID, target.VisitorID, target.Email, status, errorMessage)
	return err
}

func (s *Store) MentionNotificationStatus(ctx context.Context, commentID int64, visitorID string) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `
SELECT delivery_status
FROM mention_notifications
WHERE comment_id = ? AND mentioned_visitor_id = ?
`, commentID, visitorID).Scan(&status)
	if errorsIsNoRows(err) {
		return "", nil
	}
	return status, err
}

func extractMentionNames(content string) []string {
	matches := mentionNamePattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func normalizeCommentAction(input string) string {
	if strings.TrimSpace(strings.ToLower(input)) == "approved" {
		return "approved"
	}
	return "rejected"
}
