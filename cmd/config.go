// Copyright © 2026 Kube logging authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the runtime configuration, sourced from environment variables.
type Config struct {
	Sink            string
	StdoutNamespace string
	ResyncInterval  time.Duration
	MetricsAddr     string
	EnableMetrics   bool
	BookmarkPath    string
	Kubeconfig      string
	LogFormat       string
	LogLevel        slog.Level
}

func loadConfig() (Config, error) {
	cfg := Config{
		Sink:            envOr("EVENTROUTER_SINK", "stdout"),
		StdoutNamespace: os.Getenv("EVENTROUTER_STDOUT_NAMESPACE"),
		MetricsAddr:     envOr("EVENTROUTER_METRICS_ADDR", ":8080"),
		BookmarkPath:    os.Getenv("EVENTROUTER_BOOKMARK_PATH"),
		Kubeconfig:      envOr("EVENTROUTER_KUBECONFIG", os.Getenv("KUBECONFIG")),
		LogFormat:       envOr("EVENTROUTER_LOG_FORMAT", "json"),
	}

	var err error
	if cfg.ResyncInterval, err = envDuration("EVENTROUTER_RESYNC_INTERVAL", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.EnableMetrics, err = envBool("EVENTROUTER_ENABLE_METRICS", true); err != nil {
		return Config{}, err
	}
	if cfg.LogLevel, err = parseLogLevel(envOr("EVENTROUTER_LOG_LEVEL", "info")); err != nil {
		return Config{}, err
	}

	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		return Config{}, fmt.Errorf("invalid EVENTROUTER_LOG_FORMAT %q (want json or text)", cfg.LogFormat)
	}
	if cfg.BookmarkPath != "" {
		if cfg.BookmarkPath, err = validatedPath(cfg.BookmarkPath); err != nil {
			return Config{}, fmt.Errorf("invalid EVENTROUTER_BOOKMARK_PATH: %w", err)
		}
	}
	return cfg, nil
}

// newLogger logs to stderr so output never mixes with the stdout event stream.
func newLogger(cfg Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, opts))
}

func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid EVENTROUTER_LOG_LEVEL %q (want debug, info, warn or error)", s)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	return b, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	return d, nil
}
