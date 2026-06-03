# Introduction

Talos is a self-hosted deployment platform for Dockerized applications running on a **single VPS**. It is designed for solo operators and small technical teams who want a simple control plane for deploying and operating apps without adopting a larger platform.

## What Talos Does

- Deploys Dockerized applications from GitHub onto one VPS
- Manages app routing through Talos-managed Traefik
- Supports GitHub Actions webhook-based deployment flow
- Provides backup, restore, and upgrade workflows
- Provisions managed backing services (PostgreSQL, MySQL, Redis, Garage)

## What Talos Is Not

Talos is intentionally scoped. It is not:

- A Kubernetes replacement
- A multi-node orchestrator
- A multi-tenant hosting platform
- A general CI/CD system

## Product Principles

| Principle | Meaning |
|-----------|---------|
| One obvious happy path | The primary deploy workflow should be immediately understandable |
| Reliability before breadth | A smaller tool that works reliably beats a wider tool that breaks |
| Opinionated defaults | Good defaults reduce decision fatigue |
| One VPS is a feature | Simpler mental model, lower operational complexity |

## Quick Start

```bash
# SSH into your VPS and run the installer
curl -fsSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo bash

# Or with Docker mode
curl -fsSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo bash -s -- --docker
```

Then open `http://YOUR_VPS_IP:3000` in your browser, create an admin account, and start deploying.

## Next Steps

- [Installation](/guide/installation) — Detailed installation options
- [Configuration](/guide/configuration) — Environment variables and settings
- [Your First Deploy](/guide/first-deploy) — Deploy an app step by step
