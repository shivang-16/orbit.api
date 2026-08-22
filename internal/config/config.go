package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port             string
	Env              string
	CORSOrigins      []string
	FrontendURL      string
	ClerkSecretKey   string
	Postgres         Postgres
	AWSBedrockAPIKey string
	AWSBedrockRegion string
	AWS              AWS
	SQS              SQS
	Dodo             Dodo
	Resend           Resend
	RateLimits       RateLimits
	Credits          Credits
	Server           Server
}

type Server struct {
	DashboardTimeoutSeconds int
	InferenceTimeoutSeconds int
}

type Credits struct {
	SignupMicros              int64 `yaml:"signup_micros"`
	LowBalanceThresholdMicros int64 `yaml:"low_balance_threshold_micros"`
	DefaultOutputTokens       int   `yaml:"default_output_tokens"`
}

type RateLimits struct {
	Organization RateLimitWindow `yaml:"organization"`
	Playground   RateLimitWindow `yaml:"playground"`
}

type RateLimitWindow struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
	Burst             int `yaml:"burst"`
	Concurrent        int `yaml:"concurrent"`
}

type Dodo struct {
	APIKey     string
	Env        string
	WebhookKey string
}

type Resend struct {
	APIKey    string
	FromEmail string
}

type AWS struct {
	AccessKeyID     string
	SecretAccessKey string
}

type SQS struct {
	Region          string
	BillingQueueURL string
}

type Postgres struct {
	Host         string
	Port         string
	User         string
	Password     string
	DBName       string
	SSLMode      string
	MaxOpenConns int
	MaxIdleConns int
}

func (p Postgres) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.User, p.Password, p.DBName, p.SSLMode,
	)
}

func Load() Config {
	_ = godotenv.Load()
	file := loadFileConfig()

	cors := resolveCORS(file.Server.CORSOrigins)

	return Config{
		Port:           env("PORT", nonEmpty(file.Server.Port, "8080")),
		Env:            env("ENV", nonEmpty(file.Server.Env, "local")),
		CORSOrigins:    cors,
		FrontendURL:    env("FRONTEND_URL", nonEmpty(file.Server.FrontendURL, "http://localhost:3000")),
		ClerkSecretKey: env("CLERK_SECRET_KEY", ""),
		Postgres: Postgres{
			Host:         env("POSTGRES_HOST", "localhost"),
			Port:         env("POSTGRES_PORT", "5432"),
			User:         env("POSTGRES_USER", "postgres"),
			Password:     env("POSTGRES_PASSWORD", "postgres"),
			DBName:       env("POSTGRES_DB", "orbit"),
			SSLMode:      env("POSTGRES_SSLMODE", "disable"),
			MaxOpenConns: file.Postgres.MaxOpenConns,
			MaxIdleConns: file.Postgres.MaxIdleConns,
		},
		AWSBedrockAPIKey: env("AWS_BEDROCK_API_KEY", ""),
		AWSBedrockRegion: env("AWS_BEDROCK_REGION", nonEmpty(file.Bedrock.Region, "us-east-1")),
		AWS: AWS{
			AccessKeyID:     env("AWS_ACCESS_KEY_ID", ""),
			SecretAccessKey: env("AWS_SECRET_ACCESS_KEY", ""),
		},
		SQS: SQS{
			Region:          env("AWS_QUEUE_REGION", ""),
			BillingQueueURL: env("AWS_SQS_BILLING_QUEUE_URL", ""),
		},
		Dodo: Dodo{
			APIKey:     env("DODO_PAYMENTS_API_KEY", ""),
			Env:        env("DODO_ENV", "test"),
			WebhookKey: env("DODO_WEBHOOK_KEY", ""),
		},
		Resend: Resend{
			APIKey:    env("RESEND_API_KEY", ""),
			FromEmail: env("RESEND_FROM_EMAIL", "Shivang Yadav <hi@tryorbit.cloud>"),
		},
		RateLimits: file.RateLimits,
		Credits:    file.Credits,
		Server: Server{
			DashboardTimeoutSeconds: file.Server.DashboardTimeoutSeconds,
			InferenceTimeoutSeconds: file.Server.InferenceTimeoutSeconds,
		},
	}
}

func resolveCORS(fileOrigins []string) []string {
	if fromEnv := splitCSV(os.Getenv("CORS_ORIGINS")); len(fromEnv) > 0 {
		return fromEnv
	}
	if len(fileOrigins) > 0 {
		return fileOrigins
	}
	return []string{"http://localhost:3000"}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
