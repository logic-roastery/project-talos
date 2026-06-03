# Managed Services

Talos can provision and manage backing services -- databases, caches, and object storage -- as Docker containers. This page covers the supported services, provisioning lifecycle, credential management, and app linking.

## Supported Services

| Type | Image | Default Port | Health Check |
|------|-------|-------------|--------------|
| PostgreSQL | `postgres:16` | 5432 | `pg_isready -U $POSTGRES_USER` |
| MySQL | `mysql:8` | 3306 | `mysqladmin ping -h localhost -u root -p$MYSQL_ROOT_PASSWORD` |
| Redis | `redis:7-alpine` | 6379 | `redis-cli ping` |
| Garage | `dxflrs/garage:v1.0` | 3900 | `wget -qO- http://localhost:3900/health` |

## Provisioning Lifecycle

Each service progresses through these statuses:

```
pending --> provisioning --> active
                          --> error
active  --> stopped
stopped --> active (restart)
```

### 1. Creation

When you create a service, Talos:

1. Generates secure credentials (random passwords, keys).
2. Encrypts the credentials with AES-256-GCM using `TALOS_ENCRYPTION_KEY`.
3. Creates a persistent data directory on the host filesystem.
4. Records the service in SQLite with status `pending`.

### 2. Provisioning

Talos starts the service container:

1. Pulls the Docker image.
2. Creates a container with the generated credentials as environment variables.
3. Mounts the persistent data directory as a volume.
4. Starts the container on the `talos` Docker network.
5. Waits for the health check to pass.

### 3. Active

Once the health check passes, the service status changes to `active`. The service is ready to accept connections from linked applications.

### 4. Stopping and Restarting

Services can be stopped and restarted without losing data. The persistent volume retains all data across container restarts.

## Credential Management

### Generation

Credentials are generated automatically during service creation:

| Service | Generated Credentials |
|---------|----------------------|
| PostgreSQL | `host`, `port`, `database`, `user`, `password` |
| MySQL | `host`, `port`, `database`, `user`, `password` |
| Redis | `host`, `port`, `password` |
| Garage | `endpoint`, `region`, `access_key`, `secret_key`, `bucket` |

### Encryption

All credentials are encrypted at rest using AES-256-GCM:

1. The encryption key is derived from `TALOS_ENCRYPTION_KEY` (base64-encoded 32-byte key).
2. Credentials are serialized as JSON.
3. The JSON is encrypted and stored in the `services.credentials` column.
4. Credentials are decrypted only when needed for injection into app containers.

:::danger
If you lose `TALOS_ENCRYPTION_KEY`, encrypted credentials in the database and backups are unrecoverable. Always back up your `.env` file.
:::

### Viewing Credentials

**Web UI:** Navigate to the service detail page and click **Show Credentials**.

**API:**

```bash
curl http://localhost:3000/api/services/{serviceID}/credentials \
  -H "Cookie: session=<your-session-cookie>"
```

## Environment Variable Injection

When an app is linked to a service, Talos injects connection variables automatically during deployment. The variables are prefixed with the service alias if one is set.

### Without Alias

For a PostgreSQL service linked without an alias:

```
POSTGRES_HOST=<service-container-name>
POSTGRES_PORT=5432
POSTGRES_DB=<database-name>
POSTGRES_USER=<username>
POSTGRES_PASSWORD=<password>
```

### With Alias

For a PostgreSQL service linked with alias `DB`:

```
DB_HOST=<service-container-name>
DB_PORT=5432
DB_DB=<database-name>
DB_USER=<username>
DB_PASSWORD=<password>
```

### Injected Variables by Service Type

**PostgreSQL:**
- `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`

**MySQL:**
- `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_DATABASE`, `MYSQL_USER`, `MYSQL_PASSWORD`

**Redis:**
- `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`

**Garage:**
- `GARAGE_ENDPOINT`, `GARAGE_REGION`, `GARAGE_ACCESS_KEY`, `GARAGE_SECRET_KEY`, `GARAGE_BUCKET`

## Attachment Model

Services are linked to apps via the `app_services` join table. Each link has:

- `app_id` -- the application
- `service_id` -- the service
- `alias` -- optional prefix for environment variables

An app can link to multiple services. A service can be linked to multiple apps.

### Link a Service to an App

**Web UI:** In the app settings page, select a service to link and optionally set an alias.

**API:**

```bash
curl -X POST http://localhost:3000/api/apps/{appID}/services \
  -H "Content-Type: application/json" \
  -H "Cookie: session=<your-session-cookie>" \
  -d '{
    "service_id": 1,
    "alias": "DB"
  }'
```

### List Linked Services

```bash
curl http://localhost:3000/api/apps/{appID}/services \
  -H "Cookie: session=<your-session-cookie>"
```

### Unlink a Service

```bash
curl -X DELETE http://localhost:3000/api/apps/{appID}/services/{serviceID} \
  -H "Cookie: session=<your-session-cookie>"
```

## Service Management API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/services` | List all services |
| `POST` | `/api/services` | Create a service |
| `GET` | `/api/services/{id}` | Get service details |
| `DELETE` | `/api/services/{id}` | Delete a service |
| `POST` | `/api/services/{id}/stop` | Stop a service |
| `POST` | `/api/services/{id}/start` | Start a service |
| `GET` | `/api/services/{id}/credentials` | View decrypted credentials |

### Create a Service

```bash
curl -X POST http://localhost:3000/api/services \
  -H "Content-Type: application/json" \
  -H "Cookie: session=<your-session-cookie>" \
  -d '{
    "name": "my-database",
    "type": "postgres"
  }'
```

### Stop a Service

```bash
curl -X POST http://localhost:3000/api/services/{id}/stop \
  -H "Cookie: session=<your-session-cookie>"
```

### Start a Service

```bash
curl -X POST http://localhost:3000/api/services/{id}/start \
  -H "Cookie: session=<your-session-cookie>"
```

## Data Persistence

Service data is stored in host directories:

```
/opt/talos/data/services/<service-name>/
```

This directory is mounted into the container at the service's volume path:

| Service | Container Volume Path |
|---------|----------------------|
| PostgreSQL | `/var/lib/postgresql/data` |
| MySQL | `/var/lib/mysql` |
| Redis | `/data` |
| Garage | `/data` |

Data persists across container restarts, upgrades, and even Talos reinstallation (as long as the data directory is preserved).

## Next Steps

- [App Management](./app-management.md) -- link services to apps
- [Backup](./backup.md) -- back up service data
- [Routing](./routing.md) -- configure application access
