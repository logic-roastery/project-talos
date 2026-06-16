# Installation

## Requirements

- A Linux VPS (Ubuntu/Debian/Fedora)
- Root or sudo access
- A domain name (optional, for HTTPS)

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo bash
```

This will:

1. Install Docker if not present
2. Install the Talos binary
3. Configure Talos routing for either internal Traefik or an external edge proxy
4. Set up systemd services
5. Create the runtime directory layout

:::tip
This `curl | bash` method does not save `install.sh` on the server. If you want to rerun `sudo bash install.sh ...` later, download the script first and keep it locally.
:::

## Installation Modes

### Bare Binary (default)

```bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh
sudo bash install.sh
```

Installs Talos as a native binary managed by systemd. Best for production use.

### Docker Mode

```bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh
sudo bash install.sh --docker
```

Runs Talos inside a Docker container. Easier upgrades, but slightly more abstraction.

### Build from Source

```bash
sudo bash install.sh --from-source
```

Clones the repo and builds the Go binary locally. Requires Go 1.21+.

### Custom Port

```bash
sudo bash install.sh --port 8080
```

Changes the Talos web UI port (default: 3000).

## What Gets Installed

| Component | Location |
|-----------|----------|
| Talos binary | `/usr/local/bin/talos` |
| Configuration | `/opt/talos/.env` |
| Data directory | `/opt/talos/data/` |
| Traefik config | `/opt/talos/data/traefik/` |
| Systemd service | `/etc/systemd/system/talos.service` |

## Post-Install

After installation, open `http://YOUR_VPS_IP:3000` and create your admin account.

::: tip
If you have a domain, configure it in Talos Settings. Use `internal` proxy mode if Talos should own `80/443`, or `external` proxy mode if another reverse proxy on the VPS already owns public HTTPS.
:::

## Upgrading

If you installed via `curl | bash`, upgrade the same way:

```bash
curl -fsSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo bash -s -- --upgrade
```

If you downloaded and kept `install.sh` locally, use:

```bash
sudo bash install.sh --upgrade
```

The script automatically resolves the latest version. If you need a specific version:

```bash
sudo bash install.sh --upgrade --version-tag v0.4.0
```

For Docker mode upgrades, add `--docker`:

```bash
curl -fsSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo bash -s -- --upgrade --docker
```

The upgrade process preserves your configuration and data, and automatically rolls back on failure.

Talos generates `TALOS_ENCRYPTION_KEY` only on first install. On later upgrades or restarts, if the database already exists and the key is missing from `/opt/talos/.env`, Talos exits with an error instead of generating a new key. Keep `/opt/talos/.env` with your database and backups.

If you intentionally need a new encryption key, use:

```bash
sudo bash install.sh --upgrade --regenerate-encryption-key
```

For Docker mode:

```bash
sudo bash install.sh --upgrade --docker --regenerate-encryption-key
```

That operation makes previously encrypted service credentials unreadable unless you restore the old key or recreate the affected services.

See [Upgrading](/guide/upgrading) for details.
