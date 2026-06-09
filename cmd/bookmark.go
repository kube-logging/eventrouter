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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// validatedPath rejects relative paths and traversal so a config value cannot escape to arbitrary files.
func validatedPath(p string) (string, error) {
	if strings.Contains(p, "..") {
		return "", fmt.Errorf("path %q must not contain '..'", p)
	}
	clean := filepath.Clean(p)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("path %q must be absolute", p)
	}
	return clean, nil
}

// resourceVersionNewer reports whether rv is numerically newer than current.
// An unparseable rv counts as not newer; an empty or unparseable current as oldest.
func resourceVersionNewer(rv, current string) bool {
	if rv == "" {
		return false
	}
	rvInt, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		slog.Error("cannot parse resource version", "resourceVersion", rv, "error", err)
		return false
	}
	currentInt, err := strconv.ParseInt(current, 10, 64)
	if err != nil {
		return true
	}
	return rvInt > currentInt
}

// resourceVersionBookmark returns the last persisted resource version and a
// concurrency-safe callback to persist newer ones. An empty or invalid path disables persistence.
func resourceVersionBookmark(path string) (last string, persist func(rv string)) {
	noop := func(string) {}
	if path == "" {
		return "", noop
	}

	cleanPath, err := validatedPath(path)
	if err != nil {
		slog.Error("invalid bookmark path, disabling persistence", "error", err)
		return "", noop
	}

	switch data, err := os.ReadFile(cleanPath); {
	case err == nil:
		last = string(data)
		slog.Info("resuming from resource version", "resourceVersion", last)
	case os.IsNotExist(err):
		slog.Info("no existing resource version bookmark, starting fresh")
	default:
		slog.Error("failed to read resource version bookmark", "path", cleanPath, "error", err)
	}

	var mu sync.Mutex
	mostRecent := last
	persist = func(rv string) {
		mu.Lock()
		defer mu.Unlock()
		if !resourceVersionNewer(rv, mostRecent) {
			return
		}
		if err := os.WriteFile(cleanPath, []byte(rv), 0o600); err != nil {
			slog.Error("failed to write resource version bookmark", "path", cleanPath, "error", err)
			return
		}
		mostRecent = rv
		slog.Debug("updated resource version bookmark", "resourceVersion", rv)
	}
	return last, persist
}
