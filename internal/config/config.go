package config

import (
	"fmt"
	"os"
)

// Config holds all application configuration loaded from env vars.
type Config struct {
	DatabaseURL string
	GRPCPort    string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50051"
	}

	return &Config{
		DatabaseURL: dbURL,
		GRPCPort:    grpcPort,
	}, nil
}
