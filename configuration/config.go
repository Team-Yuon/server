package configuration

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	App      AppConfig
}

type ServerConfig struct {
	Port int    `envconfig:"SERVER_PORT" default:"8080"`
	Host string `envconfig:"SERVER_HOST" default:"0.0.0.0"`
	Mode string `envconfig:"SERVER_MODE" default:"release"`
}

type DatabaseConfig struct {
	Host     string `envconfig:"DB_HOST" default:"localhost"`
	Port     int    `envconfig:"DB_PORT" default:"5432"`
	User     string `envconfig:"DB_USER" default:"postgres"`
	Password string `envconfig:"DB_PASSWORD" default:""`
	Name     string `envconfig:"DB_NAME" default:"yuon"`
	SSLMode  string `envconfig:"DB_SSL_MODE" default:"disable"`
}

type AppConfig struct {
	Name        string `envconfig:"APP_NAME" default:"YUON"`
	Version     string `envconfig:"APP_VERSION" default:"1.0.0"`
	Environment string `envconfig:"APP_ENV" default:"development"`
}

func Load() (*Config, error) {
	var cfg Config

	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("환경 변수 로드 실패: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("설정 검증 실패: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("유효하지 않은 서버 포트: %d", c.Server.Port)
	}

	if c.Server.Mode != "debug" && c.Server.Mode != "release" {
		return fmt.Errorf("유효하지 않은 서버 모드: %s (debug 또는 release 사용)", c.Server.Mode)
	}

	if c.App.Environment != "development" && c.App.Environment != "staging" && c.App.Environment != "production" {
		return fmt.Errorf("유효하지 않은 환경: %s", c.App.Environment)
	}

	return nil
}

func (c *Config) IsDevelopment() bool {
	return c.App.Environment == "development"
}

func (c *Config) IsProduction() bool {
	return c.App.Environment == "production"
}
