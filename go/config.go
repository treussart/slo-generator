package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	RepoChartPath string `env:"REPO_CHARTS_PATH" envDefault:"/Users/Shared/dev/git/charts"`
}

func NewConfig() (*Config, error) {
	//nolint:dogsled
	_, callerFile, _, _ := runtime.Caller(0)
	dotEnvPath := filepath.Join(filepath.Dir(callerFile), ".env")
	if _, err := os.Stat(dotEnvPath); err == nil {
		if err := godotenv.Load(dotEnvPath); err != nil {
			return nil, fmt.Errorf("godotenv.Load: %w", err)
		}
	}

	config := new(Config)

	err := env.Parse(config)
	if err != nil {
		return nil, fmt.Errorf("env.Parse: %w", err)
	}
	return config, nil
}
