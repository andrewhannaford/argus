#!/usr/bin/env bash
# deploy-binary.sh — builds the Linux server binary and pushes it to the EC2.
# Run from Git Bash: bash deploy/deploy-binary.sh
# On first run you will be prompted for AGENT_TOKEN and OPERATOR_PASSWORD.

set -euo pipefail

source deploy/config
source deploy/.provision-state

KEY="deploy/argus-c2.pem"
SSH="ssh -i $KEY -o StrictHostKeyChecking=no ec2-user@$PUBLIC_IP"
SCP="scp -i $KEY -o StrictHostKeyChecking=no"

# ── Build Linux binary ────────────────────────────────────────────────────────
echo "[*] Building server binary for linux/amd64..."
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o deploy/server ./cmd/server/
echo "[+] Binary built: $(du -sh deploy/server | cut -f1)"

# ── Upload binary ─────────────────────────────────────────────────────────────
echo "[*] Uploading binary..."
$SCP deploy/server "ec2-user@$PUBLIC_IP:/tmp/server"
$SSH "sudo mv /tmp/server /opt/argus/server && sudo chmod +x /opt/argus/server && sudo chown argus:argus /opt/argus/server"

# ── Create .env if it doesn't exist ──────────────────────────────────────────
if ! $SSH "sudo test -f /opt/argus/.env"; then
  echo ""
  echo "[*] First deploy — setting up credentials."
  read -rp "    AGENT_TOKEN (agents use this to connect): " AGENT_TOKEN
  read -rsp "    OPERATOR_PASSWORD (web UI login):         " OPERATOR_PASSWORD
  echo ""

  $SSH "sudo tee /opt/argus/.env > /dev/null" << EOF
AGENT_TOKEN=${AGENT_TOKEN}
OPERATOR_PASSWORD=${OPERATOR_PASSWORD}
EOF
  $SSH "sudo chmod 600 /opt/argus/.env && sudo chown argus:argus /opt/argus/.env"
  echo "[+] Credentials saved to /opt/argus/.env on server."
fi

# ── Restart service ───────────────────────────────────────────────────────────
echo "[*] Restarting argus-server service..."
$SSH "sudo systemctl restart argus-server"
sleep 2
$SSH "sudo systemctl status argus-server --no-pager -l"

echo ""
echo "[+] Deploy complete."
echo "    Operator UI:  https://${DOMAIN}/"
echo "    Agent URL:    wss://${DOMAIN}/ws/agent"
