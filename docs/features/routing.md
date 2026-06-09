# Routing

Talos supports two proxy topologies:

- `internal` proxy mode: Talos manages its own Traefik instance and can route app custom domains.
- `external` proxy mode: another Traefik reverse proxy owns `80/443` and publishes both the Talos UI and app custom domains through Docker labels.

This page covers those proxy modes, app access modes, and the related HTTPS behavior.

## Routing Modes

Talos supports two access modes for applications:

### Domain Mode

Each app gets its own domain. Traefik matches incoming requests by `Host()` header and routes to the correct container.

```
app1.example.com --> Traefik --> talos-app1 container
app2.example.com --> Traefik --> talos-app2 container
```

**Requirements:**

- `TALOS_PROXY_MODE=internal` or `TALOS_PROXY_MODE=external`
- `TALOS_DOMAIN` must be set for the Talos UI itself
- Each app's `domain` field must be set
- DNS A records must point to the server IP

**Benefits:**

- Clean URLs (e.g., `https://app.example.com`)
- Automatic HTTPS via Let's Encrypt
- Multiple apps on standard ports (80/443)

### IP:Port Fallback Mode

When no app domain is configured, each app gets a unique external port. Talos binds the app container directly to that fallback port.

```
<server-ip>:8080 --> talos-app1 container (port 8080)
<server-ip>:8081 --> talos-app2 container (port 8081)
```

**Requirements:**

- `TALOS_DOMAIN` must be empty
- Each app's `fallback_port` must be set (unique per app)

**Benefits:**

- No domain or DNS configuration needed
- Works immediately after install
- Suitable for development and testing

## Proxy Ownership

### Internal Mode

When `TALOS_PROXY_MODE=internal`, Talos starts `talos-traefik`, binds `80/443`, and writes file-provider routes for the Talos UI and domain-based apps.

### External Mode

When `TALOS_PROXY_MODE=external`, Talos does not start `talos-traefik`. Instead:

1. Your existing edge proxy must own `80/443`.
2. The Talos container must join the shared proxy network such as `traefik-public`.
3. The edge proxy must watch Talos and app container labels.
4. Talos routes `Host(TALOS_DOMAIN)` to Talos port `3000`.
5. Domain-mode apps are published by labels on the deployed app container.

When Talos deploys a domain-mode app in external mode, it performs a brief stop/start cutover so the shared Traefik router targets a single live container at a time.

## Traefik Configuration

### Static Configuration

Talos generates Traefik's static configuration (`traefik.yml`) automatically:

```yaml
api:
  dashboard: false
  insecure: false

entryPoints:
  web:
    address: ":80"
    http:
      redirections:
        entryPoint:
          to: websecure
          scheme: https
  websecure:
    address: ":443"

certificatesResolvers:
  letsencrypt:
    acme:
      email: admin@example.com
      storage: /data/acme.json
      httpChallenge:
        entryPoint: web

providers:
  file:
    directory: /etc/traefik/config
    watch: true

log:
  level: WARN
```

### Dynamic Configuration (Per-App Routes)

In internal proxy mode, each domain-based app gets a route file in the Traefik config directory. Talos generates these automatically on deploy.

**Domain mode route:**

```yaml
http:
  routers:
    my-app:
      rule: "Host(`app.example.com`)"
      service: "my-app"
      entryPoints:
        - websecure
      tls:
        certResolver: letsencrypt
  services:
    my-app:
      loadBalancer:
        servers:
          - url: "http://talos-my-app:3000"
```

Port-mode apps do not use a Traefik route file; Talos binds the app container directly to its fallback host port.

## HTTPS via Let's Encrypt

When `TALOS_PROXY_MODE=internal` and `TALOS_DOMAIN` is set, Talos configures Traefik with automatic TLS:

1. Traefik listens on ports 80 (HTTP) and 443 (HTTPS).
2. HTTP requests are redirected to HTTPS.
3. Let's Encrypt certificates are obtained via the HTTP challenge.
4. Certificates are stored in `/data/acme.json` inside the Traefik container.
5. Certificates are renewed automatically before expiration.

### Configuration

Set these environment variables:

```bash
TALOS_DOMAIN=talos.example.com
TALOS_ACME_EMAIL=admin@example.com
TALOS_PROXY_MODE=internal
```

:::tip
The ACME email is used for certificate expiration notifications from Let's Encrypt. Use a monitored address.
:::

### Custom Domains for Apps

When Talos is in internal proxy mode, individual apps can also use custom domains:

1. Set the app's `access_mode` to `domain`.
2. Set the app's `domain` to the desired hostname (e.g., `app.example.com`).
3. Point the domain's DNS A record to the server IP.
4. Deploy the app -- Traefik will obtain a certificate automatically.

## Migration Between Modes

### From IP Mode to Domain Mode

1. Set `TALOS_DOMAIN`, `TALOS_ACME_EMAIL`, and `TALOS_PROXY_MODE=internal` in your `.env` file.
2. Point your domain's DNS A record to the server IP.
3. Restart Talos:
   ```bash
   sudo systemctl restart talos
   ```
4. Talos will start Traefik with HTTPS configuration.
5. Update each app's `access_mode` to `domain` and set its `domain` field.
6. Redeploy each app to generate new Traefik route files.

### From Domain Mode to IP Mode

1. Remove `TALOS_DOMAIN` and `TALOS_ACME_EMAIL` from your `.env` file, or keep the Talos UI behind an external proxy with `TALOS_PROXY_MODE=external`.
2. Update each app's `access_mode` to `port` and set a `fallback_port`.
3. Restart Talos.
4. Redeploy each app.
5. Stop and remove the Traefik container (it is no longer needed):
   ```bash
   docker stop talos-traefik
   docker rm talos-traefik
   ```

:::warning
Switching from domain mode to IP mode will break any existing domain-based URLs. Update DNS records and bookmarks accordingly.
:::

## Traefik Dashboard

The Traefik dashboard can be enabled for debugging:

```bash
TALOS_TRAEFIK_DASHBOARD=true
```

:::warning
The Traefik dashboard is unauthenticated by default. Only enable it for debugging and disable it in production.
:::

## External Proxy Example

For a VPS that already runs a shared Traefik edge proxy:

1. Set:
   ```bash
   TALOS_PROXY_MODE=external
   TALOS_DOMAIN=talos.example.com
   TALOS_EDGE_NETWORK=traefik-public
   TALOS_EDGE_CERT_RESOLVER=letsencrypt
   ```
2. Recreate Talos with `/opt/talos/.env` bind-mounted into the container.
3. Attach the Talos container to `traefik-public`.
4. Let the external Traefik watch Docker labels on `traefik-public`.
5. Talos will publish `Host(talos.example.com)` for the UI and custom-domain labels for app containers on new deploys.

## Container Networking

All Talos-managed containers join the `talos` Docker network (configurable via `TALOS_DOCKER_NETWORK`). This allows:

- Talos-managed Traefik to reach app containers by name in internal proxy mode
- App containers to reach service containers by name
- DNS resolution between containers on the same network

Container names are used as hostnames:

| Container | Hostname |
|-----------|----------|
| `talos-my-app` | `talos-my-app` |
| `talos-svc-my-db` | `talos-svc-my-db` |
| `talos-traefik` | `talos-traefik` |

## Next Steps

- [First Deployment](../guide/first-deploy.md) -- deploy with routing
- [Configuration](../guide/configuration.md) -- all environment variables
- [Components](../architecture/components.md) -- Traefik internals
