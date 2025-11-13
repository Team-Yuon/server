package configuration

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	App        AppConfig
	OpenAI     OpenAIConfig
	Qdrant     QdrantConfig
	OpenSearch OpenSearchConfig
	Auth       AuthConfig
	Storage    StorageConfig
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

type OpenAIConfig struct {
	APIKey         string  `envconfig:"OPENAI_API_KEY"`
	Model          string  `envconfig:"OPENAI_MODEL" default:"gpt-4o-mini"`
	EmbeddingModel string  `envconfig:"OPENAI_EMBEDDING_MODEL" default:"text-embedding-3-small"`
	MaxTokens      int     `envconfig:"OPENAI_MAX_TOKENS" default:"1000"`
	Temperature    float32 `envconfig:"OPENAI_TEMPERATURE" default:"0.7"`
}

type QdrantConfig struct {
	URL        string `envconfig:"QDRANT_URL" default:"http://localhost:6333"`
	APIKey     string `envconfig:"QDRANT_API_KEY"`
	Collection string `envconfig:"QDRANT_COLLECTION" default:"documents"`
	VectorSize int    `envconfig:"QDRANT_VECTOR_SIZE" default:"1536"`
}

type OpenSearchConfig struct {
	URL      string `envconfig:"OPENSEARCH_URL" default:"http://localhost:9200"`
	Username string `envconfig:"OPENSEARCH_USERNAME" default:"admin"`
	Password string `envconfig:"OPENSEARCH_PASSWORD" default:"admin"`
	Index    string `envconfig:"OPENSEARCH_INDEX" default:"documents"`
}

type AuthConfig struct {
	RootPassword string `envconfig:"ROOT_ADMIN_PASSWORD"`
	JWTSecret    string `envconfig:"JWT_SECRET"`
}

type StorageConfig struct {
	Endpoint   string `envconfig:"S3_ENDPOINT"`
	Region     string `envconfig:"S3_REGION" default:"us-east-1"`
	AccessKey  string `envconfig:"S3_ACCESS_KEY"`
	SecretKey  string `envconfig:"S3_SECRET_KEY"`
	Bucket     string `envconfig:"S3_BUCKET"`
	UsePath    bool   `envconfig:"S3_USE_PATH_STYLE" default:"true"`
	BaseURL    string `envconfig:"S3_BASE_URL"`
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
