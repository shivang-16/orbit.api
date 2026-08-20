package config

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed config.yaml
var embeddedYAML []byte

type fileConfig struct {
	Server     serverFile   `yaml:"server"`
	Credits    Credits      `yaml:"credits"`
	RateLimits RateLimits   `yaml:"rate_limits"`
	Postgres   postgresFile `yaml:"postgres"`
	Bedrock    bedrockFile  `yaml:"bedrock"`
}

type serverFile struct {
	Env                     string   `yaml:"env"`
	Port                    string   `yaml:"port"`
	CORSOrigins             []string `yaml:"cors_origins"`
	FrontendURL             string   `yaml:"frontend_url"`
	DashboardTimeoutSeconds int      `yaml:"dashboard_timeout_seconds"`
	InferenceTimeoutSeconds int      `yaml:"inference_timeout_seconds"`
}

type postgresFile struct {
	MaxOpenConns int `yaml:"max_open_conns"`
	MaxIdleConns int `yaml:"max_idle_conns"`
}

type bedrockFile struct {
	Region string `yaml:"region"`
}

func loadFileConfig() fileConfig {
	raw := embeddedYAML
	path := env("CONFIG_FILE", "")
	if path == "" {
		path = findConfigFile()
	}
	if path != "" {
		disk, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("config: read %s: %v", path, err)
		}
		raw = disk
		log.Printf("config: loaded %s", path)
	}

	var file fileConfig
	if err := yaml.Unmarshal(raw, &file); err != nil {
		log.Fatalf("config: parse yaml: %v", err)
	}
	if err := file.validate(); err != nil {
		log.Fatalf("config: invalid yaml: %v", err)
	}
	return file
}

func (f fileConfig) validate() error {
	if f.Credits.SignupMicros < 1 {
		return fmt.Errorf("credits.signup_micros must be >= 1")
	}
	if f.Credits.LowBalanceThresholdMicros < 1 {
		return fmt.Errorf("credits.low_balance_threshold_micros must be >= 1")
	}
	if f.Credits.DefaultOutputTokens < 1 {
		return fmt.Errorf("credits.default_output_tokens must be >= 1")
	}
	if f.Server.DashboardTimeoutSeconds < 1 {
		return fmt.Errorf("server.dashboard_timeout_seconds must be >= 1")
	}
	if f.Server.InferenceTimeoutSeconds < 1 {
		return fmt.Errorf("server.inference_timeout_seconds must be >= 1")
	}
	if f.Postgres.MaxOpenConns < 1 {
		return fmt.Errorf("postgres.max_open_conns must be >= 1")
	}
	if f.Postgres.MaxIdleConns < 1 {
		return fmt.Errorf("postgres.max_idle_conns must be >= 1")
	}
	return f.RateLimits.validate()
}

func (l RateLimits) validate() error {
	if l.Organization.RequestsPerMinute < 1 {
		return fmt.Errorf("rate_limits.organization.requests_per_minute must be >= 1")
	}
	if l.Organization.Burst < 1 {
		return fmt.Errorf("rate_limits.organization.burst must be >= 1")
	}
	if l.Organization.Concurrent < 1 {
		return fmt.Errorf("rate_limits.organization.concurrent must be >= 1")
	}
	if l.Playground.RequestsPerMinute < 1 {
		return fmt.Errorf("rate_limits.playground.requests_per_minute must be >= 1")
	}
	if l.Playground.Burst < 1 {
		return fmt.Errorf("rate_limits.playground.burst must be >= 1")
	}
	return nil
}

func findConfigFile() string {
	seen := map[string]struct{}{}
	starts := []string{}
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		dir := start
		for i := 0; i < 10; i++ {
			if _, dup := seen[dir]; dup {
				break
			}
			seen[dir] = struct{}{}
			path := filepath.Join(dir, "internal", "config", "config.yaml")
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return ""
}
