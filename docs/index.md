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
  - icon:
      light: /icons/light/deploy.svg
      dark: /icons/dark/deploy.svg
    title: One-Command Deploy
    details: Push to GitHub, Talos receives the webhook, pulls the image, verifies health, and switches traffic — zero downtime.
  - icon:
      light: /icons/light/backup.svg
      dark: /icons/dark/backup.svg
    title: Backup & Restore
    details: Scheduled backups with configurable retention, one-click restore, and full backup lifecycle management.
  - icon:
      light: /icons/light/routing.svg
      dark: /icons/dark/routing.svg
    title: Traefik Routing
    details: Automatic domain routing with HTTPS via Let's Encrypt, plus IP:port fallback for quick testing before DNS is ready.
  - icon:
      light: /icons/light/database.svg
      dark: /icons/dark/database.svg
    title: Managed Services
    details: Provision PostgreSQL, MySQL, Redis, and Garage object storage — credentials injected as environment variables.
  - icon:
      light: /icons/light/lock.svg
      dark: /icons/dark/lock.svg
    title: Secure by Default
    details: Encrypted secrets at rest, signed webhook verification, server-side sessions, and nacl encryption for GitHub Actions secrets.
  - icon:
      light: /icons/light/binary.svg
      dark: /icons/dark/binary.svg
    title: Single Binary
    details: One Go binary, one SQLite database, one VPS. No Kubernetes, no cluster complexity, no vendor lock-in.
