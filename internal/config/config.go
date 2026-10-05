// Package config loads and validates process environment configuration.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment       string
	HTTPAddr          string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// Load applies documented defaults to unset variables. Explicit empty or
// malformed values are rejected; error messages never include supplied values.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

func load(lookup func(string) (string, bool)) (Config, error) {
	value := func(key, fallback string) string {
		if v, ok := lookup(key); ok {
			return strings.TrimSpace(v)
		}
		return fallback
	}
	cfg := Config{
		Environment: value("APP_ENV", "development"),
		HTTPAddr:    value("HTTP_ADDR", "127.0.0.1:8080"),
	}
	switch cfg.Environment {
	case "development", "test", "production":
	default:
		return Config{}, fmt.Errorf("APP_ENV must be development, test or production")
	}
	host, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return Config{}, fmt.Errorf("HTTP_ADDR must be a host:port address")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Config{}, fmt.Errorf("HTTP_ADDR port must be an integer from 1 to 65535")
	}
	if host != "" && host != "localhost" && net.ParseIP(host) == nil {
		return Config{}, fmt.Errorf("HTTP_ADDR host must be an IP address, localhost or empty")
	}
	for _, setting := range []struct {
		key      string
		fallback string
		target   *time.Duration
	}{
		{"HTTP_READ_HEADER_TIMEOUT", "5s", &cfg.ReadHeaderTimeout},
		{"HTTP_READ_TIMEOUT", "15s", &cfg.ReadTimeout},
		{"HTTP_WRITE_TIMEOUT", "15s", &cfg.WriteTimeout},
		{"HTTP_IDLE_TIMEOUT", "60s", &cfg.IdleTimeout},
		{"HTTP_SHUTDOWN_TIMEOUT", "10s", &cfg.ShutdownTimeout},
	} {
		d, err := time.ParseDuration(value(setting.key, setting.fallback))
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration, such as 5s", setting.key)
		}
		*setting.target = d
	}
	return cfg, nil
}
