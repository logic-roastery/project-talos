# First Deployment

This guide walks you through deploying your first application with Talos, from creating the app to verifying the live deployment.

## Prerequisites

Before you begin, make sure you have:

- Talos installed and running (see [Installation](./installation.md))
- The Talos web UI accessible at `http://<your-server-ip>:3000`
- An admin account created via the setup wizard
- A Docker image available in a registry (e.g., GHCR, Docker Hub)

## Step 1: Create an Application

1. Open the Talos web UI in your browser.
2. Navigate to **Apps** and click **New App**.
3. Fill in the application details:

| Field | Description | Example |
|-------|-------------|---------|
| **Name** | Unique identifier for your app | `my-app` |
| **Repository URL** | Git repository URL | `https://github.com/org/my-app` |
| **Branch** | Default branch to deploy | `main` |
| **Internal Port** | Port your app listens on inside the container | `3000` |
| **Access Mode** | How to access the app: `domain` or `port` | `port` |
| **Domain** | Custom domain (if access mode is `domain`) | `app.example.com` |
| **Fallback Port** | External port for IP-based access | `8080` |

4. Click **Create App**.

:::tip
If you choose **domain mode**, make sure your DNS A record points to your server IP. Talos will automatically configure Traefik with HTTPS via Let's Encrypt.
:::

## Step 2: Set Up GitHub Actions Workflow

To enable automatic deployments on push, add a GitHub Actions workflow to your repository. Create the file `.github/workflows/deploy.yml`:

```yaml
name: Build and Deploy

on:
  push:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build-and-push:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=sha,prefix=

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}

  deploy:
    needs: build-and-push
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'

    steps:
      - name: Trigger Talos deploy
        run: |
          curl -X POST "${{ secrets.TALOS_URL }}/api/webhooks/github" \
            -H "Content-Type: application/json" \
            -H "X-GitHub-Event: workflow_run" \
            -H "X-Hub-Signature-256: sha256=${{ secrets.WEBHOOK_SIGNATURE }}" \
            -d '{
              "action": "completed",
              "workflow_run": {
                "status": "completed",
                "conclusion": "success",
                "head_sha": "${{ github.sha }}",
                "head_branch": "main"
              },
              "repository": {
                "full_name": "${{ github.repository }}"
              }
            }'
```

:::warning
Store your Talos webhook secret and URL as GitHub repository secrets. Never commit them directly to your repository.
:::

### Alternative: GitHub App Integration

For a more robust integration, set up a GitHub App through the Talos UI:

1. Go to **Settings > GitHub Setup** in the Talos web UI.
2. Follow the wizard to create and install a GitHub App.
3. The app will automatically receive `workflow_run` webhooks and trigger deploys.

## Step 3: Push Your Code

Commit and push your workflow file along with your application code:

```bash
git add .
git commit -m "ci: add deploy workflow"
git push origin main
```

Your CI pipeline will build the Docker image, push it to the registry, and trigger a deployment in Talos.

## Step 4: Verify the Deployment

1. Open the Talos web UI and navigate to your app's detail page.
2. You should see a new deploy entry with status **running**.
3. The deployment page shows real-time events:
   - `start` -- deploy initiated
   - `pull` -- image being pulled
   - `start` -- staging container started
   - `health_check` -- waiting for health check (30s timeout)
   - `route_update` -- Traefik route updated
   - `stop_old` -- old container stopped
   - `finalize` -- deploy completed successfully

4. Once the status changes to **success**, visit your app at its configured URL.

:::tip
Talos uses blue/green deployments. The old container stays running until the new one passes its health check. If the health check fails, the old container continues serving traffic with zero downtime.
:::

## Step 5: Rollback (If Needed)

If a deployment fails or your app is not behaving correctly:

1. Go to the app's detail page in the Talos web UI.
2. Click **Rollback**.
3. Talos will redeploy the last successful image.

Alternatively, use the API:

```bash
curl -X POST http://localhost:3000/api/apps/{appID}/deploys/rollback \
  -H "Cookie: session=<your-session-cookie>"
```

The rollback creates a new deploy record referencing the previous successful deploy's image. The same blue/green process applies -- the rollback image is health-checked before traffic switches over.

## What Happens Under the Hood

When you trigger a deploy, Talos executes the following sequence:

1. **Validates** required environment variables are set
2. **Captures** an environment variable snapshot for diff tracking
3. **Pulls** the target container image from the registry
4. **Starts** a staging container alongside the live one
5. **Health-checks** the staging container (30-second timeout)
6. **Switches** the Traefik route to the staging container on success
7. **Stops** the old live container
8. **Records** the deploy and all events in SQLite

If the health check fails at step 5, the staging container is destroyed and the old container continues serving traffic. The deploy is marked as `auto_rollback`.

## Next Steps

- [Configuration](./configuration.md) -- environment variables and options
- [Backup & Restore](./backup.md) -- protect your data
- [Managed Services](../features/managed-services.md) -- add databases and caches
- [App Management](../features/app-management.md) -- manage your applications
