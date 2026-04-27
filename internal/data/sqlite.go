package data

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path))
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;
PRAGMA temp_store = MEMORY;
PRAGMA mmap_size = 268435456;
`); err != nil {
		return nil, err
	}

	if err := migrate(db); err != nil {
		return nil, err
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			slug TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			slug TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			summary TEXT NOT NULL DEFAULT '',
			markdown_source TEXT NOT NULL,
			rendered_html TEXT NOT NULL,
			toc_json TEXT NOT NULL DEFAULT '[]',
			excerpt TEXT NOT NULL DEFAULT '',
			cover_image TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'draft',
			category_id INTEGER,
			word_count INTEGER NOT NULL DEFAULT 0,
			reading_time INTEGER NOT NULL DEFAULT 1,
			render_version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			published_at TEXT,
			FOREIGN KEY(category_id) REFERENCES categories(id) ON DELETE SET NULL
		);`,
		`CREATE TABLE IF NOT EXISTS post_tags (
			post_id INTEGER NOT NULL,
			tag_id INTEGER NOT NULL,
			PRIMARY KEY (post_id, tag_id),
			FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE CASCADE,
			FOREIGN KEY(tag_id) REFERENCES tags(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			post_id INTEGER NOT NULL,
			parent_id INTEGER,
			visitor_id TEXT NOT NULL,
			ip_hash TEXT NOT NULL DEFAULT '',
			author_name TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			ai_review_status TEXT NOT NULL DEFAULT 'pending',
			ai_review_reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE CASCADE,
			FOREIGN KEY(parent_id) REFERENCES comments(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS post_likes (
			post_id INTEGER NOT NULL,
			visitor_id TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (post_id, visitor_id),
			FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS comment_likes (
			comment_id INTEGER NOT NULL,
			visitor_id TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (comment_id, visitor_id),
			FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS visitors (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				visitor_id TEXT NOT NULL UNIQUE,
				fingerprint TEXT NOT NULL DEFAULT '',
				user_agent TEXT NOT NULL DEFAULT '',
				language TEXT NOT NULL DEFAULT '',
				contact_email TEXT NOT NULL DEFAULT '',
				last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
			);`,
		`CREATE TABLE IF NOT EXISTS page_views (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			post_id INTEGER,
			visitor_id TEXT NOT NULL,
			path TEXT NOT NULL,
			referrer TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE SET NULL
		);`,
		`CREATE TABLE IF NOT EXISTS search_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			visitor_id TEXT NOT NULL,
			query TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS admin_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_login_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS comment_notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			comment_id INTEGER NOT NULL,
			delivery_status TEXT NOT NULL DEFAULT 'queued',
			error_message TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS app_settings (
				id INTEGER PRIMARY KEY CHECK (id = 1),
				smtp_host TEXT NOT NULL DEFAULT '',
				smtp_port TEXT NOT NULL DEFAULT '587',
				smtp_username TEXT NOT NULL DEFAULT '',
				smtp_password TEXT NOT NULL DEFAULT '',
				smtp_from TEXT NOT NULL DEFAULT '',
				admin_notify_email TEXT NOT NULL DEFAULT '',
				llm_base_url TEXT NOT NULL DEFAULT '',
				llm_api_key TEXT NOT NULL DEFAULT '',
				llm_model TEXT NOT NULL DEFAULT '',
				llm_system_prompt TEXT NOT NULL DEFAULT '',
				embedding_base_url TEXT NOT NULL DEFAULT '',
				embedding_api_key TEXT NOT NULL DEFAULT '',
				embedding_model TEXT NOT NULL DEFAULT '',
				embedding_dimensions INTEGER NOT NULL DEFAULT 0,
				embedding_timeout_ms INTEGER NOT NULL DEFAULT 15000,
				semantic_search_enabled INTEGER NOT NULL DEFAULT 0,
				comment_review_mode TEXT NOT NULL DEFAULT 'manual_all',
				about_name TEXT NOT NULL DEFAULT '',
				about_tagline TEXT NOT NULL DEFAULT '',
				about_avatar_url TEXT NOT NULL DEFAULT '',
				about_email TEXT NOT NULL DEFAULT '',
				about_github_url TEXT NOT NULL DEFAULT '',
				about_bio TEXT NOT NULL DEFAULT '',
				about_repo_count TEXT NOT NULL DEFAULT '',
				about_star_count TEXT NOT NULL DEFAULT '',
				about_fork_count TEXT NOT NULL DEFAULT '',
				about_friend_links TEXT NOT NULL DEFAULT '',
				updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
			);`,
		`CREATE TABLE IF NOT EXISTS post_semantic_index (
				post_id INTEGER PRIMARY KEY,
				embedding_model TEXT NOT NULL DEFAULT '',
				embedding_dimensions INTEGER NOT NULL DEFAULT 0,
				content_hash TEXT NOT NULL DEFAULT '',
				source_text TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL DEFAULT 'pending',
				error_message TEXT NOT NULL DEFAULT '',
				updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE CASCADE
			);`,
		`CREATE TABLE IF NOT EXISTS comment_review_tokens (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				comment_id INTEGER NOT NULL,
				action TEXT NOT NULL,
				token_hash TEXT NOT NULL UNIQUE,
				expires_at TEXT NOT NULL,
				used_at TEXT,
				created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
			);`,
		`CREATE TABLE IF NOT EXISTS mention_notifications (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				comment_id INTEGER NOT NULL,
				mentioned_visitor_id TEXT NOT NULL,
				mentioned_email TEXT NOT NULL DEFAULT '',
				delivery_status TEXT NOT NULL DEFAULT 'queued',
				error_message TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(comment_id, mentioned_visitor_id),
				FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
			);`,
		`CREATE TABLE IF NOT EXISTS agent_api_keys (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL DEFAULT '',
				key_prefix TEXT NOT NULL,
				key_hash TEXT NOT NULL UNIQUE,
				created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				last_used_at TEXT,
				revoked_at TEXT
			);`,
		`CREATE TABLE IF NOT EXISTS static_sites (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				route_id TEXT NOT NULL UNIQUE,
				entry_path TEXT NOT NULL DEFAULT '',
				storage_mode TEXT NOT NULL DEFAULT 'empty',
				download_name TEXT NOT NULL DEFAULT '',
				page_title TEXT NOT NULL DEFAULT '',
				file_count INTEGER NOT NULL DEFAULT 0,
				total_size INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
			);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS post_fts USING fts5(
				post_id UNINDEXED,
				title,
			summary,
			content,
			tokenize = 'porter unicode61'
		);`,
		`CREATE INDEX IF NOT EXISTS idx_posts_status_published_at ON posts(status, published_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_comments_post_status_created_at ON comments(post_id, status, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_page_views_visitor_created_at ON page_views(visitor_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_page_views_post_created_at ON page_views(post_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_search_logs_query_created_at ON search_logs(query, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_post_semantic_index_status_updated_at ON post_semantic_index(status, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_comment_review_tokens_comment_expires_at ON comment_review_tokens(comment_id, expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_mention_notifications_comment_status ON mention_notifications(comment_id, delivery_status);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_api_keys_prefix ON agent_api_keys(key_prefix);`,
		`CREATE INDEX IF NOT EXISTS idx_static_sites_route_id ON static_sites(route_id);`,
		`CREATE INDEX IF NOT EXISTS idx_static_sites_updated_at ON static_sites(updated_at DESC);`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	optionalStatements := []string{
		`ALTER TABLE visitors ADD COLUMN display_name TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE visitors ADD COLUMN contact_email TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN embedding_base_url TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN embedding_api_key TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN embedding_model TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN embedding_dimensions INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE app_settings ADD COLUMN embedding_timeout_ms INTEGER NOT NULL DEFAULT 15000;`,
		`ALTER TABLE app_settings ADD COLUMN semantic_search_enabled INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE app_settings ADD COLUMN comment_review_mode TEXT NOT NULL DEFAULT 'manual_all';`,
		`ALTER TABLE app_settings ADD COLUMN about_name TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN about_tagline TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN about_avatar_url TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN about_email TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN about_github_url TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN about_bio TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN about_repo_count TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN about_star_count TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN about_fork_count TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE app_settings ADD COLUMN about_friend_links TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE static_sites ADD COLUMN page_title TEXT NOT NULL DEFAULT '';`,
	}
	for _, stmt := range optionalStatements {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return err
		}
	}

	return nil
}
