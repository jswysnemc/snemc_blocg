package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type RuntimeSettingsOverrides struct {
	SMTPHost              *string
	SMTPPort              *string
	SMTPUsername          *string
	SMTPPassword          *string
	SMTPFrom              *string
	AdminNotifyEmail      *string
	LLMBaseURL            *string
	LLMAPIKey             *string
	LLMModel              *string
	LLMSystemPrompt       *string
	EmbeddingBaseURL      *string
	EmbeddingAPIKey       *string
	EmbeddingModel        *string
	EmbeddingDimensions   *int
	EmbeddingTimeoutMS    *int
	SemanticSearchEnabled *bool
}

type Config struct {
	ConfigPath            string
	Addr                  string
	DataDir               string
	DatabasePath          string
	MediaDir              string
	UploadsDir            string
	FrontendDistDir       string
	PublicAssetsDir       string
	SiteName              string
	SiteURL               string
	JWTSecret             string
	AdminUsername         string
	AdminPassword         string
	CommentNotifyTo       string
	SMTPHost              string
	SMTPPort              string
	SMTPUsername          string
	SMTPPassword          string
	SMTPFrom              string
	LLMBaseURL            string
	LLMAPIKey             string
	LLMModel              string
	LLMSystemPrompt       string
	EmbeddingBaseURL      string
	EmbeddingAPIKey       string
	EmbeddingModel        string
	EmbeddingDimensions   int
	EmbeddingTimeoutMS    int
	SemanticSearchEnabled bool
	CommentCooldown       int
	RecommendationN       int
	RuntimeOverrides      RuntimeSettingsOverrides
}

type fileConfig struct {
	Server    fileServerConfig    `toml:"server"`
	Site      fileSiteConfig      `toml:"site"`
	Admin     fileAdminConfig     `toml:"admin"`
	Mail      fileMailConfig      `toml:"mail"`
	LLM       fileLLMConfig       `toml:"llm"`
	Embedding fileEmbeddingConfig `toml:"embedding"`
	Features  fileFeaturesConfig  `toml:"features"`
}

type fileServerConfig struct {
	Addr            *string `toml:"addr"`
	DataDir         *string `toml:"data_dir"`
	DatabasePath    *string `toml:"database_path"`
	MediaDir        *string `toml:"media_dir"`
	UploadsDir      *string `toml:"uploads_dir"`
	FrontendDistDir *string `toml:"frontend_dist_dir"`
}

type fileSiteConfig struct {
	Name *string `toml:"name"`
	URL  *string `toml:"url"`
}

type fileAdminConfig struct {
	JWTSecret *string `toml:"jwt_secret"`
	Username  *string `toml:"username"`
	Password  *string `toml:"password"`
}

type fileMailConfig struct {
	NotifyTo *string `toml:"notify_to"`
	SMTPHost *string `toml:"smtp_host"`
	SMTPPort *string `toml:"smtp_port"`
	SMTPUser *string `toml:"smtp_username"`
	SMTPPass *string `toml:"smtp_password"`
	SMTPFrom *string `toml:"smtp_from"`
}

type fileLLMConfig struct {
	BaseURL      *string `toml:"base_url"`
	APIKey       *string `toml:"api_key"`
	Model        *string `toml:"model"`
	SystemPrompt *string `toml:"system_prompt"`
}

type fileEmbeddingConfig struct {
	Enabled    *bool   `toml:"enabled"`
	BaseURL    *string `toml:"base_url"`
	APIKey     *string `toml:"api_key"`
	Model      *string `toml:"model"`
	Dimensions *int    `toml:"dimensions"`
	TimeoutMS  *int    `toml:"timeout_ms"`
}

type fileFeaturesConfig struct {
	CommentCooldown *int `toml:"comment_cooldown"`
	RecommendationN *int `toml:"recommendation_n"`
}

func Load(root string) Config {
	cfg := Config{
		Addr:               ":8080",
		DataDir:            filepath.Join(root, "data"),
		FrontendDistDir:    filepath.Join(root, "frontend", "dist"),
		PublicAssetsDir:    filepath.Join(root, "web", "assets"),
		SiteName:           "Snemc Blocg",
		SiteURL:            "http://localhost:8080",
		JWTSecret:          "change-this-before-production",
		AdminUsername:      "admin",
		AdminPassword:      "ChangeMe123!",
		CommentNotifyTo:    "admin@example.com",
		SMTPPort:           "587",
		SMTPFrom:           "noreply@example.com",
		LLMSystemPrompt:    "You are a blog moderation assistant.",
		EmbeddingTimeoutMS: 15000,
		CommentCooldown:    60,
		RecommendationN:    4,
	}
	cfg.DatabasePath = filepath.Join(cfg.DataDir, "blog.sqlite3")
	cfg.MediaDir = filepath.Join(cfg.DataDir, "media")
	cfg.UploadsDir = filepath.Join(cfg.DataDir, "uploads")

	configPath := strings.TrimSpace(os.Getenv("BLOG_CONFIG"))
	if configPath == "" {
		configPath = filepath.Join(root, "config.toml")
	}
	cfg.ConfigPath = configPath
	cfg = applyFileConfig(cfg, configPath)
	cfg = applyEnvOverrides(cfg, root)
	return cfg
}

func applyFileConfig(cfg Config, path string) Config {
	if strings.TrimSpace(path) == "" {
		return cfg
	}
	if _, err := os.Stat(path); err != nil {
		return cfg
	}

	var file fileConfig
	if _, err := toml.DecodeFile(path, &file); err != nil {
		return cfg
	}

	if file.Server.Addr != nil {
		cfg.Addr = *file.Server.Addr
	}
	if file.Server.DataDir != nil {
		cfg.DataDir = *file.Server.DataDir
	}
	if file.Server.DatabasePath != nil {
		cfg.DatabasePath = *file.Server.DatabasePath
	} else if file.Server.DataDir != nil {
		cfg.DatabasePath = filepath.Join(cfg.DataDir, "blog.sqlite3")
	}
	if file.Server.MediaDir != nil {
		cfg.MediaDir = *file.Server.MediaDir
	} else if file.Server.DataDir != nil {
		cfg.MediaDir = filepath.Join(cfg.DataDir, "media")
	}
	if file.Server.UploadsDir != nil {
		cfg.UploadsDir = *file.Server.UploadsDir
	} else if file.Server.DataDir != nil {
		cfg.UploadsDir = filepath.Join(cfg.DataDir, "uploads")
	}
	if file.Server.FrontendDistDir != nil {
		cfg.FrontendDistDir = *file.Server.FrontendDistDir
	}
	if file.Site.Name != nil {
		cfg.SiteName = *file.Site.Name
	}
	if file.Site.URL != nil {
		cfg.SiteURL = *file.Site.URL
	}
	if file.Admin.JWTSecret != nil {
		cfg.JWTSecret = *file.Admin.JWTSecret
	}
	if file.Admin.Username != nil {
		cfg.AdminUsername = *file.Admin.Username
	}
	if file.Admin.Password != nil {
		cfg.AdminPassword = *file.Admin.Password
	}
	if file.Features.CommentCooldown != nil {
		cfg.CommentCooldown = *file.Features.CommentCooldown
	}
	if file.Features.RecommendationN != nil {
		cfg.RecommendationN = *file.Features.RecommendationN
	}

	applyRuntimeString(&cfg.SMTPHost, file.Mail.SMTPHost, &cfg.RuntimeOverrides.SMTPHost)
	applyRuntimeString(&cfg.SMTPPort, file.Mail.SMTPPort, &cfg.RuntimeOverrides.SMTPPort)
	applyRuntimeString(&cfg.SMTPUsername, file.Mail.SMTPUser, &cfg.RuntimeOverrides.SMTPUsername)
	applyRuntimeString(&cfg.SMTPPassword, file.Mail.SMTPPass, &cfg.RuntimeOverrides.SMTPPassword)
	applyRuntimeString(&cfg.SMTPFrom, file.Mail.SMTPFrom, &cfg.RuntimeOverrides.SMTPFrom)
	applyRuntimeString(&cfg.CommentNotifyTo, file.Mail.NotifyTo, &cfg.RuntimeOverrides.AdminNotifyEmail)

	applyRuntimeString(&cfg.LLMBaseURL, file.LLM.BaseURL, &cfg.RuntimeOverrides.LLMBaseURL)
	applyRuntimeString(&cfg.LLMAPIKey, file.LLM.APIKey, &cfg.RuntimeOverrides.LLMAPIKey)
	applyRuntimeString(&cfg.LLMModel, file.LLM.Model, &cfg.RuntimeOverrides.LLMModel)
	applyRuntimeString(&cfg.LLMSystemPrompt, file.LLM.SystemPrompt, &cfg.RuntimeOverrides.LLMSystemPrompt)

	applyRuntimeString(&cfg.EmbeddingBaseURL, file.Embedding.BaseURL, &cfg.RuntimeOverrides.EmbeddingBaseURL)
	applyRuntimeString(&cfg.EmbeddingAPIKey, file.Embedding.APIKey, &cfg.RuntimeOverrides.EmbeddingAPIKey)
	applyRuntimeString(&cfg.EmbeddingModel, file.Embedding.Model, &cfg.RuntimeOverrides.EmbeddingModel)
	applyRuntimeInt(&cfg.EmbeddingDimensions, file.Embedding.Dimensions, &cfg.RuntimeOverrides.EmbeddingDimensions)
	applyRuntimeInt(&cfg.EmbeddingTimeoutMS, file.Embedding.TimeoutMS, &cfg.RuntimeOverrides.EmbeddingTimeoutMS)
	applyRuntimeBool(&cfg.SemanticSearchEnabled, file.Embedding.Enabled, &cfg.RuntimeOverrides.SemanticSearchEnabled)

	return cfg
}

func applyEnvOverrides(cfg Config, root string) Config {
	if value, ok := envValue("BLOG_ADDR"); ok {
		cfg.Addr = value
	}
	if value, ok := envValue("BLOG_DATA_DIR"); ok {
		cfg.DataDir = value
	}
	if value, ok := envValue("BLOG_DB_PATH"); ok {
		cfg.DatabasePath = value
	} else if cfg.DatabasePath == "" {
		cfg.DatabasePath = filepath.Join(cfg.DataDir, "blog.sqlite3")
	}
	if value, ok := envValue("BLOG_MEDIA_DIR"); ok {
		cfg.MediaDir = value
	} else if cfg.MediaDir == "" {
		cfg.MediaDir = filepath.Join(cfg.DataDir, "media")
	}
	if value, ok := envValue("BLOG_UPLOADS_DIR"); ok {
		cfg.UploadsDir = value
	} else if cfg.UploadsDir == "" {
		cfg.UploadsDir = filepath.Join(cfg.DataDir, "uploads")
	}
	if value, ok := envValue("BLOG_FRONTEND_DIST"); ok {
		cfg.FrontendDistDir = value
	} else if cfg.FrontendDistDir == "" {
		cfg.FrontendDistDir = filepath.Join(root, "frontend", "dist")
	}
	if value, ok := envValue("BLOG_SITE_NAME"); ok {
		cfg.SiteName = value
	}
	if value, ok := envValue("BLOG_SITE_URL"); ok {
		cfg.SiteURL = value
	}
	if value, ok := envValue("BLOG_JWT_SECRET"); ok {
		cfg.JWTSecret = value
	}
	if value, ok := envValue("BLOG_ADMIN_USERNAME"); ok {
		cfg.AdminUsername = value
	}
	if value, ok := envValue("BLOG_ADMIN_PASSWORD"); ok {
		cfg.AdminPassword = value
	}
	if value, ok := envValue("BLOG_COMMENT_COOLDOWN"); ok {
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			cfg.CommentCooldown = parsed
		}
	}
	if value, ok := envValue("BLOG_RECOMMENDATION_N"); ok {
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			cfg.RecommendationN = parsed
		}
	}

	if value, ok := envValue("BLOG_SMTP_HOST"); ok {
		cfg.SMTPHost = value
		cfg.RuntimeOverrides.SMTPHost = &cfg.SMTPHost
	}
	if value, ok := envValue("BLOG_SMTP_PORT"); ok {
		cfg.SMTPPort = value
		cfg.RuntimeOverrides.SMTPPort = &cfg.SMTPPort
	}
	if value, ok := envValue("BLOG_SMTP_USERNAME"); ok {
		cfg.SMTPUsername = value
		cfg.RuntimeOverrides.SMTPUsername = &cfg.SMTPUsername
	}
	if value, ok := envValue("BLOG_SMTP_PASSWORD"); ok {
		cfg.SMTPPassword = value
		cfg.RuntimeOverrides.SMTPPassword = &cfg.SMTPPassword
	}
	if value, ok := envValue("BLOG_SMTP_FROM"); ok {
		cfg.SMTPFrom = value
		cfg.RuntimeOverrides.SMTPFrom = &cfg.SMTPFrom
	}
	if value, ok := envValue("BLOG_COMMENT_NOTIFY_TO"); ok {
		cfg.CommentNotifyTo = value
		cfg.RuntimeOverrides.AdminNotifyEmail = &cfg.CommentNotifyTo
	}
	if value, ok := envValue("BLOG_LLM_BASE_URL"); ok {
		cfg.LLMBaseURL = value
		cfg.RuntimeOverrides.LLMBaseURL = &cfg.LLMBaseURL
	}
	if value, ok := envValue("BLOG_LLM_API_KEY"); ok {
		cfg.LLMAPIKey = value
		cfg.RuntimeOverrides.LLMAPIKey = &cfg.LLMAPIKey
	}
	if value, ok := envValue("BLOG_LLM_MODEL"); ok {
		cfg.LLMModel = value
		cfg.RuntimeOverrides.LLMModel = &cfg.LLMModel
	}
	if value, ok := envValue("BLOG_LLM_SYSTEM_PROMPT"); ok {
		cfg.LLMSystemPrompt = value
		cfg.RuntimeOverrides.LLMSystemPrompt = &cfg.LLMSystemPrompt
	}
	if value, ok := envValue("BLOG_EMBEDDING_BASE_URL"); ok {
		cfg.EmbeddingBaseURL = value
		cfg.RuntimeOverrides.EmbeddingBaseURL = &cfg.EmbeddingBaseURL
	}
	if value, ok := envValue("BLOG_EMBEDDING_API_KEY"); ok {
		cfg.EmbeddingAPIKey = value
		cfg.RuntimeOverrides.EmbeddingAPIKey = &cfg.EmbeddingAPIKey
	}
	if value, ok := envValue("BLOG_EMBEDDING_MODEL"); ok {
		cfg.EmbeddingModel = value
		cfg.RuntimeOverrides.EmbeddingModel = &cfg.EmbeddingModel
	}
	if value, ok := envIntValue("BLOG_EMBEDDING_DIMENSIONS"); ok {
		cfg.EmbeddingDimensions = value
		cfg.RuntimeOverrides.EmbeddingDimensions = &cfg.EmbeddingDimensions
	}
	if value, ok := envIntValue("BLOG_EMBEDDING_TIMEOUT_MS"); ok {
		cfg.EmbeddingTimeoutMS = value
		cfg.RuntimeOverrides.EmbeddingTimeoutMS = &cfg.EmbeddingTimeoutMS
	}
	if value, ok := envBoolValue("BLOG_SEMANTIC_SEARCH_ENABLED"); ok {
		cfg.SemanticSearchEnabled = value
		cfg.RuntimeOverrides.SemanticSearchEnabled = &cfg.SemanticSearchEnabled
	}
	return cfg
}

func applyRuntimeString(target *string, source *string, marker **string) {
	if source == nil {
		return
	}
	*target = *source
	value := *source
	*marker = &value
}

func applyRuntimeInt(target *int, source *int, marker **int) {
	if source == nil {
		return
	}
	*target = *source
	value := *source
	*marker = &value
}

func applyRuntimeBool(target *bool, source *bool, marker **bool) {
	if source == nil {
		return
	}
	*target = *source
	value := *source
	*marker = &value
}

func envValue(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	return value, ok
}

func envIntValue(key string) (int, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func envBoolValue(key string) (bool, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return false, false
	}
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}
