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

package sinks

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// captureStdout runs fn while collecting everything it writes to os.Stdout.
func captureStdout(t *testing.T, fn func()) []byte {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	return out
}

func TestStdoutSinkUpdateEvents(t *testing.T) {
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "test-event", Namespace: "default"},
		Reason:     "Started",
	}

	t.Run("default emits a bare EventData object", func(t *testing.T) {
		out := captureStdout(t, func() {
			NewStdoutSink("").UpdateEvents(event, nil)
		})

		var got EventData
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("output is not valid EventData JSON: %v (%q)", err, out)
		}
		if got.Verb != "ADDED" {
			t.Errorf("Verb = %q, want ADDED", got.Verb)
		}
		if got.Event == nil || got.Event.Name != "test-event" {
			t.Errorf("Event = %v, want event named test-event", got.Event)
		}
	})

	t.Run("namespace wraps the EventData under the given key", func(t *testing.T) {
		out := captureStdout(t, func() {
			NewStdoutSink("kube").UpdateEvents(event, nil)
		})

		var got map[string]EventData
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("output is not valid namespaced JSON: %v (%q)", err, out)
		}
		wrapped, ok := got["kube"]
		if !ok {
			t.Fatalf("output missing %q key: %v", "kube", got)
		}
		if wrapped.Verb != "ADDED" || wrapped.Event == nil || wrapped.Event.Name != "test-event" {
			t.Errorf("wrapped EventData = %+v, want ADDED event named test-event", wrapped)
		}
	})
}
