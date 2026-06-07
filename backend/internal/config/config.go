package config

import (
	"os"
)

type Config struct {
	Port       string
	SchemaPath string
}

func Load() Config {
	return Config{
		Port:       getenv("PORT", "8080"),
		SchemaPath: getenv("SCRIPT_SCHEMA_PATH", "../schemas/script.schema.json"),
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
