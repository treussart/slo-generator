# slo-generator

A tool that generates [Sloth](https://github.com/slok/sloth)-based SLO recording and alerting rules as Helm-templated [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator) `PrometheusRule` manifests.

## Overview

slo-generator builds SLO specifications (availability, ingress availability, latency) from code, runs them through Sloth, and post-processes the output into Helm-compatible YAML. The generated templates include Helm value toggles for enabling/disabling SLOs and alerting, proper `{{ }}` escaping, and standard Helm metadata.

Three implementations are provided:

- **Go** (primary) — uses Sloth as a library, no external binary needed.
- **Python** — shells out to a prebuilt `sloth` CLI binary.
- **Helm Hand written** - uses Helm templates to generate the manifests.

### Supported SLO types

| Type | Metrics source |
|------|---------------|
| HTTP availability | `handler_http_response_seconds_count` |
| Nginx availability | `nginx_http_requests_total` |
| Ingress availability | `nginx_ingress_controller_request_duration_seconds_count` |
| Latency | `handler_http_response_seconds_bucket` (histogram) |

### Priority-to-objective mapping

| Priority | Objective |
|----------|-----------|
| `priority-critical` | 99.9% |
| `priority-high` | 99.5% |
| `priority-medium` | 99.0% |
| `priority-low` | 99.0% |

## Project structure

```
go/               Go module — main generator implementation
  main.go         Entrypoint
  config.go       Environment-based configuration
  generate.go     SLO spec building, Sloth invocation, post-processing
  go.mod / go.sum Module dependencies
python/           Python alternative using external sloth binary
  generate-slo.py YAML-based SLO generation script
templates/        Reference handwritten PrometheusRule + values examples
```
