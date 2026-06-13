# App Management

Talos provides full lifecycle management for applications through the web UI and REST API.

## Application Model

Each application in Talos has the following properties:

| Field | Description | Example |
|-------|-------------|---------|
| **Name** | Unique identifier, used in container names and URLs | `my-app` |
| **App Type** | `managed`, `adopted_container`, or `external_service` | `managed` |
| **Build Mode** | `external_ci` or `talos_build` | `external_ci` |
| **Project Type** | Build detection override: `auto`, `static`, `node`, `go`, or `java` | `auto` |
| **Source** | Where the app comes from | `github` |
| **Repository URL** | Git repository URL | `https://github.com/org/my-app` |
| **Branch** | Default deployment branch | `main` |
| **Internal Port** | Port the app listens on inside the container | `3000` |
| **Image Ref** | Current container image | `ghcr.io/org/my-app:abc1234` |
| **Domain** | Custom domain for domain-mode routing | `app.example.com` |
| **Fallback Port** | External port for IP-mode access | `8080` |
| **Access Mode** | `domain` or `port` | `port` |
| **Status** | `active`, `inactive`, or `error` | `active` |

### Build Modes

| Mode | Description |
|------|-------------|
| `external_ci` | GitHub Actions builds and pushes the image. Talos deploys on webhook notification. |
| `talos_build` | Talos clones the repo and builds the image locally on push events. |

### Project Types

When using **Talos Build** mode and no `Dockerfile` exists in the repository, Talos auto-detects the project type by inspecting sentinel files (e.g. `package.json`, `go.mod`, `pom.xml`). The **Project Type** field lets you override this detection.

| Value | Detection Sentinel | Generated Dockerfile | Default Port |
|-------|-------------------|---------------------|--------------|
| `auto` (default) | Auto-detected from repo files | Depends on detected type | Depends on detected type |
| `static` | `index.html` | nginx serving static files | 80 |
| `node` | `package.json` | Node.js or Bun runtime | 3000 |
| `go` | `go.mod` | Go build + scratch runtime | 8080 |
| `java` | `pom.xml` or `build.gradle` | Maven/Gradle build + JDK runtime | 8080 |

**When to use a specific type:**

- **Monorepos** where detection picks the wrong provider (e.g. a Go repo with a root `package.json` for tooling)
- **Unusual layouts** where sentinel files are in non-standard locations
- **Overriding default port** behavior (the port is set by the provider, not the form field)

**How it works:**

1. If a `Dockerfile` exists in the repo root, Talos uses it directly — Project Type is ignored.
2. If no `Dockerfile` exists and Project Type is `auto`, Talos runs auto-detection.
3. If no `Dockerfile` exists and Project Type is set to a specific value, Talos skips detection and uses that provider directly.
4. If the chosen provider cannot generate a plan (e.g. `go` selected but no `go.mod` found), the deploy fails at build time with a descriptive error.

::: tip
Project Type only affects **Talos Build** mode. In **External CI** mode, the build happens in GitHub Actions and Talos only deploys the resulting image.
:::

## CRUD Operations

### Create an App

**Web UI:** Navigate to Apps > New App and fill in the form.

**API:**

```bash
curl -X POST http://localhost:3000/api/apps \
  -H "Content-Type: application/json" \
  -H "Cookie: session=<your-session-cookie>" \
  -d '{
    "name": "my-app",
    "app_type": "managed",
    "build_mode": "external_ci",
    "repo_url": "https://github.com/org/my-app",
    "branch": "main",
    "internal_port": 3000,
    "access_mode": "port",
    "fallback_port": 8080
  }'
```

**With Talos Build mode (auto-detect project type):**

```bash
curl -X POST http://localhost:3000/api/apps \
  -H "Content-Type: application/json" \
  -H "Cookie: session=<your-session-cookie>" \
  -d '{
    "name": "my-app",
    "app_type": "managed",
    "build_mode": "talos_build",
    "repo_url": "https://github.com/org/my-app",
    "branch": "main",
    "internal_port": 3000,
    "access_mode": "port",
    "fallback_port": 8080
  }'
```

**With Talos Build mode and explicit project type:**

```bash
curl -X POST http://localhost:3000/api/apps \
  -H "Content-Type: application/json" \
  -H "Cookie: session=<your-session-cookie>" \
  -d '{
    "name": "my-go-app",
    "app_type": "managed",
    "build_mode": "talos_build",
    "project_type": "go",
    "repo_url": "https://github.com/org/my-go-app",
    "branch": "main",
    "internal_port": 8080,
    "access_mode": "port",
    "fallback_port": 8081
  }'
```

When `project_type` is omitted or set to `""`, Talos auto-detects from repo files. Set it to `static`, `node`, `go`, or `java` to skip detection.

### List Apps

**Web UI:** The dashboard shows all applications with their current status.

**API:**

```bash
curl http://localhost:3000/api/apps \
  -H "Cookie: session=<your-session-cookie>"
```

### Get App Details

**Web UI:** Click on an app name to see its detail page.

**API:**

```bash
curl http://localhost:3000/api/apps/{appID} \
  -H "Cookie: session=<your-session-cookie>"
```

### Update an App

**Web UI:** Use the app settings page to modify configuration.

**API:**

```bash
curl -X PUT http://localhost:3000/api/apps/{appID} \
  -H "Content-Type: application/json" \
  -H "Cookie: session=<your-session-cookie>" \
  -d '{
    "branch": "develop",
    "internal_port": 8080
  }'
```

### Delete an App

**Web UI:** Click Delete on the app detail page.

**API:**

```bash
curl -X DELETE http://localhost:3000/api/apps/{appID} \
  -H "Cookie: session=<your-session-cookie>"
```

:::warning
Deleting an app removes its deploy history, environment variables, and service links. The running container is stopped and removed. This action cannot be undone.
:::

## Deploy Status

Each app tracks its current deployment state:

| Status | Description |
|--------|-------------|
| `active` | App has a running container |
| `inactive` | App exists but has no running container |
| `error` | Last deploy failed |

The app detail page shows:

- Current image reference
- Live container name
- Current deploy ID
- Access URL
- Full deploy history

## Deploy History

Every deployment is recorded with:

- Image reference and commit SHA
- Trigger source (webhook, manual, rollback)
- Status (pending, running, success, failed, auto_rollback)
- Start and completion timestamps
- Environment variable snapshot
- Structured events for each pipeline step

### Trigger a Deploy

**Web UI:** Click Deploy on the app detail page.

**API:**

```bash
curl -X POST http://localhost:3000/api/apps/{appID}/deploys \
  -H "Content-Type: application/json" \
  -H "Cookie: session=<your-session-cookie>" \
  -d '{
    "image_ref": "ghcr.io/org/my-app:abc1234"
  }'
```

### View Deploy Events

Each deploy emits structured events at every step:

```bash
curl http://localhost:3000/api/deploys/{deployID}/events \
  -H "Cookie: session=<your-session-cookie>"
```

Response:

```json
[
  {
    "id": 1,
    "deploy_id": 42,
    "timestamp": "2025-01-01T12:00:00Z",
    "level": "info",
    "step": "start",
    "message": "deploy started for my-app with image ghcr.io/org/my-app:abc1234"
  },
  {
    "id": 2,
    "deploy_id": 42,
    "timestamp": "2025-01-01T12:00:01Z",
    "level": "info",
    "step": "pull",
    "message": "pulling image ghcr.io/org/my-app:abc1234"
  }
]
```

## Rollback

Rollback redeploys the image from the last successful deploy:

**Web UI:** Click Rollback on the app detail page.

**API:**

```bash
curl -X POST http://localhost:3000/api/apps/{appID}/deploys/rollback \
  -H "Cookie: session=<your-session-cookie>"
```

The rollback follows the same blue/green process as a regular deploy -- the previous image is health-checked before traffic switches over.

## Environment Variables

Manage per-app environment variables through the app settings page or API.

### Set an Environment Variable

```bash
curl -X POST http://localhost:3000/api/apps/{appID}/env \
  -H "Content-Type: application/json" \
  -H "Cookie: session=<your-session-cookie>" \
  -d '{
    "key": "DATABASE_URL",
    "value": "postgres://user:pass@host/db",
    "is_secret": true,
    "required": true
  }'
```

### List Environment Variables

```bash
curl http://localhost:3000/api/apps/{appID}/env \
  -H "Cookie: session=<your-session-cookie>"
```

### View Change History

```bash
curl http://localhost:3000/api/apps/{appID}/env/DATABASE_URL/history \
  -H "Cookie: session=<your-session-cookie>"
```

### Reveal a Secret Value

```bash
curl http://localhost:3000/api/apps/{appID}/env/DATABASE_URL/reveal \
  -H "Cookie: session=<your-session-cookie>"
```

## Service Linking

Link managed services to apps to inject connection credentials automatically:

```bash
# Link a service
curl -X POST http://localhost:3000/api/apps/{appID}/services \
  -H "Content-Type: application/json" \
  -H "Cookie: session=<your-session-cookie>" \
  -d '{
    "service_id": 1,
    "alias": "DB"
  }'

# List linked services
curl http://localhost:3000/api/apps/{appID}/services \
  -H "Cookie: session=<your-session-cookie>"

# Unlink a service
curl -X DELETE http://localhost:3000/api/apps/{appID}/services/{serviceID} \
  -H "Cookie: session=<your-session-cookie>"
```

## Live Logs

Stream container logs in real time:

```bash
curl http://localhost:3000/api/apps/{appID}/logs/stream \
  -H "Cookie: session=<your-session-cookie>"
```

The web UI provides a log viewer on the app detail page.

## Next Steps

- [Managed Services](./managed-services.md) -- add databases and caches
- [Routing](./routing.md) -- configure domains and access
- [First Deployment](../guide/first-deploy.md) -- deploy your first app
