export interface Tag {
  id: number;
  name: string;
  slug: string;
}

export interface Category {
  id: number;
  name: string;
  slug: string;
  description: string;
}

export interface PostSummary {
  id: number;
  title: string;
  slug: string;
  summary: string;
  excerpt: string;
  cover_image: string;
  status: string;
  category_id: number;
  category_name: string;
  category_slug: string;
  word_count: number;
  reading_time: number;
  published_at: string;
  updated_at: string;
  views: number;
  likes: number;
  tags: Tag[];
}

export interface PostDetail extends PostSummary {
  markdown_source: string;
  rendered_html: string;
  toc_json: string;
  liked_by_visitor?: boolean;
}

export interface DashboardStats {
  published_posts: number;
  draft_posts: number;
  pending_comments: number;
  total_views: number;
  active_visitors: number;
  searches_7d: number;
}

export interface CommentNode {
  id: number;
  post_id: number;
  parent_id?: number | null;
  post_title?: string;
  visitor_id: string;
  author_name: string;
  email: string;
  content: string;
  status: string;
  ai_review_status: string;
  ai_review_reason: string;
  notify_status: string;
  notify_error: string;
  likes: number;
  liked_by_visitor: boolean;
  created_at: string;
  replies?: CommentNode[];
}

export interface TaxonomyBundle {
  categories: Category[];
  tags: Tag[];
}

export interface AdminUser {
  id: number;
  username: string;
}

export interface AppSettings {
  smtp_host: string;
  smtp_port: string;
  smtp_username: string;
  smtp_password: string;
  smtp_from: string;
  admin_notify_email: string;
  llm_base_url: string;
  llm_api_key: string;
  llm_model: string;
  llm_system_prompt: string;
  comment_review_mode: string;
  embedding_base_url: string;
  embedding_api_key: string;
  embedding_model: string;
  embedding_dimensions: number;
  embedding_timeout_ms: number;
  semantic_search_enabled: boolean;
  about_name: string;
  about_tagline: string;
  about_avatar_url: string;
  about_email: string;
  about_github_url: string;
  about_bio: string;
  about_repo_count: string;
  about_star_count: string;
  about_fork_count: string;
  about_friend_links: string;
}

export interface AgentAPIKey {
  id: number;
  name: string;
  key_prefix: string;
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
}
