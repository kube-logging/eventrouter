# Eventrouter

This repository contains a simple event router for the [Kubernetes][kubernetes] project. The event router serves as an active watcher of _event_ resource in the kubernetes system, which takes those events and _pushes_ them to a user specified _sink_.  This is useful for a number of different purposes, but most notably long term behavioral analysis of your
workloads running on your kubernetes cluster.

## Goals

This project has several objectives, which include:

* Persist events for longer period of time to allow for system debugging
* Allows operators to forward events to other system(s) for archiving/ML/introspection/etc.
* It should be relatively low overhead
* Emit events to a pluggable _sink_

### NOTE

eventrouter writes each event as a structured JSON object to **stdout**, ready to be
collected by a node-level log forwarder (for example [Fluent Bit][fluentbit] or
[Fluentd][fluentd]) and shipped to your logging backend. `stdout` is currently the
only built-in sink.

## Non-Goals

* This service does not provide a querable extension, that is a responsibility of the
_sink_
* This service does not serve as a storage layer, that is also the responsibility of the _sink_

## Running Eventrouter

Standup:

```sh
kubectl apply -f https://raw.githubusercontent.com/kube-logging/eventrouter/master/deploy/eventrouter.yaml
```

Teardown:

```sh
kubectl delete -f https://raw.githubusercontent.com/kube-logging/eventrouter/master/deploy/eventrouter.yaml
```

The manifest deploys eventrouter into the `kube-system` namespace with a `stdout` sink.
See [`deploy/eventrouter.yaml`](deploy/eventrouter.yaml) for the full example and tweak it to fit your cluster.

### Inspecting the output

```sh
kubectl logs -f deployment/eventrouter -n kube-system
```

Watch events roll through the system as JSON, ready to be picked up by your log pipeline.
Events are written to **stdout**; application logs are written to **stderr** (structured,
JSON by default) so the two streams never mix.

## Configuration

eventrouter is configured entirely through environment variables — all are optional:

| Variable | Default | Description |
|----------|---------|-------------|
| `EVENTROUTER_SINK` | `stdout` | Sink to emit events to. Only `stdout` is built in. |
| `EVENTROUTER_STDOUT_NAMESPACE` | _(empty)_ | If set, wraps each event under this key in the JSON output. |
| `EVENTROUTER_RESYNC_INTERVAL` | `30m` | Informer resync interval (Go duration). |
| `EVENTROUTER_METRICS_ADDR` | `:8080` | Address for the metrics + health HTTP server. |
| `EVENTROUTER_ENABLE_METRICS` | `true` | Enable the Prometheus `/metrics` endpoint. |
| `EVENTROUTER_BOOKMARK_PATH` | _(empty)_ | Absolute path to persist the last seen resource version, to resume after a restart. |
| `EVENTROUTER_KUBECONFIG` / `KUBECONFIG` | _(empty)_ | Path to a kubeconfig; defaults to in-cluster config. |
| `EVENTROUTER_LOG_FORMAT` | `json` | Log format: `json` or `text`. |
| `EVENTROUTER_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, or `error`. |

## Observability

The HTTP server on `EVENTROUTER_METRICS_ADDR` exposes:

* `GET /metrics` — Prometheus counters (`eventrouter_{normal,warning,info,unknown}_total`)
* `GET /healthz` — liveness (always `200` while the process is up)
* `GET /readyz` — readiness (`200` only once the informer cache has synced)

[kubernetes]: https://github.com/kubernetes/kubernetes/ "Kubernetes"
[fluentbit]: https://fluentbit.io/ "Fluent Bit"
[fluentd]: https://www.fluentd.org/ "Fluentd"

## Contributing

If you find this project useful, help us

* Support the development of this project and star this repo! :star:
* Help new users with issues they may encounter. :muscle:
* Send a pull request with your new features and bug fixes. :rocket:

## License

The project is licensed under the [Apache 2.0 License](LICENSE).
