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
	"log/slog"
	"testing"
	"time"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{"debug", slog.LevelDebug, false},
		{"INFO", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"bogus", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseLogLevel(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseLogLevel(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}
	if cfg.Sink != "stdout" {
		t.Errorf("Sink = %q, want stdout", cfg.Sink)
	}
	if cfg.MetricsAddr != ":8080" {
		t.Errorf("MetricsAddr = %q, want :8080", cfg.MetricsAddr)
	}
	if cfg.ResyncInterval != 30*time.Minute {
		t.Errorf("ResyncInterval = %v, want 30m", cfg.ResyncInterval)
	}
	if !cfg.EnableMetrics {
		t.Error("EnableMetrics = false, want true")
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json", cfg.LogFormat)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	t.Setenv("EVENTROUTER_SINK", "stdout")
	t.Setenv("EVENTROUTER_RESYNC_INTERVAL", "5m")
	t.Setenv("EVENTROUTER_ENABLE_METRICS", "false")
	t.Setenv("EVENTROUTER_METRICS_ADDR", ":9090")
	t.Setenv("EVENTROUTER_BOOKMARK_PATH", "/var/lib/eventrouter/bookmark")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}
	if cfg.ResyncInterval != 5*time.Minute {
		t.Errorf("ResyncInterval = %v, want 5m", cfg.ResyncInterval)
	}
	if cfg.EnableMetrics {
		t.Error("EnableMetrics = true, want false")
	}
	if cfg.MetricsAddr != ":9090" {
		t.Errorf("MetricsAddr = %q, want :9090", cfg.MetricsAddr)
	}
	if cfg.BookmarkPath != "/var/lib/eventrouter/bookmark" {
		t.Errorf("BookmarkPath = %q, want /var/lib/eventrouter/bookmark", cfg.BookmarkPath)
	}
}

func TestLoadConfigInvalid(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{"bad duration", "EVENTROUTER_RESYNC_INTERVAL", "soon"},
		{"bad bool", "EVENTROUTER_ENABLE_METRICS", "maybe"},
		{"bad log level", "EVENTROUTER_LOG_LEVEL", "loud"},
		{"bad log format", "EVENTROUTER_LOG_FORMAT", "yaml"},
		{"relative bookmark path", "EVENTROUTER_BOOKMARK_PATH", "relative/bookmark"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)
			if _, err := loadConfig(); err == nil {
				t.Fatalf("loadConfig() with %s=%q: want error, got nil", tt.key, tt.value)
			}
		})
	}
}
