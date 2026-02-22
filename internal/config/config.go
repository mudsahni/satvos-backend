package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	Server     ServerConfig
	DB         DBConfig
	JWT        JWTConfig
	S3         S3Config
	Log        LogConfig
	Parser     ParserConfig
	CORS       CORSConfig
	Queue      QueueConfig
	FreeTier   FreeTierConfig
	Email      EmailConfig
	GoogleAuth GoogleAuthConfig
	DynamoDB   DynamoDBConfig
}

// DynamoDBConfig holds DynamoDB settings for email processing config.
type DynamoDBConfig struct {
	TableName string `mapstructure:"table_name"`
	Region    string `mapstructure:"region"`
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
}

// GoogleAuthConfig holds Google OAuth settings.
type GoogleAuthConfig struct {
	ClientID string `mapstructure:"client_id"`
}

// EmailConfig holds email delivery settings.
type EmailConfig struct {
	Provider    string `mapstructure:"provider"`
	Region      string `mapstructure:"region"`
	FromAddress string `mapstructure:"from_address"`
	FromName    string `mapstructure:"from_name"`
	FrontendURL string `mapstructure:"frontend_url"`
	AccessKey   string `mapstructure:"access_key"`
	SecretKey   string `mapstructure:"secret_key"`
}

// FreeTierConfig holds free tier settings.
type FreeTierConfig struct {
	TenantSlug   string `mapstructure:"tenant_slug"`
	MonthlyLimit int    `mapstructure:"monthly_limit"`
}

// QueueConfig holds parse queue worker settings.
type QueueConfig struct {
	PollIntervalSecs int `mapstructure:"poll_interval_secs"`
	MaxRetries       int `mapstructure:"max_retries"`
	Concurrency      int `mapstructure:"concurrency"`
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// ParserProviderConfig holds settings for a single LLM parser provider.
type ParserProviderConfig struct {
	Provider     string `mapstructure:"provider"`
	APIKey       string `mapstructure:"api_key"`
	DefaultModel string `mapstructure:"default_model"`
	MaxRetries   int    `mapstructure:"max_retries"`
	TimeoutSecs  int    `mapstructure:"timeout_secs"`
}

// ParserConfig holds LLM document parser settings with multi-provider support.
type ParserConfig struct {
	// Legacy flat fields (backwards-compatible)
	Provider     string `mapstructure:"provider"`
	APIKey       string `mapstructure:"api_key"`
	DefaultModel string `mapstructure:"default_model"`
	MaxRetries   int    `mapstructure:"max_retries"`
	TimeoutSecs  int    `mapstructure:"timeout_secs"`

	// Multi-provider fields
	Primary   ParserProviderConfig `mapstructure:"primary"`
	Secondary ParserProviderConfig `mapstructure:"secondary"`
	Tertiary  ParserProviderConfig `mapstructure:"tertiary"`
}

// PrimaryConfig returns the primary parser provider config, falling back to legacy flat fields.
func (p *ParserConfig) PrimaryConfig() *ParserProviderConfig {
	if p.Primary.Provider != "" {
		return &p.Primary
	}
	return &ParserProviderConfig{
		Provider:     p.Provider,
		APIKey:       p.APIKey,
		DefaultModel: p.DefaultModel,
		MaxRetries:   p.MaxRetries,
		TimeoutSecs:  p.TimeoutSecs,
	}
}

// SecondaryConfig returns the secondary parser provider config, or nil if not configured.
func (p *ParserConfig) SecondaryConfig() *ParserProviderConfig {
	if p.Secondary.Provider != "" {
		return &p.Secondary
	}
	return nil
}

// TertiaryConfig returns the tertiary parser provider config, or nil if not configured.
func (p *ParserConfig) TertiaryConfig() *ParserProviderConfig {
	if p.Tertiary.Provider != "" {
		return &p.Tertiary
	}
	return nil
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port         string        `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	Environment  string        `mapstructure:"environment"`
	APIBaseURL   string        `mapstructure:"api_base_url"`
}

// DBConfig holds PostgreSQL connection settings.
type DBConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	SSLMode  string `mapstructure:"sslmode"`
	MaxOpen  int    `mapstructure:"max_open"`
	MaxIdle  int    `mapstructure:"max_idle"`
}

// DSN returns the PostgreSQL connection string.
func (d *DBConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode,
	)
}

// JWTConfig holds JWT signing and expiry settings.
type JWTConfig struct {
	Secret             string        `mapstructure:"secret"`
	AccessTokenExpiry  time.Duration `mapstructure:"access_expiry"`
	RefreshTokenExpiry time.Duration `mapstructure:"refresh_expiry"`
	Issuer             string        `mapstructure:"issuer"`
}

// S3Config holds AWS S3 settings.
type S3Config struct {
	Region        string `mapstructure:"region"`
	Bucket        string `mapstructure:"bucket"`
	Endpoint      string `mapstructure:"endpoint"`
	AccessKey     string `mapstructure:"access_key"`
	SecretKey     string `mapstructure:"secret_key"`
	MaxFileSizeMB int64  `mapstructure:"max_file_size_mb"`
	PresignExpiry int64  `mapstructure:"presign_expiry"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Load reads configuration from environment variables with the SATVOS_ prefix.
func Load() (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("SATVOS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)
	if err := bindEnvVars(v); err != nil {
		return nil, err
	}

	return &Config{
		Server:     buildServerConfig(v),
		DB:         buildDBConfig(v),
		JWT:        buildJWTConfig(v),
		S3:         buildS3Config(v),
		Log:        buildLogConfig(v),
		CORS:       buildCORSConfig(v),
		Parser:     buildParserConfig(v),
		Queue:      buildQueueConfig(v),
		FreeTier:   buildFreeTierConfig(v),
		Email:      buildEmailConfig(v),
		GoogleAuth: buildGoogleAuthConfig(v),
		DynamoDB:   buildDynamoDBConfig(v),
	}, nil
}

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.port", ":8080")
	v.SetDefault("server.read_timeout", "15s")
	v.SetDefault("server.write_timeout", "15s")
	v.SetDefault("server.environment", "development")
	v.SetDefault("server.api_base_url", "http://localhost:8080")

	// DB
	v.SetDefault("db.host", "localhost")
	v.SetDefault("db.port", 5432)
	v.SetDefault("db.user", "satvos")
	v.SetDefault("db.password", "satvos_secret")
	v.SetDefault("db.name", "satvos_db")
	v.SetDefault("db.sslmode", "disable")
	v.SetDefault("db.max_open", 25)
	v.SetDefault("db.max_idle", 10)

	// JWT
	v.SetDefault("jwt.secret", "change-me-in-production")
	v.SetDefault("jwt.access_expiry", "15m")
	v.SetDefault("jwt.refresh_expiry", "168h")
	v.SetDefault("jwt.issuer", "satvos")

	// S3
	v.SetDefault("s3.region", "us-east-1")
	v.SetDefault("s3.bucket", "satvos-uploads")
	v.SetDefault("s3.endpoint", "")
	v.SetDefault("s3.max_file_size_mb", 50)
	v.SetDefault("s3.presign_expiry", 3600)

	// Log
	v.SetDefault("log.level", "debug")
	v.SetDefault("log.format", "console")

	// CORS (localhost origins for development)
	v.SetDefault("cors.allowed_origins", "http://localhost:3000,http://127.0.0.1:3000,http://localhost:3001,http://127.0.0.1:3001")

	// Queue
	v.SetDefault("queue.poll_interval_secs", 10)
	v.SetDefault("queue.max_retries", 5)
	v.SetDefault("queue.concurrency", 5)

	// Email
	v.SetDefault("email.provider", "noop")
	v.SetDefault("email.region", "ap-south-1")
	v.SetDefault("email.from_address", "noreply@satvos.com")
	v.SetDefault("email.from_name", "SATVOS")
	v.SetDefault("email.frontend_url", "http://localhost:3000")
	v.SetDefault("email.access_key", "")
	v.SetDefault("email.secret_key", "")

	// Google Auth
	v.SetDefault("google_auth.client_id", "")

	// Free tier
	v.SetDefault("free_tier.tenant_slug", "satvos")
	v.SetDefault("free_tier.monthly_limit", 5)

	// DynamoDB
	v.SetDefault("dynamodb.table_name", "")
	v.SetDefault("dynamodb.region", "ap-south-1")
	v.SetDefault("dynamodb.endpoint", "")
	v.SetDefault("dynamodb.access_key", "")
	v.SetDefault("dynamodb.secret_key", "")

	// Parser (legacy flat)
	v.SetDefault("parser.provider", "claude")
	v.SetDefault("parser.api_key", "")
	v.SetDefault("parser.default_model", "claude-sonnet-4-20250514")
	v.SetDefault("parser.max_retries", 2)
	v.SetDefault("parser.timeout_secs", 120)

	// Parser primary/secondary/tertiary
	v.SetDefault("parser.primary.provider", "")
	v.SetDefault("parser.primary.api_key", "")
	v.SetDefault("parser.primary.default_model", "")
	v.SetDefault("parser.primary.max_retries", 2)
	v.SetDefault("parser.primary.timeout_secs", 120)
	v.SetDefault("parser.secondary.provider", "")
	v.SetDefault("parser.secondary.api_key", "")
	v.SetDefault("parser.secondary.default_model", "")
	v.SetDefault("parser.secondary.max_retries", 2)
	v.SetDefault("parser.secondary.timeout_secs", 120)
	v.SetDefault("parser.tertiary.provider", "")
	v.SetDefault("parser.tertiary.api_key", "")
	v.SetDefault("parser.tertiary.default_model", "")
	v.SetDefault("parser.tertiary.max_retries", 2)
	v.SetDefault("parser.tertiary.timeout_secs", 120)
}

func bindEnvVars(v *viper.Viper) error {
	envBindings := map[string]string{
		"server.port":          "SATVOS_SERVER_PORT",
		"server.read_timeout":  "SATVOS_SERVER_READ_TIMEOUT",
		"server.write_timeout": "SATVOS_SERVER_WRITE_TIMEOUT",
		"server.environment":   "SATVOS_SERVER_ENVIRONMENT",
		"server.api_base_url":  "SATVOS_SERVER_API_BASE_URL",
		"db.host":              "SATVOS_DB_HOST",
		"db.port":              "SATVOS_DB_PORT",
		"db.user":              "SATVOS_DB_USER",
		"db.password":          "SATVOS_DB_PASSWORD",
		"db.name":              "SATVOS_DB_NAME",
		"db.sslmode":           "SATVOS_DB_SSLMODE",
		"db.max_open":          "SATVOS_DB_MAX_OPEN",
		"db.max_idle":          "SATVOS_DB_MAX_IDLE",
		"jwt.secret":           "SATVOS_JWT_SECRET",
		"jwt.access_expiry":    "SATVOS_JWT_ACCESS_EXPIRY",
		"jwt.refresh_expiry":   "SATVOS_JWT_REFRESH_EXPIRY",
		"jwt.issuer":           "SATVOS_JWT_ISSUER",
		"s3.region":            "SATVOS_S3_REGION",
		"s3.bucket":            "SATVOS_S3_BUCKET",
		"s3.endpoint":          "SATVOS_S3_ENDPOINT",
		"s3.access_key":        "SATVOS_S3_ACCESS_KEY",
		"s3.secret_key":        "SATVOS_S3_SECRET_KEY",
		"s3.max_file_size_mb":  "SATVOS_S3_MAX_FILE_SIZE_MB",
		"s3.presign_expiry":    "SATVOS_S3_PRESIGN_EXPIRY",
		"log.level":            "SATVOS_LOG_LEVEL",
		"log.format":           "SATVOS_LOG_FORMAT",
		"cors.allowed_origins":           "SATVOS_CORS_ALLOWED_ORIGINS",
		"queue.poll_interval_secs":       "SATVOS_QUEUE_POLL_INTERVAL_SECS",
		"queue.max_retries":              "SATVOS_QUEUE_MAX_RETRIES",
		"queue.concurrency":              "SATVOS_QUEUE_CONCURRENCY",
		"parser.provider":                "SATVOS_PARSER_PROVIDER",
		"parser.api_key":                 "SATVOS_PARSER_API_KEY",
		"parser.default_model":           "SATVOS_PARSER_DEFAULT_MODEL",
		"parser.max_retries":             "SATVOS_PARSER_MAX_RETRIES",
		"parser.timeout_secs":            "SATVOS_PARSER_TIMEOUT_SECS",
		"parser.primary.provider":        "SATVOS_PARSER_PRIMARY_PROVIDER",
		"parser.primary.api_key":         "SATVOS_PARSER_PRIMARY_API_KEY",
		"parser.primary.default_model":   "SATVOS_PARSER_PRIMARY_DEFAULT_MODEL",
		"parser.primary.max_retries":     "SATVOS_PARSER_PRIMARY_MAX_RETRIES",
		"parser.primary.timeout_secs":    "SATVOS_PARSER_PRIMARY_TIMEOUT_SECS",
		"parser.secondary.provider":      "SATVOS_PARSER_SECONDARY_PROVIDER",
		"parser.secondary.api_key":       "SATVOS_PARSER_SECONDARY_API_KEY",
		"parser.secondary.default_model": "SATVOS_PARSER_SECONDARY_DEFAULT_MODEL",
		"parser.secondary.max_retries":   "SATVOS_PARSER_SECONDARY_MAX_RETRIES",
		"parser.secondary.timeout_secs":  "SATVOS_PARSER_SECONDARY_TIMEOUT_SECS",
		"parser.tertiary.provider":       "SATVOS_PARSER_TERTIARY_PROVIDER",
		"parser.tertiary.api_key":        "SATVOS_PARSER_TERTIARY_API_KEY",
		"parser.tertiary.default_model":  "SATVOS_PARSER_TERTIARY_DEFAULT_MODEL",
		"parser.tertiary.max_retries":    "SATVOS_PARSER_TERTIARY_MAX_RETRIES",
		"parser.tertiary.timeout_secs":   "SATVOS_PARSER_TERTIARY_TIMEOUT_SECS",
		"email.provider":                 "SATVOS_EMAIL_PROVIDER",
		"email.region":                   "SATVOS_EMAIL_REGION",
		"email.from_address":             "SATVOS_EMAIL_FROM_ADDRESS",
		"email.from_name":                "SATVOS_EMAIL_FROM_NAME",
		"email.frontend_url":             "SATVOS_EMAIL_FRONTEND_URL",
		"email.access_key":               "SATVOS_EMAIL_ACCESS_KEY",
		"email.secret_key":               "SATVOS_EMAIL_SECRET_KEY",
		"free_tier.tenant_slug":          "SATVOS_FREE_TIER_TENANT_SLUG",
		"free_tier.monthly_limit":        "SATVOS_FREE_TIER_MONTHLY_LIMIT",
		"google_auth.client_id":          "SATVOS_GOOGLE_AUTH_CLIENT_ID",
		"dynamodb.table_name":            "SATVOS_DYNAMODB_TABLE_NAME",
		"dynamodb.region":                "SATVOS_DYNAMODB_REGION",
		"dynamodb.endpoint":              "SATVOS_DYNAMODB_ENDPOINT",
		"dynamodb.access_key":            "SATVOS_DYNAMODB_ACCESS_KEY",
		"dynamodb.secret_key":            "SATVOS_DYNAMODB_SECRET_KEY",
	}
	for key, env := range envBindings {
		if err := v.BindEnv(key, env); err != nil {
			return fmt.Errorf("binding env var %s: %w", env, err)
		}
	}
	return nil
}

func buildServerConfig(v *viper.Viper) ServerConfig {
	// Railway/Heroku/Render set a PORT env var. Use it if SATVOS_SERVER_PORT is not explicitly set.
	serverPort := v.GetString("server.port")
	if port := os.Getenv("PORT"); port != "" && os.Getenv("SATVOS_SERVER_PORT") == "" {
		serverPort = ":" + port
	}
	return ServerConfig{
		Port:         serverPort,
		ReadTimeout:  v.GetDuration("server.read_timeout"),
		WriteTimeout: v.GetDuration("server.write_timeout"),
		Environment:  v.GetString("server.environment"),
		APIBaseURL:   v.GetString("server.api_base_url"),
	}
}

func buildDBConfig(v *viper.Viper) DBConfig {
	return DBConfig{
		Host:     v.GetString("db.host"),
		Port:     v.GetInt("db.port"),
		User:     v.GetString("db.user"),
		Password: v.GetString("db.password"),
		Name:     v.GetString("db.name"),
		SSLMode:  v.GetString("db.sslmode"),
		MaxOpen:  v.GetInt("db.max_open"),
		MaxIdle:  v.GetInt("db.max_idle"),
	}
}

func buildJWTConfig(v *viper.Viper) JWTConfig {
	return JWTConfig{
		Secret:             v.GetString("jwt.secret"),
		AccessTokenExpiry:  v.GetDuration("jwt.access_expiry"),
		RefreshTokenExpiry: v.GetDuration("jwt.refresh_expiry"),
		Issuer:             v.GetString("jwt.issuer"),
	}
}

func buildS3Config(v *viper.Viper) S3Config {
	return S3Config{
		Region:        v.GetString("s3.region"),
		Bucket:        v.GetString("s3.bucket"),
		Endpoint:      v.GetString("s3.endpoint"),
		AccessKey:     v.GetString("s3.access_key"),
		SecretKey:     v.GetString("s3.secret_key"),
		MaxFileSizeMB: v.GetInt64("s3.max_file_size_mb"),
		PresignExpiry: v.GetInt64("s3.presign_expiry"),
	}
}

func buildLogConfig(v *viper.Viper) LogConfig {
	return LogConfig{
		Level:  v.GetString("log.level"),
		Format: v.GetString("log.format"),
	}
}

func buildCORSConfig(v *viper.Viper) CORSConfig {
	var origins []string
	for _, o := range strings.Split(v.GetString("cors.allowed_origins"), ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins = append(origins, o)
		}
	}
	return CORSConfig{AllowedOrigins: origins}
}

func buildParserConfig(v *viper.Viper) ParserConfig {
	return ParserConfig{
		Provider:     v.GetString("parser.provider"),
		APIKey:       v.GetString("parser.api_key"),
		DefaultModel: v.GetString("parser.default_model"),
		MaxRetries:   v.GetInt("parser.max_retries"),
		TimeoutSecs:  v.GetInt("parser.timeout_secs"),
		Primary: ParserProviderConfig{
			Provider:     v.GetString("parser.primary.provider"),
			APIKey:       v.GetString("parser.primary.api_key"),
			DefaultModel: v.GetString("parser.primary.default_model"),
			MaxRetries:   v.GetInt("parser.primary.max_retries"),
			TimeoutSecs:  v.GetInt("parser.primary.timeout_secs"),
		},
		Secondary: ParserProviderConfig{
			Provider:     v.GetString("parser.secondary.provider"),
			APIKey:       v.GetString("parser.secondary.api_key"),
			DefaultModel: v.GetString("parser.secondary.default_model"),
			MaxRetries:   v.GetInt("parser.secondary.max_retries"),
			TimeoutSecs:  v.GetInt("parser.secondary.timeout_secs"),
		},
		Tertiary: ParserProviderConfig{
			Provider:     v.GetString("parser.tertiary.provider"),
			APIKey:       v.GetString("parser.tertiary.api_key"),
			DefaultModel: v.GetString("parser.tertiary.default_model"),
			MaxRetries:   v.GetInt("parser.tertiary.max_retries"),
			TimeoutSecs:  v.GetInt("parser.tertiary.timeout_secs"),
		},
	}
}

func buildQueueConfig(v *viper.Viper) QueueConfig {
	return QueueConfig{
		PollIntervalSecs: v.GetInt("queue.poll_interval_secs"),
		MaxRetries:       v.GetInt("queue.max_retries"),
		Concurrency:      v.GetInt("queue.concurrency"),
	}
}

func buildFreeTierConfig(v *viper.Viper) FreeTierConfig {
	return FreeTierConfig{
		TenantSlug:   v.GetString("free_tier.tenant_slug"),
		MonthlyLimit: v.GetInt("free_tier.monthly_limit"),
	}
}

func buildEmailConfig(v *viper.Viper) EmailConfig {
	return EmailConfig{
		Provider:    v.GetString("email.provider"),
		Region:      v.GetString("email.region"),
		FromAddress: v.GetString("email.from_address"),
		FromName:    v.GetString("email.from_name"),
		FrontendURL: v.GetString("email.frontend_url"),
		AccessKey:   v.GetString("email.access_key"),
		SecretKey:   v.GetString("email.secret_key"),
	}
}

func buildGoogleAuthConfig(v *viper.Viper) GoogleAuthConfig {
	return GoogleAuthConfig{
		ClientID: v.GetString("google_auth.client_id"),
	}
}

func buildDynamoDBConfig(v *viper.Viper) DynamoDBConfig {
	return DynamoDBConfig{
		TableName: v.GetString("dynamodb.table_name"),
		Region:    v.GetString("dynamodb.region"),
		Endpoint:  v.GetString("dynamodb.endpoint"),
		AccessKey: v.GetString("dynamodb.access_key"),
		SecretKey: v.GetString("dynamodb.secret_key"),
	}
}
