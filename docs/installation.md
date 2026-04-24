# Installation

Pre-built `bambu-observer` binaries for **linux/amd64** and **linux/arm64** are attached to each [GitHub Release](https://github.com/szymon3/bambu-middleman/releases) alongside a `sha256sums.txt` checksum file.

## Install script (recommended)

The install script downloads the latest release, verifies the checksum, creates a dedicated system user, and sets up a systemd service:

```bash
curl -fsSL https://raw.githubusercontent.com/szymon3/bambu-middleman/master/install.sh | sudo bash
```

What it does:

1. Detects your architecture (amd64 or arm64).
2. Downloads the latest release binary and checksum from GitHub.
3. Verifies the SHA-256 checksum.
4. Creates a `bambu-observer` system user (no home directory, no login shell).
5. Installs the binary to `/usr/local/bin/bambu-observer`.
6. Copies `env.example` to `/etc/bambu-observer/env` (first install only — never overwrites existing config).
7. Installs a systemd unit and enables the service.

After installation, edit the configuration file and start the service:

```bash
sudo nano /etc/bambu-observer/env    # set PRINTER_IP, PRINTER_SERIAL, PRINTER_ACCESS_CODE
sudo systemctl restart bambu-observer
sudo journalctl -u bambu-observer -f  # watch logs
```

### Upgrading

The `--upgrade` flag swaps the binary without touching your configuration:

```bash
curl -fsSL https://raw.githubusercontent.com/szymon3/bambu-middleman/master/install.sh | sudo bash -s -- --upgrade
```

## Systemd service

The installed service runs as:

```ini
[Service]
Type=simple
User=bambu-observer
EnvironmentFile=/etc/bambu-observer/env
ExecStart=/usr/local/bin/bambu-observer
Restart=on-failure
RestartSec=5s
```

Configuration lives in `/etc/bambu-observer/env`. Restart the service after any change:

```bash
sudo systemctl restart bambu-observer
```

## Build from source

Requires Go 1.22 or later.

```bash
git clone https://github.com/szymon3/bambu-middleman.git
cd bambu-middleman
go build -o bambu-observer ./cmd/observer
```

For development, run directly without building:

```bash
go run ./cmd/observer
```
