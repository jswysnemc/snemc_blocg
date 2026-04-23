package store

import "time"

type Tag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Category struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

type PostSummary struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	Slug         string    `json:"slug"`
	Summary      string    `json:"summary"`
	Excerpt      string    `json:"excerpt"`
	CoverImage   string    `json:"cover_image"`
	Status       string    `json:"status"`
	CategoryID   int64     `json:"category_id"`
	CategoryName string    `json:"category_name"`
	CategorySlug string    `json:"category_slug"`
	WordCount    int       `json:"word_count"`
	ReadingTime  int       `json:"reading_time"`
	PublishedAt  time.Time `json:"published_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Views        int       `json:"views"`
	Likes        int       `json:"likes"`
	Tags         []Tag     `json:"tags"`
}

type PostDetail struct {
	PostSummary
	MarkdownSource string `json:"markdown_source"`
	RenderedHTML   string `json:"rendered_html"`
	TOCJSON        string `json:"toc_json"`
	LikedByVisitor bool   `json:"liked_by_visitor"`
}

type PostInput struct {
	ID           int64    `json:"id"`
	Title        string   `json:"title"`
	Slug         string   `json:"slug"`
	Summary      string   `json:"summary"`
	Markdown     string   `json:"markdown"`
	CoverImage   string   `json:"cover_image"`
	Status       string   `json:"status"`
	CategoryName string   `json:"category_name"`
	Tags         []string `json:"tags"`
}

type Comment struct {
	ID             int64      `json:"id"`
	PostID         int64      `json:"post_id"`
	ParentID       *int64     `json:"parent_id,omitempty"`
	PostTitle      string     `json:"post_title,omitempty"`
	PostSlug       string     `json:"post_slug,omitempty"`
	VisitorID      string     `json:"visitor_id"`
	AuthorName     string     `json:"author_name"`
	Email          string     `json:"email"`
	Content        string     `json:"content"`
	Status         string     `json:"status"`
	AIReviewStatus string     `json:"ai_review_status"`
	AIReviewReason string     `json:"ai_review_reason"`
	NotifyStatus   string     `json:"notify_status"`
	NotifyError    string     `json:"notify_error"`
	Likes          int        `json:"likes"`
	LikedByVisitor bool       `json:"liked_by_visitor"`
	CreatedAt      time.Time  `json:"created_at"`
	Replies        []*Comment `json:"replies,omitempty"`
}

type CommentInput struct {
	PostID     int64  `json:"post_id"`
	ParentID   *int64 `json:"parent_id"`
	VisitorID  string `json:"visitor_id"`
	AuthorName string `json:"author_name"`
	Email      string `json:"email"`
	Content    string `json:"content"`
	IP         string `json:"ip"`
	PostTitle  string `json:"post_title"`
}

type VisitorInput struct {
	VisitorID   string `json:"visitor_id"`
	Fingerprint string `json:"fingerprint"`
	UserAgent   string `json:"user_agent"`
	Language    string `json:"language"`
}

type VisitorProfile struct {
	VisitorID    string `json:"visitor_id"`
	DisplayName  string `json:"display_name"`
	ContactEmail string `json:"contact_email,omitempty"`
}

type DashboardStats struct {
	PublishedPosts  int `json:"published_posts"`
	DraftPosts      int `json:"draft_posts"`
	PendingComments int `json:"pending_comments"`
	TotalViews      int `json:"total_views"`
	ActiveVisitors  int `json:"active_visitors"`
	Searches7d      int `json:"searches_7d"`
}

type Recommendation struct {
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	Category   string `json:"category"`
	Reason     string `json:"reason"`
	CoverImage string `json:"cover_image"`
}

type SearchResult struct {
	PostSummary
	Snippet  string  `json:"snippet"`
	Distance float64 `json:"distance,omitempty"`
}

type LikeResult struct {
	Likes int  `json:"likes"`
	Liked bool `json:"liked"`
}

type TaxonomyBundle struct {
	Categories []Category `json:"categories"`
	Tags       []Tag      `json:"tags"`
}

type AdminUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type AppSettings struct {
	SMTPHost              string `json:"smtp_host"`
	SMTPPort              string `json:"smtp_port"`
	SMTPUsername          string `json:"smtp_username"`
	SMTPPassword          string `json:"smtp_password"`
	SMTPFrom              string `json:"smtp_from"`
	AdminNotifyEmail      string `json:"admin_notify_email"`
	LLMBaseURL            string `json:"llm_base_url"`
	LLMAPIKey             string `json:"llm_api_key"`
	LLMModel              string `json:"llm_model"`
	LLMSystemPrompt       string `json:"llm_system_prompt"`
	CommentReviewMode     string `json:"comment_review_mode"`
	EmbeddingBaseURL      string `json:"embedding_base_url"`
	EmbeddingAPIKey       string `json:"embedding_api_key"`
	EmbeddingModel        string `json:"embedding_model"`
	EmbeddingDimensions   int    `json:"embedding_dimensions"`
	EmbeddingTimeoutMS    int    `json:"embedding_timeout_ms"`
	SemanticSearchEnabled bool   `json:"semantic_search_enabled"`
	AboutName             string `json:"about_name"`
	AboutTagline          string `json:"about_tagline"`
	AboutAvatarURL        string `json:"about_avatar_url"`
	AboutEmail            string `json:"about_email"`
	AboutGitHubURL        string `json:"about_github_url"`
	AboutBio              string `json:"about_bio"`
	AboutRepoCount        string `json:"about_repo_count"`
	AboutStarCount        string `json:"about_star_count"`
	AboutForkCount        string `json:"about_fork_count"`
	AboutFriendLinks      string `json:"about_friend_links"`
}

type AgentAPIKey struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

type MentionTarget struct {
	VisitorID   string `json:"visitor_id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type SemanticPostSource struct {
	PostID       int64
	Status       string
	Title        string
	Summary      string
	CategoryName string
	RenderedHTML string
	Tags         []string
}

type SemanticIndexRecord struct {
	PostID              int64
	EmbeddingModel      string
	EmbeddingDimensions int
	ContentHash         string
	SourceText          string
	Status              string
	ErrorMessage        string
	UpdatedAt           time.Time
}
