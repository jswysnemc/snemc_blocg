package server

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/snemc/snemc-blog/internal/email"
	"github.com/snemc/snemc-blog/internal/store"
)

func (a *App) queueAdminCommentEmail(post store.PostDetail, comment store.Comment, to string) {
	to = strings.TrimSpace(to)
	if to == "" {
		_ = a.store.MarkCommentNotification(context.Background(), comment.ID, "skipped", "admin notify email not configured")
		return
	}

	approveToken, err := a.store.CreateCommentReviewToken(context.Background(), comment.ID, "approved", 72*time.Hour)
	if err != nil {
		_ = a.store.MarkCommentNotification(context.Background(), comment.ID, "failed", err.Error())
		return
	}
	rejectToken, err := a.store.CreateCommentReviewToken(context.Background(), comment.ID, "rejected", 72*time.Hour)
	if err != nil {
		_ = a.store.MarkCommentNotification(context.Background(), comment.ID, "failed", err.Error())
		return
	}

	mail := a.buildAdminCommentMail(post, comment, to, approveToken, rejectToken)
	mail.Done = func(err error) {
		status := "sent"
		errorMessage := ""
		if err != nil {
			status = "failed"
			errorMessage = err.Error()
		}
		_ = a.store.MarkCommentNotification(context.Background(), comment.ID, status, errorMessage)
	}
	a.mailer.Enqueue(mail)
}

func (a *App) queueMentionNotifications(comment store.Comment) {
	fullComment := comment
	if fullComment.PostSlug == "" || fullComment.PostTitle == "" {
		loaded, err := a.store.GetCommentByID(context.Background(), comment.ID)
		if err != nil {
			return
		}
		fullComment = loaded
	}

	targets, err := a.store.ResolveMentionTargets(context.Background(), fullComment)
	if err != nil || len(targets) == 0 {
		return
	}

	postURL := strings.TrimRight(a.cfg.SiteURL, "/") + "/posts/" + url.PathEscape(fullComment.PostSlug)
	for _, target := range targets {
		status, err := a.store.MentionNotificationStatus(context.Background(), fullComment.ID, target.VisitorID)
		if err != nil || status == "sent" {
			continue
		}
		_ = a.store.MarkMentionNotification(context.Background(), fullComment.ID, target, "queued", "")
		targetCopy := target
		mail := a.buildMentionMail(fullComment, targetCopy, postURL)
		mail.Done = func(err error) {
			nextStatus := "sent"
			errorMessage := ""
			if err != nil {
				nextStatus = "failed"
				errorMessage = err.Error()
			}
			_ = a.store.MarkMentionNotification(context.Background(), fullComment.ID, targetCopy, nextStatus, errorMessage)
		}
		a.mailer.Enqueue(mail)
	}
}

func (a *App) handleCommentReviewAction(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		a.renderCommentReviewResult(w, http.StatusBadRequest, "链接无效", "审核链接缺少 token。", "", "")
		return
	}

	comment, err := a.store.ApplyCommentReviewToken(r.Context(), token)
	switch err {
	case nil:
		if comment.Status == "approved" {
			a.queueMentionNotifications(comment)
		}
		postURL := strings.TrimRight(a.cfg.SiteURL, "/") + "/posts/" + url.PathEscape(comment.PostSlug)
		a.renderCommentReviewResult(
			w,
			http.StatusOK,
			"审核已完成",
			fmt.Sprintf("评论已处理为“%s”。", humanCommentStatus(comment.Status)),
			comment.PostTitle,
			postURL,
		)
		return
	case store.ErrUsedToken:
		a.renderCommentReviewResult(w, http.StatusConflict, "链接已使用", "这个审核链接已经处理过。", "", "")
		return
	case store.ErrExpiredToken:
		a.renderCommentReviewResult(w, http.StatusGone, "链接已过期", "这个审核链接已经过期，请到后台重新处理。", "", "")
		return
	case store.ErrNotFound:
		a.renderCommentReviewResult(w, http.StatusNotFound, "链接不存在", "没有找到对应的审核请求。", "", "")
		return
	default:
		a.renderCommentReviewResult(w, http.StatusInternalServerError, "处理失败", "评论审核回调执行失败。", "", "")
		return
	}
}

func (a *App) renderCommentReviewResult(w http.ResponseWriter, status int, title string, message string, postTitle string, postURL string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	escapedTitle := template.HTMLEscapeString(title)
	escapedMessage := template.HTMLEscapeString(message)
	escapedPostTitle := template.HTMLEscapeString(postTitle)
	escapedPostURL := template.HTMLEscapeString(postURL)
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>%s</title>
  <style>
    body { margin:0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background:#f6f7fb; color:#111827; }
    .wrap { min-height:100vh; display:flex; align-items:center; justify-content:center; padding:24px; }
    .card { width:min(560px, 100%%); background:#fff; border-radius:20px; box-shadow:0 18px 60px rgba(17,24,39,.12); padding:28px; }
    h1 { margin:0 0 12px; font-size:28px; }
    p { margin:0 0 16px; line-height:1.7; color:#4b5563; }
    a { display:inline-block; padding:10px 16px; border-radius:999px; background:#111827; color:#fff; text-decoration:none; }
    .meta { margin-top:14px; font-size:13px; color:#6b7280; }
  </style>
</head>
<body>
  <div class="wrap">
    <section class="card">
      <h1>%s</h1>
      <p>%s</p>
      %s
      %s
    </section>
  </div>
</body>
</html>`,
		escapedTitle,
		escapedTitle,
		escapedMessage,
		linkHTML(escapedPostURL, escapedPostTitle),
		metaHTML(escapedPostTitle),
	)
}

func (a *App) buildAdminCommentMail(post store.PostDetail, comment store.Comment, to string, approveToken string, rejectToken string) email.Mail {
	postURL := strings.TrimRight(a.cfg.SiteURL, "/") + "/posts/" + url.PathEscape(post.Slug)
	approveURL := strings.TrimRight(a.cfg.SiteURL, "/") + "/review/comment?token=" + url.QueryEscape(approveToken)
	rejectURL := strings.TrimRight(a.cfg.SiteURL, "/") + "/review/comment?token=" + url.QueryEscape(rejectToken)
	createdAt := comment.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	textBody := strings.Join([]string{
		"博客评论待审核",
		"",
		"文章: " + post.Title,
		"评论人: " + comment.AuthorName,
		"邮箱: " + fallbackText(comment.Email, "未填写"),
		"AI 结果: " + humanCommentStatus(comment.AIReviewStatus),
		"当前状态: " + humanCommentStatus(comment.Status),
		"时间: " + createdAt.Format("2006-01-02 15:04:05"),
		"",
		"评论内容:",
		comment.Content,
		"",
		"通过: " + approveURL,
		"拒绝: " + rejectURL,
		"原文: " + postURL,
	}, "\n")

	htmlBody := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
  <body style="margin:0;padding:24px;background:#f5f7fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#111827;">
    <div style="max-width:720px;margin:0 auto;background:#ffffff;border-radius:18px;padding:28px;box-shadow:0 16px 48px rgba(15,23,42,.12);">
      <div style="font-size:12px;letter-spacing:.08em;text-transform:uppercase;color:#6b7280;margin-bottom:12px;">AI 审核通知</div>
      <h1 style="margin:0 0 8px;font-size:28px;line-height:1.3;">新评论待处理</h1>
      <p style="margin:0 0 20px;color:#4b5563;line-height:1.7;">这封邮件包含评论摘要、AI 审核结果和直接处理按钮。</p>
      <div style="padding:16px;border:1px solid #e5e7eb;border-radius:14px;background:#fafafa;">
        <div style="margin-bottom:8px;"><strong>文章：</strong>%s</div>
        <div style="margin-bottom:8px;"><strong>评论人：</strong>%s</div>
        <div style="margin-bottom:8px;"><strong>邮箱：</strong>%s</div>
        <div style="margin-bottom:8px;"><strong>AI 结果：</strong>%s</div>
        <div style="margin-bottom:8px;"><strong>当前状态：</strong>%s</div>
        <div style="margin-bottom:8px;"><strong>提交时间：</strong>%s</div>
        <div style="margin-top:14px;padding:14px;border-radius:12px;background:#fff;border:1px solid #e5e7eb;white-space:pre-wrap;line-height:1.7;">%s</div>
      </div>
      <div style="margin-top:20px;display:flex;gap:12px;flex-wrap:wrap;">
        <a href="%s" style="display:inline-block;padding:12px 18px;border-radius:999px;background:#16a34a;color:#fff;text-decoration:none;font-weight:600;">直接通过</a>
        <a href="%s" style="display:inline-block;padding:12px 18px;border-radius:999px;background:#dc2626;color:#fff;text-decoration:none;font-weight:600;">直接拒绝</a>
        <a href="%s" style="display:inline-block;padding:12px 18px;border-radius:999px;background:#111827;color:#fff;text-decoration:none;font-weight:600;">查看文章</a>
      </div>
      <p style="margin:18px 0 0;color:#6b7280;font-size:12px;line-height:1.7;">审核按钮默认 72 小时内有效，使用后会失效。</p>
    </div>
  </body>
</html>`,
		template.HTMLEscapeString(post.Title),
		template.HTMLEscapeString(comment.AuthorName),
		template.HTMLEscapeString(fallbackText(comment.Email, "未填写")),
		template.HTMLEscapeString(humanCommentStatus(comment.AIReviewStatus)),
		template.HTMLEscapeString(humanCommentStatus(comment.Status)),
		template.HTMLEscapeString(createdAt.Format("2006-01-02 15:04:05")),
		template.HTMLEscapeString(comment.Content),
		template.HTMLEscapeString(approveURL),
		template.HTMLEscapeString(rejectURL),
		template.HTMLEscapeString(postURL),
	)

	return email.Mail{
		To:       to,
		Subject:  "新评论待审核 - " + post.Title,
		TextBody: textBody,
		HTMLBody: htmlBody,
	}
}

func (a *App) buildMentionMail(comment store.Comment, target store.MentionTarget, postURL string) email.Mail {
	subject := fmt.Sprintf("你在《%s》中被提及", comment.PostTitle)
	textBody := strings.Join([]string{
		"有人在博客评论区提到了你。",
		"",
		"文章: " + comment.PostTitle,
		"评论人: " + comment.AuthorName,
		"评论内容:",
		comment.Content,
		"",
		"查看文章: " + postURL,
	}, "\n")

	htmlBody := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
  <body style="margin:0;padding:24px;background:#f5f7fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#111827;">
    <div style="max-width:640px;margin:0 auto;background:#ffffff;border-radius:18px;padding:28px;box-shadow:0 16px 48px rgba(15,23,42,.12);">
      <div style="font-size:12px;letter-spacing:.08em;text-transform:uppercase;color:#6b7280;margin-bottom:12px;">评论提及提醒</div>
      <h1 style="margin:0 0 8px;font-size:26px;line-height:1.3;">有人提到了你</h1>
      <p style="margin:0 0 18px;color:#4b5563;line-height:1.7;">%s 在《%s》的评论区提到了 %s。</p>
      <div style="padding:14px;border-radius:12px;background:#f9fafb;border:1px solid #e5e7eb;white-space:pre-wrap;line-height:1.7;">%s</div>
      <div style="margin-top:20px;">
        <a href="%s" style="display:inline-block;padding:12px 18px;border-radius:999px;background:#111827;color:#fff;text-decoration:none;font-weight:600;">查看上下文</a>
      </div>
    </div>
  </body>
</html>`,
		template.HTMLEscapeString(comment.AuthorName),
		template.HTMLEscapeString(comment.PostTitle),
		template.HTMLEscapeString(target.DisplayName),
		template.HTMLEscapeString(comment.Content),
		template.HTMLEscapeString(postURL),
	)

	return email.Mail{
		To:       target.Email,
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
	}
}

func humanCommentStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "approved":
		return "已通过"
	case "rejected":
		return "已拒绝"
	default:
		return "待人工审核"
	}
}

func fallbackText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func linkHTML(urlValue string, label string) string {
	if strings.TrimSpace(urlValue) == "" || strings.TrimSpace(label) == "" {
		return ""
	}
	return fmt.Sprintf(`<a href="%s">查看文章：%s</a>`, urlValue, label)
}

func metaHTML(postTitle string) string {
	if strings.TrimSpace(postTitle) == "" {
		return ""
	}
	return fmt.Sprintf(`<div class="meta">文章：%s</div>`, postTitle)
}
