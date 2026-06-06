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
3. Configure Traefik as a reverse proxy
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
| Configuration | `/etc/talos/.env` |
| Data directory | `/var/lib/talos/` |
| Traefik config | `/var/lib/talos/traefik/` |
| Systemd service | `/etc/systemd/system/talos.service` |

## Post-Install

After installation, open `http://YOUR_VPS_IP:3000` and create your admin account.

::: tip
If you have a domain, configure it in the app settings after creation. Traefik will automatically provision HTTPS via Let's Encrypt.
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

Or target a specific version:

```bash
sudo bash install.sh --upgrade --version-tag v0.2.0
```

The upgrade process preserves your configuration and data, and automatically rolls back on failure.

See [Upgrading](/guide/upgrading) for details.
