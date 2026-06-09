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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewEventData(t *testing.T) {
	newEvent := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "new"}}
	oldEvent := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "old"}}

	t.Run("nil old event is ADDED", func(t *testing.T) {
		got := NewEventData(newEvent, nil)
		if got.Verb != "ADDED" {
			t.Errorf("Verb = %q, want ADDED", got.Verb)
		}
		if got.Event != newEvent {
			t.Errorf("Event = %v, want the new event", got.Event)
		}
		if got.OldEvent != nil {
			t.Errorf("OldEvent = %v, want nil", got.OldEvent)
		}
	})

	t.Run("non-nil old event is UPDATED", func(t *testing.T) {
		got := NewEventData(newEvent, oldEvent)
		if got.Verb != "UPDATED" {
			t.Errorf("Verb = %q, want UPDATED", got.Verb)
		}
		if got.Event != newEvent {
			t.Errorf("Event = %v, want the new event", got.Event)
		}
		if got.OldEvent != oldEvent {
			t.Errorf("OldEvent = %v, want the old event", got.OldEvent)
		}
	})
}
