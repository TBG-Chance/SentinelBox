package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const maxConfigBytes = 1 << 20

func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, fmt.Errorf("configuration file path is required")
	}

	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open configuration file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Config{}, fmt.Errorf("inspect configuration file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Config{}, fmt.Errorf("configuration path must be a regular file")
	}
	if info.Size() > maxConfigBytes {
		return Config{}, fmt.Errorf("configuration file exceeds %d bytes", maxConfigBytes)
	}

	cfg := Default()
	decoder := toml.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		var strictError *toml.StrictMissingError
		if errors.As(err, &strictError) {
			return Config{}, fmt.Errorf("decode configuration: %w: %s", err, strictError.String())
		}
		return Config{}, fmt.Errorf("decode configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
