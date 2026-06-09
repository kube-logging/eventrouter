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
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/kube-logging/eventrouter/sinks"
)

func main() {
	if err := run(); err != nil {
		slog.Error("eventrouter exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)
	klog.SetSlogLogger(logger)

	clientset, err := newClientset(cfg.Kubeconfig)
	if err != nil {
		return fmt.Errorf("initializing kubernetes client: %w", err)
	}

	lastResourceVersion, persistResourceVersion := resourceVersionBookmark(cfg.BookmarkPath)

	sharedInformers := informers.NewSharedInformerFactory(clientset, cfg.ResyncInterval)
	eventsInformer := sharedInformers.Core().V1().Events()

	eventRouter, err := NewEventRouter(eventsInformer, lastResourceVersion, persistResourceVersion,
		sinks.ManufactureSink(cfg.Sink, cfg.StdoutNamespace), cfg.EnableMetrics)
	if err != nil {
		return fmt.Errorf("creating event router: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Go(func() { serveHTTP(ctx, cfg, eventRouter.Ready) })
	wg.Go(func() { eventRouter.Run(ctx) })

	slog.Info("starting shared informer(s)")
	sharedInformers.Start(ctx.Done())

	wg.Wait()
	slog.Warn("shutting down")
	return nil
}

func newClientset(kubeconfig string) (kubernetes.Interface, error) {
	var (
		config *rest.Config
		err    error
	)
	if kubeconfig != "" {
		slog.Info("using kubeconfig", "path", kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		slog.Info("using in-cluster config")
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("building kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clientset: %w", err)
	}
	return clientset, nil
}

func serveHTTP(ctx context.Context, cfg Config, ready func() bool) {
	mux := http.NewServeMux()
	if cfg.EnableMetrics {
		mux.Handle("/metrics", promhttp.Handler())
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ready() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	srv := &http.Server{
		Addr:         cfg.MetricsAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("starting HTTP server", "addr", cfg.MetricsAddr, "metrics", cfg.EnableMetrics)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("HTTP server error", "error", err)
	}
}
