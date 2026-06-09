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
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/kube-logging/eventrouter/sinks"
)

func newEventCounter(suffix, help string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "eventrouter_" + suffix,
		Help: help,
	}, []string{
		"involved_object_kind",
		"involved_object_namespace",
		"reason",
		"source",
	})
}

var (
	warningEvents = newEventCounter("warnings_total", "Total number of warning events in the kubernetes cluster")
	normalEvents  = newEventCounter("normal_total", "Total number of normal events in the kubernetes cluster")
	infoEvents    = newEventCounter("info_total", "Total number of info events in the kubernetes cluster")
	unknownEvents = newEventCounter("unknown_total", "Total number of events of unknown type in the kubernetes cluster")
)

// EventRouter maintains a stream of kubernetes Events and pushes them to a sink.
type EventRouter struct {
	eListerSynched cache.InformerSynced
	eSink          sinks.EventSinkInterface

	lastSeenResourceVersion     string
	lastResourceVersionPosition func(rv string)

	enableMetrics bool
	ready         atomic.Bool
}

// NewEventRouter creates an event router wired to the given events informer and sink.
func NewEventRouter(eventsInformer coreinformers.EventInformer, lastSeenResourceVersion string,
	persist func(rv string), sink sinks.EventSinkInterface, enableMetrics bool,
) (*EventRouter, error) {
	if enableMetrics {
		prometheus.MustRegister(warningEvents, normalEvents, infoEvents, unknownEvents)
	}

	er := &EventRouter{
		eListerSynched:              eventsInformer.Informer().HasSynced,
		eSink:                       sink,
		lastSeenResourceVersion:     lastSeenResourceVersion,
		lastResourceVersionPosition: persist,
		enableMetrics:               enableMetrics,
	}
	if _, err := eventsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    er.addEvent,
		UpdateFunc: er.updateEvent,
		DeleteFunc: er.deleteEvent,
	}); err != nil {
		return nil, fmt.Errorf("registering event handler: %w", err)
	}
	return er, nil
}

// Run starts the controller and blocks until ctx is canceled.
func (er *EventRouter) Run(ctx context.Context) {
	defer utilruntime.HandleCrash()

	slog.Info("starting EventRouter")
	defer slog.Info("shutting down EventRouter")

	if !cache.WaitForCacheSync(ctx.Done(), er.eListerSynched) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}
	er.ready.Store(true)
	<-ctx.Done()
}

// Ready reports whether the informer cache has synced.
func (er *EventRouter) Ready() bool {
	return er.ready.Load()
}

func (er *EventRouter) shouldProcessEvent(resourceVersion string) bool {
	return resourceVersionNewer(resourceVersion, er.lastSeenResourceVersion)
}

func (er *EventRouter) addEvent(obj any) {
	e, ok := obj.(*corev1.Event)
	if !ok || e == nil {
		slog.Error("addEvent: expected a non-nil *v1.Event", "type", fmt.Sprintf("%T", obj))
		return
	}
	if !er.shouldProcessEvent(e.ResourceVersion) {
		slog.Debug("event already processed", "name", e.Name, "resourceVersion", e.ResourceVersion)
		return
	}
	er.recordMetric(e)
	er.eSink.UpdateEvents(e, nil)
	er.lastResourceVersionPosition(e.ResourceVersion)
}

func (er *EventRouter) updateEvent(objOld, objNew any) {
	eNew, okNew := objNew.(*corev1.Event)
	eOld, okOld := objOld.(*corev1.Event)
	if !okNew || eNew == nil || !okOld {
		slog.Error("updateEvent: expected non-nil *v1.Event objects",
			"oldType", fmt.Sprintf("%T", objOld), "newType", fmt.Sprintf("%T", objNew))
		return
	}
	if !er.shouldProcessEvent(eNew.ResourceVersion) {
		slog.Debug("event already processed", "name", eNew.Name, "resourceVersion", eNew.ResourceVersion)
		return
	}
	er.recordMetric(eNew)
	er.eSink.UpdateEvents(eNew, eOld)
	er.lastResourceVersionPosition(eNew.ResourceVersion)
}

// deleteEvent only happens on TTL expiration, so there is nothing to forward.
func (er *EventRouter) deleteEvent(obj any) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	e, ok := obj.(*corev1.Event)
	if !ok || e == nil {
		slog.Error("deleteEvent: expected a non-nil *v1.Event", "type", fmt.Sprintf("%T", obj))
		return
	}
	slog.Debug("event deleted from the system",
		"namespace", e.Namespace, "name", e.Name, "reason", e.Reason, "resourceVersion", e.ResourceVersion)
}

func (er *EventRouter) recordMetric(event *corev1.Event) {
	if !er.enableMetrics {
		return
	}

	safe := func(s string) string {
		if s == "" {
			return "unknown"
		}
		return s
	}
	kind := safe(event.InvolvedObject.Kind)
	namespace := safe(event.InvolvedObject.Namespace)
	reason := safe(event.Reason)
	source := safe(event.Source.Host)

	var counter *prometheus.CounterVec
	switch event.Type {
	case "Normal":
		counter = normalEvents
	case "Warning":
		counter = warningEvents
	case "Info":
		counter = infoEvents
	default:
		counter = unknownEvents
	}

	c, err := counter.GetMetricWithLabelValues(kind, namespace, reason, source)
	if err != nil {
		slog.Error("failed to get prometheus counter", "namespace", namespace, "kind", kind, "error", err)
		return
	}
	c.Add(1)
}
