/*
Copyright 2017 Heptio Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/heptiolabs/eventrouter/sinks"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cast"
	"github.com/spf13/viper"

	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

var (
	kubernetesWarningEventCounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "heptio_eventrouter_warnings_total",
		Help: "Total number of warning events in the kubernetes cluster",
	}, []string{
		"involved_object_kind",
		"involved_object_name",
		"involved_object_namespace",
		"reason",
		"source",
	})
	kubernetesNormalEventCounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "heptio_eventrouter_normal_total",
		Help: "Total number of normal events in the kubernetes cluster",
	}, []string{
		"involved_object_kind",
		"involved_object_name",
		"involved_object_namespace",
		"reason",
		"source",
	})
	kubernetesInfoEventCounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "heptio_eventrouter_info_total",
		Help: "Total number of info events in the kubernetes cluster",
	}, []string{
		"involved_object_kind",
		"involved_object_name",
		"involved_object_namespace",
		"reason",
		"source",
	})
	kubernetesUnknownEventCounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "heptio_eventrouter_unknown_total",
		Help: "Total number of events of unknown type in the kubernetes cluster",
	}, []string{
		"involved_object_kind",
		"involved_object_name",
		"involved_object_namespace",
		"reason",
		"source",
	})
)

// EventRouter is responsible for maintaining a stream of kubernetes
// system Events and pushing them to another channel for storage
type EventRouter struct {
	// kubeclient is the main kubernetes interface
	kubeClient kubernetes.Interface

	// store of events populated by the shared informer
	eLister corelisters.EventLister

	// returns true if the event store has been synced
	eListerSynched cache.InformerSynced

	// event sink
	// TODO: Determine if we want to support multiple sinks.
	eSink sinks.EventSinkInterface

	lastSeenResourceVersion     string
	lastResourceVersionPosition func(string)
}

// NewEventRouter will create a new event router using the input params
func NewEventRouter(kubeClient kubernetes.Interface, eventsInformer coreinformers.EventInformer,
	lastSeenResourceVersion string, lastResourceVersionPosition func(rv string)) *EventRouter {
	if viper.GetBool("enable-prometheus") {
		prometheus.MustRegister(kubernetesWarningEventCounterVec)
		prometheus.MustRegister(kubernetesNormalEventCounterVec)
		prometheus.MustRegister(kubernetesInfoEventCounterVec)
		prometheus.MustRegister(kubernetesUnknownEventCounterVec)
	}

	er := &EventRouter{
		kubeClient:                  kubeClient,
		eSink:                       sinks.ManufactureSink(),
		lastSeenResourceVersion:     lastSeenResourceVersion,
		lastResourceVersionPosition: lastResourceVersionPosition,
	}
	eventsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    er.addEvent,
		UpdateFunc: er.updateEvent,
		DeleteFunc: er.deleteEvent,
	})
	er.eLister = eventsInformer.Lister()
	er.eListerSynched = eventsInformer.Informer().HasSynced
	return er
}

// Run starts the EventRouter/Controller.
func (er *EventRouter) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer glog.Infof("Shutting down EventRouter")

	glog.Infof("Starting EventRouter")

	// here is where we kick the caches into gear
	if !cache.WaitForCacheSync(stopCh, er.eListerSynched) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}
	<-stopCh
}

// shouldProcessEvent checks if an event should be processed based on resource version
func (er *EventRouter) shouldProcessEvent(resourceVersion string) bool {
	if resourceVersion == "" {
		return false
	}

	if er.lastSeenResourceVersion == "" {
		return true
	}

	return cast.ToInt(er.lastSeenResourceVersion) < cast.ToInt(resourceVersion)
}

// addEvent is called when an event is created, or during the initial list
func (er *EventRouter) addEvent(obj interface{}) {
	e, ok := obj.(*v1.Event)
	if !ok {
		glog.Errorf("Expected *v1.Event, got %T", obj)
		return
	}

	if e == nil {
		glog.Error("Received nil event")
		return
	}

	if er.shouldProcessEvent(e.ResourceVersion) {
		prometheusEvent(e)
		if er.eSink != nil {
			er.eSink.UpdateEvents(e, nil)
		} else {
			glog.Error("Event sink is nil, cannot process event")
			return
		}
		if er.lastResourceVersionPosition != nil {
			er.lastResourceVersionPosition(e.ResourceVersion)
		}
	} else {
		glog.V(5).Infof("Event had already been processed: %s (resource version: %s)", e.Name, e.ResourceVersion)
	}
}

// updateEvent is called any time there is an update to an existing event
func (er *EventRouter) updateEvent(objOld interface{}, objNew interface{}) {
	eOld, okOld := objOld.(*v1.Event)
	if !okOld {
		glog.Errorf("Expected *v1.Event for old object, got %T", objOld)
		return
	}

	eNew, okNew := objNew.(*v1.Event)
	if !okNew {
		glog.Errorf("Expected *v1.Event for new object, got %T", objNew)
		return
	}

	if eNew == nil {
		glog.Error("Received nil new event")
		return
	}

	if er.shouldProcessEvent(eNew.ResourceVersion) {
		prometheusEvent(eNew)
		if er.eSink != nil {
			er.eSink.UpdateEvents(eNew, eOld)
		} else {
			glog.Error("Event sink is nil, cannot process event update")
			return
		}
		if er.lastResourceVersionPosition != nil {
			er.lastResourceVersionPosition(eNew.ResourceVersion)
		}
	} else {
		glog.V(5).Infof("Event had already been processed: %s (resource version: %s)", eNew.Name, eNew.ResourceVersion)
	}
}

// prometheusEvent is called when an event is added or updated
func prometheusEvent(event *v1.Event) {
	if !viper.GetBool("enable-prometheus") {
		return
	}

	if event == nil {
		glog.Error("Cannot record metrics for nil event")
		return
	}

	// Safely get label values with defaults
	safeString := func(s string) string {
		if s == "" {
			return "unknown"
		}
		return s
	}

	kind := safeString(event.InvolvedObject.Kind)
	name := safeString(event.InvolvedObject.Name)
	namespace := safeString(event.InvolvedObject.Namespace)
	reason := safeString(event.Reason)
	sourceHost := safeString(event.Source.Host)

	var counter prometheus.Counter
	var err error

	switch event.Type {
	case "Normal":
		counter, err = kubernetesNormalEventCounterVec.GetMetricWithLabelValues(
			kind, name, namespace, reason, sourceHost)
	case "Warning":
		counter, err = kubernetesWarningEventCounterVec.GetMetricWithLabelValues(
			kind, name, namespace, reason, sourceHost)
	case "Info":
		counter, err = kubernetesInfoEventCounterVec.GetMetricWithLabelValues(
			kind, name, namespace, reason, sourceHost)
	default:
		glog.V(4).Infof("Unknown event type: %s", event.Type)
		counter, err = kubernetesUnknownEventCounterVec.GetMetricWithLabelValues(
			kind, name, namespace, reason, sourceHost)
	}

	if err != nil {
		glog.Errorf("Failed to get Prometheus counter for event %s/%s: %v", namespace, name, err)
	} else {
		counter.Add(1)
		glog.V(6).Infof("Recorded Prometheus metric for event %s/%s (type: %s)", namespace, name, event.Type)
	}
}

// deleteEvent should only occur when the system garbage collects events via TTL expiration
func (er *EventRouter) deleteEvent(obj interface{}) {
	e, ok := obj.(*v1.Event)
	if !ok {
		glog.Errorf("Expected *v1.Event in deleteEvent, got %T", obj)
		return
	}

	if e == nil {
		glog.Error("Received nil event in deleteEvent")
		return
	}

	// NOTE: This should *only* happen on TTL expiration there
	// is no reason to push this to a sink
	glog.V(5).Infof("Event deleted from the system: %s/%s (reason: %s, resource version: %s)",
		e.Namespace, e.Name, e.Reason, e.ResourceVersion)
}
