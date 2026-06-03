---
layout: home

hero:
  name: Talos
  text: Self-hosted deployment platform
  tagline: Deploy and manage Dockerized applications on a single VPS with a simple, reliable control plane.
  actions:
    - theme: brand
      text: Get Started
      link: /guide/
    - theme: alt
      text: View on GitHub
      link: https://github.com/logic-roastery/project-talos

features:
  - icon: 🚀
    title: One-Command Deploy
    details: Push to GitHub, Talos receives the webhook, pulls the image, verifies health, and switches traffic — zero downtime.
  - icon: 🔄
    title: Backup & Restore
    details: Scheduled backups with configurable retention, one-click restore, and full backup lifecycle management.
  - icon: 🌐
    title: Traefik Routing
    details: Automatic domain routing with HTTPS via Let's Encrypt, plus IP:port fallback for quick testing before DNS is ready.
  - icon: 🗄️
    title: Managed Services
    details: Provision PostgreSQL, MySQL, Redis, and Garage object storage — credentials injected as environment variables.
  - icon: 🔐
    title: Secure by Default
    details: Encrypted secrets at rest, signed webhook verification, server-side sessions, and nacl encryption for GitHub Actions secrets.
  - icon: 📦
    title: Single Binary
    details: One Go binary, one SQLite database, one VPS. No Kubernetes, no cluster complexity, no vendor lock-in.
