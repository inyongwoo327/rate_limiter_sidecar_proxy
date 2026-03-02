# rate-limiter-sidecar

A production-grade HTTP reverse proxy written in Go that protects downstream
services from traffic spikes using the **Token Bucket** algorithm. Runs as a
Kubernetes sidecar container with zero external dependencies, full Prometheus
observability, and race-condition-free concurrency verified under 200+ concurrent
virtual users.


```
Client → :8080 [Rate Limiter Sidecar] → localhost:80 [Downstream Service]
                        ↓
                  :9090 /metrics  (Prometheus)
                  :8080 /healthz  (Kubernetes liveness probe)
```

---

## Table of Contents

Most production services eventually face the same problem: one misbehaving
client — a bug in a third-party integration, a runaway retry loop, a scrapers
— floods the downstream service and causes it to fail for everyone else.

The standard solution is rate limiting. But rather than baking it into every
service individually, this project implements it as a **sidecar** — a separate
container that intercepts all traffic before it reaches the real service. The
downstream service stays simple and focused. The sidecar handles the traffic
shaping.

**What this project demonstrates:**

- The Token Bucket algorithm implemented idiomatically in Go
- Concurrent-safe data structures (`sync.Mutex`, `sync.RWMutex`)
- Go's `net/http` middleware pattern
- Kubernetes sidecar container pattern
- Prometheus metrics from scratch (no client library)
- Production-grade testing: unit tests, integration tests, race detector, k6 load testing

## Table of Contents

- [Why This Project](#why-this-project)
- [How It Works](#how-it-works)
- [Prerequisites](#prerequisites)
- [Getting Started — Run in 3 Steps](#getting-started--run-in-3-steps)
- [Configuration](#configuration)
- [Verifying Rate Limiting Works](#verifying-rate-limiting-works)
- [Running Tests](#running-tests)
- [Load Testing with k6](#load-testing-with-k6)
- [Docker Compose — Local Full Stack](#docker-compose--local-full-stack)
- [Kubernetes Deployment](#kubernetes-deployment)
  - [Install KinD](#install-kind)
  - [Create the KinD cluster](#create-the-kind-cluster)
  - [Load image, deploy, test](#load-the-image-into-kind)
- [API Reference](#api-reference)
- [Metrics Reference](#metrics-reference)
- [Design Decisions (ADR)](#design-decisions-adr)
- [Extending the Project](#extending-the-project)
- [Troubleshooting](#troubleshooting)
- [License](#license)

## How It Works

### Token Bucket Algorithm

Think of a bucket that holds tokens — like coins. Each HTTP request reaches
into the bucket and takes one coin. If there are coins, the request goes
through. If the bucket is empty, the request is rejected with `429 Too Many
Requests`.

Over time, coins drip back into the bucket at a fixed rate (`RATE_LIMIT_REFILL_RATE`
tokens per second). The bucket has a maximum capacity (`RATE_LIMIT_CAPACITY`)
— it cannot overflow. This allows short bursts of traffic while enforcing a
long-term average rate.

```
tokens_to_add  = elapsed_seconds × refill_rate
current_tokens = min(capacity, previous_tokens + tokens_to_add)

if current_tokens >= 1:
    current_tokens -= 1
    → forward request (200)
else:
    → reject request (429)
    → Retry-After: ceil(1 / refill_rate)
```

### Lazy Refill

Tokens are **not** refilled by a background goroutine running on a timer.
Instead, refill is calculated on-demand every time a request arrives, based
on how much time has passed since the last request. This is called lazy refill.

Why? A background goroutine per client would create one goroutine for every
unique IP address that ever hits the proxy. Under a DDoS with thousands of
unique source IPs, this would exhaust memory and goroutine budget. Lazy refill
costs O(1) per request with zero goroutine overhead — the idiomatic Go approach.

### Concurrency Safety

The proxy handles many requests at the same time (concurrently). Without
protection, two goroutines could both read `tokens = 1`, both decide "yes,
there's a token", and both subtract 1 — allowing two requests when only one
token existed. That's a race condition.

This project uses two primitives to prevent this:

- `sync.Mutex` on each `TokenBucket` — only one goroutine can read or modify
  a bucket's token count at a time
- `sync.RWMutex` on the client `Store` — many goroutines can look up existing
  clients simultaneously (read lock), but adding a new client requires
  exclusive access (write lock)

Verified with Go's built-in race detector: `go test ./... -race`

---

## Prerequisites

| Tool | Version | Purpose | Install |
|---|---|---|---|
| Go | 1.22+ | Build and run the project | [go.dev/dl](https://go.dev/dl/) |
| Docker | 20.10+ | Build container image and run KinD | [docs.docker.com](https://docs.docker.com/get-docker/) |
| Docker Compose | v2+ | Run local full stack | Bundled with Docker Desktop |
| kubectl | any | Interact with the Kubernetes cluster | [kubernetes.io](https://kubernetes.io/docs/tasks/tools/) |
| KinD | 0.20+ | Local Kubernetes cluster inside Docker | [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) |
| k6 | 0.46+ | Load testing | [k6.io/docs](https://k6.io/docs/get-started/installation/) |

For the quickest start, only Go is required. Docker Compose, kubectl, and k6
are only needed for their respective sections.

**Why KinD over minikube or K3s?**

KinD (Kubernetes in Docker) runs the entire cluster as Docker containers.
Since Docker is already required for this project, KinD adds zero additional
tooling. It starts in ~30 seconds, tears down cleanly, and is the tool used
by Flux, Argo, and most CNCF projects themselves for CI testing. Minikube
spins up a VM (slower, heavier resources), and K3s is designed for real Linux
servers rather than local development on a laptop.

---

