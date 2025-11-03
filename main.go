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
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cast"
	"github.com/spf13/viper"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// addr tells us what address to have the Prometheus metrics listen on.
var addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")

// setup a signal hander to gracefully exit
func sigHandler() <-chan struct{} {
	stop := make(chan struct{})
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c,
			syscall.SIGINT,  // Ctrl+C
			syscall.SIGTERM, // Termination Request
			syscall.SIGSEGV, // FullDerp
			syscall.SIGABRT, // Abnormal termination
			syscall.SIGILL,  // illegal instruction
			syscall.SIGFPE) // floating point - this is why we can't have nice things
		sig := <-c
		glog.Warningf("Signal (%v) Detected, Shutting Down", sig)
		close(stop)
	}()
	return stop
}

// loadConfig will parse input + config file and return a clientset
func loadConfig() (kubernetes.Interface, error) {
	var config *rest.Config
	var err error

	flag.Parse()

	// leverages a file|(ConfigMap)
	// to be located at /etc/eventrouter/config
	viper.SetConfigType("json")
	viper.SetConfigName("config")
	viper.AddConfigPath("/etc/eventrouter/")
	viper.AddConfigPath(".")
	viper.SetDefault("kubeconfig", "")
	viper.SetDefault("sink", "stdout")
	viper.SetDefault("resync-interval", time.Minute*30)
	viper.SetDefault("enable-prometheus", true)

	if err = viper.ReadInConfig(); err != nil {
		glog.Warningf("Failed to read config file, using defaults: %v", err)
		// Continue with default values instead of failing
	} else {
		glog.Infof("Using config file: %s", viper.ConfigFileUsed())
	}

	viper.BindEnv("kubeconfig") // Allows the KUBECONFIG env var to override where the kubeconfig is

	// Allow specifying a custom config file via the EVENTROUTER_CONFIG env var
	if forceCfg := os.Getenv("EVENTROUTER_CONFIG"); forceCfg != "" {
		viper.SetConfigFile(forceCfg)
		if err = viper.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read forced config file %s: %w", forceCfg, err)
		}
	}

	kubeconfig := viper.GetString("kubeconfig")
	if len(kubeconfig) > 0 {
		glog.Infof("Using kubeconfig: %s", kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		glog.Info("Using in-cluster config")
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	// creates the clientset from kubeconfig
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	glog.Info("Successfully initialized Kubernetes client")
	return clientset, nil
}

// main entry point of the program
func main() {
	var wg sync.WaitGroup

	clientset, err := loadConfig()
	if err != nil {
		glog.Errorf("Failed to initialize: %v", err)
		os.Exit(1)
	}

	var lastResourceVersionPosition string
	var mostRecentResourceVersion *string

	resourceVersionPositionPath := viper.GetString("lastResourceVersionPositionPath")

	// Helper function to safely compare resource versions
	safeCompareResourceVersions := func(rv1, rv2 string) bool {
		if rv1 == "" && rv2 == "" {
			return false
		}
		if rv1 == "" {
			return false
		}
		if rv2 == "" {
			return true
		}

		v1 := cast.ToInt(rv1)
		v2 := cast.ToInt(rv2)
		return v1 > v2
	}

	resourceVersionPositionFunc := func(resourceVersion string) {
		if resourceVersionPositionPath == "" || resourceVersion == "" {
			return
		}

		currentVersion := ""
		if mostRecentResourceVersion != nil {
			currentVersion = *mostRecentResourceVersion
		}

		if safeCompareResourceVersions(resourceVersion, currentVersion) {
			err := os.WriteFile(resourceVersionPositionPath, []byte(resourceVersion), 0600)
			if err != nil {
				glog.Errorf("Failed to write resource version position to %s: %v", resourceVersionPositionPath, err)
			} else {
				mostRecentResourceVersion = &resourceVersion
				glog.V(4).Infof("Updated resource version position to %s", resourceVersion)
			}
		}
	}

	if resourceVersionPositionPath != "" {
		if _, err := os.Stat(resourceVersionPositionPath); err == nil {
			resourceVersionBytes, err := os.ReadFile(resourceVersionPositionPath)
			if err != nil {
				glog.Errorf("Failed to read resource version bookmark from %s: %v", resourceVersionPositionPath, err)
			} else {
				lastResourceVersionPosition = string(resourceVersionBytes)
				mostRecentResourceVersion = &lastResourceVersionPosition
				glog.Infof("Resuming from resource version: %s", lastResourceVersionPosition)
			}
		} else if !os.IsNotExist(err) {
			glog.Errorf("Error checking resource version position file %s: %v", resourceVersionPositionPath, err)
		} else {
			glog.Infof("No existing resource version position file found, starting fresh")
		}
	}

	sharedInformers := informers.NewSharedInformerFactory(clientset, viper.GetDuration("resync-interval"))
	eventsInformer := sharedInformers.Core().V1().Events()

	// TODO: Support locking for HA https://github.com/kubernetes/kubernetes/pull/42666
	eventRouter := NewEventRouter(clientset, eventsInformer, lastResourceVersionPosition, resourceVersionPositionFunc)
	stop := sigHandler()

	// Startup the http listener for Prometheus Metrics endpoint.
	if viper.GetBool("enable-prometheus") {
		go func() {
			glog.Info("Starting prometheus metrics.")
			http.Handle("/metrics", promhttp.Handler())
			glog.Warning(http.ListenAndServe(*addr, nil))
		}()
	}

	// Startup the EventRouter
	wg.Add(1)
	go func() {
		defer wg.Done()
		eventRouter.Run(stop)
	}()

	// Startup the Informer(s)
	glog.Infof("Starting shared Informer(s)")
	sharedInformers.Start(stop)
	wg.Wait()
	glog.Warningf("Exiting main()")
	os.Exit(1)
}
