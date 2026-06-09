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
	"log/slog"

	corev1 "k8s.io/api/core/v1"
)

// EventSinkInterface is the interface used to shunt events.
type EventSinkInterface interface {
	UpdateEvents(eNew, eOld *corev1.Event)
}

// ManufactureSink builds a sink for the given name, falling back to stdout.
func ManufactureSink(sink, stdoutNamespace string) EventSinkInterface {
	switch sink {
	case "stdout":
		return NewStdoutSink(stdoutNamespace)
	default:
		slog.Warn("unknown or unsupported sink, defaulting to stdout", "sink", sink)
		return NewStdoutSink("")
	}
}
