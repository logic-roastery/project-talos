# First Deployment

This guide walks you through deploying your first application with Talos.

## Prerequisites

- Talos installed and running (see [Installation](./installation.md))
- The Talos web UI accessible at `http://<your-server-ip>:3000`
- An admin account created via the setup wizard

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

::: tip
If you choose **domain mode**, make sure your DNS A record points to your server IP. Talos will automatically configure Traefik with HTTPS via Let's Encrypt.
:::

## Step 2: Connect GitHub App

Talos uses a GitHub App to automatically receive push events and trigger deploys. This is the recommended integration — no manual workflow files needed.

1. Go to **Settings > GitHub Setup** in the Talos web UI.
2. Click **Create GitHub App**. Talos will redirect you to GitHub using GitHub's supported **manifest flow**.
3. Review the generated app on GitHub and complete the registration.
4. Install the GitHub App on your repository.
5. Talos will receive the generated credentials automatically and store them for you.

Once connected, every push to your app's configured branch will trigger an automatic deployment.

::: tip
The GitHub App handles authentication, webhook delivery, and secret management for you. You don't need to manually configure webhook URLs or signing secrets.
:::

::: info What to Expect on GitHub
Talos uses the GitHub App **manifest** flow, not the manual "fill every field yourself" flow. GitHub may not show every field as visibly prefilled on the page, but Talos is still passing the webhook URL, callback URL, permissions, and subscribed events in the manifest. The app name can still be edited before you submit it.
:::

::: warning No Domain Yet?
If Talos is running on a raw IP address without a domain, the generated GitHub App manifest will use `http://<your-server-ip>:3000` for the homepage, webhook URL, and setup callback URL. This is acceptable for local testing, but a proper domain with HTTPS is strongly recommended before relying on GitHub App webhooks in a real deployment.
:::

### Manual Fallback: Fill the GitHub App Form Yourself

If you cannot use Talos's **Create GitHub App** button, use these values when GitHub shows the manual registration form.

Assuming Talos is published at `https://talos.example.com`:

- **GitHub App name**: any unique name such as `talos-example-deploy`
- **Homepage URL**: `https://talos.example.com`
- **Callback URL**: `https://talos.example.com/api/github/setup-callback`
- **Setup URL**: leave empty
- **Webhook Active**: enabled
- **Webhook URL**: `https://talos.example.com/api/webhooks/github`
- **Webhook Secret**: generate a random secret and reuse the same value in `TALOS_GITHUB_WEBHOOK_SECRET`

Keep these options disabled unless you explicitly need them:

- **Expire user authorization tokens**
- **Request user authorization (OAuth) during installation**
- **Enable Device Flow**
- **Redirect on update**

Use these repository permissions:

- **Contents**: `Read and write`
- **Actions**: `Read and write`
- **Metadata**: `Read-only`
- **Packages**: `Read-only`

Leave all other repository, organization, and account permissions as **No access**.

Subscribe only to these events:

- `Workflow run`
- `Installation`

For installation scope, choose:

- **Only on this account** if you only deploy your own repositories
- **Any account** only if you intentionally want wider installation support

### Add the GitHub App Credentials to Talos

After you create the app on GitHub, copy these values into `/opt/talos/.env`:

```env
TALOS_GITHUB_WEBHOOK_SECRET=your-webhook-secret
TALOS_GITHUB_APP_ID=1234567
TALOS_GITHUB_APP_SLUG=talos-example-deploy
TALOS_GITHUB_APP_PRIVATE_KEY=/opt/talos/github-app.private-key.pem
TALOS_GITHUB_APP_CLIENT_ID=Iv23li...
TALOS_GITHUB_APP_CLIENT_SECRET=your-client-secret
```

Save the GitHub private key as a PEM file on the server:

```bash
sudo mkdir -p /opt/talos
sudo nano /opt/talos/github-app.private-key.pem
sudo chmod 600 /opt/talos/github-app.private-key.pem
```

Paste the full private key including:

```pem
-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----
```

Then recreate Talos so it reloads the updated env file and mounted PEM key. In Docker mode, make sure both are mounted:

```bash
docker stop talos
docker rm talos
docker run -d \
  --name talos \
  --restart unless-stopped \
  --network talos \
  -p 3000:3000 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /opt/talos/data:/data \
  -v /opt/talos/.env:/opt/talos/.env \
  -v /opt/talos/github-app.private-key.pem:/opt/talos/github-app.private-key.pem:ro \
  --env-file /opt/talos/.env \
  ghcr.io/logic-roastery/project-talos:latest
```

Talos logs should then show that the GitHub App is configured.

### Troubleshooting GitHub Repo Discovery

If the **New App** page loads but the GitHub repository list looks incomplete or fails to load, Talos includes a temporary authenticated debug endpoint for GitHub App diagnostics.

Enable it only while troubleshooting:

```env
TALOS_DEBUG_ENDPOINTS=true
```

Then recreate or restart Talos and call:

```bash
curl -s http://127.0.0.1:3000/api/github/debug \
  -H 'Cookie: talos_session=YOUR_SESSION_COOKIE'
```

This endpoint can help confirm:

- whether the GitHub App is configured
- whether the private key is readable
- how many installations GitHub returned
- whether Talos can list repositories for each installation

When you are done debugging, disable it again:

```env
TALOS_DEBUG_ENDPOINTS=false
```

## Step 3: Push Your Code

```bash
git add .
git commit -m "feat: initial deploy"
git push origin main
```

Talos will automatically:

1. Receive the push event from GitHub
2. Pull the container image
3. Start a staging container
4. Run health checks
5. Switch traffic to the new container
6. Record the deploy in history

## Step 4: Verify the Deployment

1. Open the Talos web UI and navigate to your app's detail page.
2. You should see a new deploy entry with status **running**.
3. The deployment page shows real-time events:
   - `start` — deploy initiated
   - `pull` — image being pulled
   - `start` — staging container started
   - `health_check` — waiting for health check (30s timeout)
   - `route_update` — Traefik route updated
   - `stop_old` — old container stopped
   - `finalize` — deploy completed successfully

4. Once the status changes to **success**, visit your app at its configured URL.

::: tip
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

The rollback creates a new deploy record referencing the previous successful deploy's image. The same blue/green process applies — the rollback image is health-checked before traffic switches over.

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

## Alternative: Manual Webhook Setup

If you prefer not to use the GitHub App, you can set up a manual webhook:

1. Go to your app's settings in the Talos web UI.
2. Copy the **Webhook URL** and **Webhook Secret**.
3. Add them as GitHub repository secrets (`TALOS_URL` and `WEBHOOK_SIGNATURE`).
4. Create `.github/workflows/deploy.yml` in your repository:

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

::: warning
Store your Talos webhook secret and URL as GitHub repository secrets. Never commit them directly to your repository.
:::

## Next Steps

- [Configuration](./configuration.md) — environment variables and options
- [Backup & Restore](./backup.md) — protect your data
- [Managed Services](../features/managed-services.md) — add databases and caches
- [App Management](../features/app-management.md) — manage your applications
