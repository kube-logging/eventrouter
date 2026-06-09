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
	"os"
	"path/filepath"
	"testing"
)

func TestResourceVersionNewer(t *testing.T) {
	tests := []struct {
		name    string
		rv      string
		current string
		want    bool
	}{
		{"both empty", "", "", false},
		{"empty rv", "", "5", false},
		{"empty current is older than anything", "5", "", true},
		{"strictly newer", "5", "3", true},
		{"older", "3", "5", false},
		{"equal", "5", "5", false},
		{"numeric not lexical", "100", "99", true},
		{"unparseable rv is not newer", "abc", "3", false},
		{"unparseable current treated as older", "10", "abc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resourceVersionNewer(tt.rv, tt.current); got != tt.want {
				t.Errorf("resourceVersionNewer(%q, %q) = %v, want %v", tt.rv, tt.current, got, tt.want)
			}
		})
	}
}

func TestValidatedPath(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"absolute clean", "/etc/eventrouter/config.json", "/etc/eventrouter/config.json", false},
		{"collapses slashes", "/foo//bar", "/foo/bar", false},
		{"rejects parent traversal", "/var/lib/eventrouter/../etc/passwd", "", true},
		{"rejects relative", "relative/path", "", true},
		{"rejects empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validatedPath(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validatedPath(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("validatedPath(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("validatedPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResourceVersionBookmark(t *testing.T) {
	readFile := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading bookmark: %v", err)
		}
		return string(data)
	}

	t.Run("empty path disables persistence", func(t *testing.T) {
		last, persist := resourceVersionBookmark("")
		if last != "" {
			t.Fatalf("want empty last, got %q", last)
		}
		persist("5") // must be a safe no-op
	})

	t.Run("invalid path disables persistence", func(t *testing.T) {
		last, persist := resourceVersionBookmark("relative/path")
		if last != "" {
			t.Fatalf("want empty last, got %q", last)
		}
		persist("5") // must be a safe no-op
	})

	t.Run("persists only strictly newer versions", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bookmark")
		last, persist := resourceVersionBookmark(path)
		if last != "" {
			t.Fatalf("want empty last for missing file, got %q", last)
		}

		persist("5")
		if got := readFile(t, path); got != "5" {
			t.Fatalf("after persist(5): got %q, want 5", got)
		}
		persist("3") // older, ignored
		if got := readFile(t, path); got != "5" {
			t.Fatalf("after persist(3): got %q, want 5", got)
		}
		persist("10")
		if got := readFile(t, path); got != "10" {
			t.Fatalf("after persist(10): got %q, want 10", got)
		}
	})

	t.Run("resumes from existing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bookmark")
		if err := os.WriteFile(path, []byte("7"), 0o600); err != nil {
			t.Fatal(err)
		}
		last, persist := resourceVersionBookmark(path)
		if last != "7" {
			t.Fatalf("want last 7, got %q", last)
		}
		persist("5") // older than the loaded 7, ignored
		if got := readFile(t, path); got != "7" {
			t.Fatalf("after persist(5): got %q, want 7", got)
		}
		persist("9")
		if got := readFile(t, path); got != "9" {
			t.Fatalf("after persist(9): got %q, want 9", got)
		}
	})
}
