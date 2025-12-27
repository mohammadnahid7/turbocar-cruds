package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cast"
)

type Config struct {
	Postgres PostgresConfig
	Server   ServerConfig
	Mongo    MongoDBConfig
	Redis    RedisConfig
	Kafka    KafkaConfig
	Token    Token
}

type PostgresConfig struct {
	PDB_NAME     string
	PDB_PORT     string
	PDB_PASSWORD string
	PDB_USER     string
	PDB_HOST     string
}

type RedisConfig struct {
	RDB_ADDRESS  string
	RDB_PASSWORD string
}

type ServerConfig struct {
	CRUD_SERVICE string
	CRUD_SERVER  string
}

type MongoDBConfig struct {
	MDB_ADDRESS string
	MDB_NAME    string
}

type KafkaConfig struct {
	Brokers []string
}

type Token struct {
	TOKEN_KEY string
}

func Load() *Config {
	if err := godotenv.Load(".env"); err != nil {
		log.Printf("error while loading .env file: %v", err)
	}

	return &Config{
		Postgres: PostgresConfig{
			PDB_HOST:     cast.ToString(coalesce("PDB_HOST", "localhost")),
			PDB_PORT:     cast.ToString(coalesce("PDB_PORT", "5432")),
			PDB_USER:     cast.ToString(coalesce("PDB_USER", "postgres")),
			PDB_NAME:     cast.ToString(coalesce("PDB_NAME", "sale")),
			PDB_PASSWORD: cast.ToString(coalesce("PDB_PASSWORD", "3333")),
		},
		Server: ServerConfig{
			CRUD_SERVICE: getPort("CRUD_SERVICE", "50051"),
			CRUD_SERVER:  getPort("CRUD_SERVER", "8090"),
		},
		Mongo: MongoDBConfig{
			MDB_ADDRESS: cast.ToString(coalesce("MDB_ADDRESS", "mongodb://localhost:27017")),
			MDB_NAME:    cast.ToString(coalesce("MDB_NAME", "test")),
		},
		Kafka: KafkaConfig{
			Brokers: cast.ToStringSlice(coalesce("KAFKA_BROKERS", "localhost:9092")),
		},
		Token: Token{
			TOKEN_KEY: cast.ToString(coalesce("TOKEN_KEY", "my-secret-key")),
		},
	}
}

func coalesce(key string, value interface{}) interface{} {
	val, exist := os.LookupEnv(key)
	if exist {
		return val
	}
	return value
}

// getPort returns the port with ":" prefix, checking PORT env var first (Railway compatibility)
func getPort(envKey string, defaultPort string) string {
	// Railway sets PORT environment variable
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	// Check for custom env var
	if port := os.Getenv(envKey); port != "" {
		if port[0] != ':' {
			return ":" + port
		}
		return port
	}
	// Return default with ":"
	if defaultPort[0] != ':' {
		return ":" + defaultPort
	}
	return defaultPort
}
