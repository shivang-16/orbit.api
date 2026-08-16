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
	ClerkSecretKey   string
	Postgres         Postgres
	AWSBedrockAPIKey string
	AWSBedrockRegion string
	AWS              AWS
	SQS              SQS
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
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

func (p Postgres) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		p.Host, p.Port, p.User, p.Password, p.DBName,
	)
}

func Load() Config {
	_ = godotenv.Load()

	return Config{
		Port:           env("PORT", "8080"),
		Env:            env("ENV", "local"),
		CORSOrigins:    splitCSV(env("CORS_ORIGINS", "http://localhost:3000")),
		ClerkSecretKey: env("CLERK_SECRET_KEY", ""),
		Postgres: Postgres{
			Host:     env("POSTGRES_HOST", "localhost"),
			Port:     env("POSTGRES_PORT", "5432"),
			User:     env("POSTGRES_USER", "postgres"),
			Password: env("POSTGRES_PASSWORD", "postgres"),
			DBName:   env("POSTGRES_DB", "orbit"),
		},
		AWSBedrockAPIKey: env("AWS_BEDROCK_API_KEY", ""),
		AWSBedrockRegion: env("AWS_BEDROCK_REGION", "us-east-1"),
		AWS: AWS{
			AccessKeyID:     env("AWS_ACCESS_KEY_ID", ""),
			SecretAccessKey: env("AWS_SECRET_ACCESS_KEY", ""),
		},
		SQS: SQS{
			Region:          env("AWS_QUEUE_REGION", ""),
			BillingQueueURL: env("AWS_SQS_BILLING_QUEUE_URL", ""),
		},
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
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
