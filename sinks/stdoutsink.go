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
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
)

// StdoutSink writes each event as a single JSON line to stdout.
type StdoutSink struct {
	namespace string
}

// NewStdoutSink returns a StdoutSink; a non-empty namespace wraps each event under that key.
func NewStdoutSink(namespace string) EventSinkInterface {
	return &StdoutSink{namespace: namespace}
}

// UpdateEvents implements EventSinkInterface.
func (s *StdoutSink) UpdateEvents(eNew, eOld *corev1.Event) {
	var payload any = NewEventData(eNew, eOld)
	if s.namespace != "" {
		payload = map[string]any{s.namespace: payload}
	}

	out, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to serialize event", "error", err)
		return
	}
	fmt.Println(string(out))
}
