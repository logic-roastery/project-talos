# Deployment Flow

Talos uses blue/green deployments to ensure zero-downtime releases. This page describes the deployment process in detail.

## Blue/Green Deployment Concept

In a blue/green deployment, two versions of an application run simultaneously:

- **Blue** -- the current live version serving production traffic
- **Green** -- the new version being validated

Traffic only switches to the green version after it passes health checks. If the green version fails, it is destroyed and the blue version continues serving traffic unchanged.

```
Before deploy:
  Traefik --> [Blue Container] (live)

During deploy:
  Traefik --> [Blue Container] (live)
             [Green Container] (staging, health checking)

After successful deploy:
  Traefik --> [Green Container] (now live)
             [Blue Container] (stopped and removed)

After failed deploy:
  Traefik --> [Blue Container] (still live)
             [Green Container] (destroyed)
```

## Deployment Modes

Talos supports two automatic deployment modes for managed apps:

| Mode | Trigger | Build Location | Best For |
|------|---------|----------------|----------|
| **External CI** | workflow_run webhook | GitHub Actions | Complex pipelines, existing CI |
| **Talos Build** | push webhook | Talos server | Simple projects, no external CI |
| **Manual** | User action | N/A (user provides image) | Fallback, testing |

## Deployment Steps

A deployment progresses through the following steps. Each step emits a structured event stored in the `deploy_events` table.

### 1. Initialization

- A deploy is triggered (manual, webhook, push event, or rollback).
- Talos identifies the app using stable GitHub repo identity.
- Branch is validated against app configuration.
- App type is validated (only managed apps support auto-deploy).
- Talos checks that no other deploy is currently running for this app.
- A deploy record is created with status `pending`.

```
Event: level=info, step=start, message="deploy started for my-app with image ghcr.io/org/app:abc1234"
```

### 2. Validation

- Required environment variables are checked.
- If any required variable is missing, the deploy fails immediately.

```
Event: level=info, step=start, message="validation passed"
-- or --
Event: level=error, step=start, message="validation failed: missing required env vars: DATABASE_URL"
```

### 3. Environment Snapshot

- A snapshot of all current environment variables is captured.
- The snapshot is stored in the deploy record for diff visibility.

### 4. Image Pull

- The target container image is pulled from the registry.
- This may take time depending on image size and network speed.

```
Event: level=info, step=pull, message="pulling image ghcr.io/org/app:abc1234"
Event: level=info, step=pull, message="image pulled successfully"
-- or --
Event: level=error, step=pull, message="pull failed: ..."
```

### 5. Staging Container Start

- A new container is created with the naming pattern `talos-<app-name>-<deploy-id>`.
- Environment variables from linked services and app-level env vars are injected.
- The container starts on the `talos` Docker network.

```
Event: level=info, step=start, message="starting staging container talos-my-app-42"
Event: level=info, step=start, message="staging container started: <container-id>"
```

### 6. Health Check

- Talos waits for the staging container to become healthy (30-second timeout).
- Docker's built-in health check mechanism is used.

```
Event: level=info, step=health_check, message="waiting for health check (30s timeout)"
```

**If the health check passes:**

```
Event: level=info, step=health_check, message="health check passed"
```

**If the health check fails:**

```
Event: level=error, step=health_check, message="health check failed: context deadline exceeded"
Event: level=info, step=auto_rollback, message="stopping staging container, live container preserved"
Event: level=info, step=auto_rollback, message="auto-rollback complete, previous version still live"
```

The deploy is marked as `auto_rollback` and the process stops here.

### 7. Route Update

- Traefik configuration is updated to point to the staging container.
- Traefik picks up the change via file watching.

```
Event: level=info, step=route_update, message="updating traefik route to talos-my-app-42"
Event: level=info, step=route_update, message="route updated successfully"
```

### 8. Old Container Stop

- The previous live container is stopped and removed.
- This only happens after the new container is healthy and routed.

```
Event: level=info, step=stop_old, message="stopping old container talos-my-app"
Event: level=info, step=stop_old, message="old container stopped"
```

### 9. Finalization

- The deploy record is updated to `success`.
- The app record is updated with the new image ref, deploy ID, and container name.
- The app status is set to `active`.

```
Event: level=info, step=finalize, message="deploy completed successfully"
```

## Deploy Statuses

| Status | Description |
|--------|-------------|
| `pending` | Deploy created, waiting to start |
| `running` | Deploy in progress |
| `success` | Deploy completed successfully |
| `failed` | Deploy failed (image pull, start, or route update error) |
| `auto_rollback` | Health check failed, staging container destroyed, old container still live |
| `rollback` | Manual rollback triggered by user |

## Rollback

A rollback is a deploy that targets the image from the last successful deploy:

1. Talos finds the most recent successful deploy for the app.
2. A new deploy is created with that image, marked as `triggered_by: rollback`.
3. The same blue/green process runs -- the rollback image is health-checked before switching.

This means rollbacks are safe and follow the same zero-downtime guarantees as regular deploys.

## Environment Variable Handling

During deployment, environment variables are collected from two sources:

### App-Level Variables

Defined in the app settings. Support:

- **Required** -- deploy fails if missing
- **Secret** -- masked in UI and API responses
- **History** -- previous values are recorded on change

### Service-Linked Variables

When an app is linked to a managed service, Talos injects connection variables automatically:

| Service Type | Injected Variables |
|-------------|-------------------|
| PostgreSQL | `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` |
| MySQL | `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_DATABASE`, `MYSQL_USER`, `MYSQL_PASSWORD` |
| Redis | `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD` |
| Garage | `GARAGE_ENDPOINT`, `GARAGE_REGION`, `GARAGE_ACCESS_KEY`, `GARAGE_SECRET_KEY`, `GARAGE_BUCKET` |

Variables can be prefixed with an alias when linking a service to an app.

## Concurrency

Only one deploy can run per application at a time. If a deploy is already in progress (`status: running`), new deploy requests are rejected with an error.

Different applications can deploy concurrently -- each app has its own deploy queue.

## Next Steps

- [Data Model](./data-model.md) -- deploy and event table schemas
- [Components](./components.md) -- deploy engine internals
- [First Deployment](../guide/first-deploy.md) -- hands-on deployment guide
