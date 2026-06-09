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

import "testing"

func TestManufactureSink(t *testing.T) {
	tests := []struct {
		name string
		sink string
	}{
		{"stdout", "stdout"},
		{"unknown falls back to stdout", "bogus"},
		{"empty falls back to stdout", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ManufactureSink(tt.sink, "")
			if got == nil {
				t.Fatal("ManufactureSink() returned nil")
			}
			if _, ok := got.(*StdoutSink); !ok {
				t.Fatalf("ManufactureSink() = %T, want *StdoutSink", got)
			}
		})
	}
}
