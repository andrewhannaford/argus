#!/usr/bin/env bash
# setup-server.sh — SSHes into the EC2 and configures nginx + TLS + systemd
# Run after provision.sh: bash deploy/setup-server.sh

set -euo pipefail

source deploy/config
source deploy/.provision-state

KEY="deploy/argus-c2.pem"
SSH="ssh -i $KEY -o StrictHostKeyChecking=no ec2-user@$PUBLIC_IP"

echo "[*] Waiting for SSH on $PUBLIC_IP..."
until $SSH "echo ok" 2>/dev/null; do sleep 5; done
echo "[+] SSH ready."

# ── Upload configs ────────────────────────────────────────────────────────────
echo "[*] Uploading nginx config and systemd unit..."
# Substitute DOMAIN variable in nginx.conf before uploading
sed "s|your-domain-here.com|${DOMAIN}|g" deploy/nginx.conf > deploy/.tmp-nginx.conf
scp -i "$KEY" -o StrictHostKeyChecking=no \
  deploy/.tmp-nginx.conf \
  deploy/argus-server.service \
  "ec2-user@$PUBLIC_IP:/tmp/"
rm -f deploy/.tmp-nginx.conf

# ── Run remote setup ─────────────────────────────────────────────────────────
$SSH "sudo bash -s" << 'REMOTE'
set -euo pipefail

# Step 1: ensure nginx is running with the HTTP-only config from userdata
# (just in case it isn't up yet)
systemctl is-active nginx || systemctl start nginx

# Step 2: get TLS cert via webroot (avoids the nginx plugin / cert chicken-and-egg)
certbot certonly \
  --webroot \
  --webroot-path /var/www/certbot \
  -d "${DOMAIN}" \
  --non-interactive \
  --agree-tos \
  --email "${EMAIL}"

# Step 3: now that the cert exists, swap in the full nginx config with SSL
cp /tmp/nginx.conf /etc/nginx/conf.d/argus.conf
nginx -t && systemctl reload nginx

# Auto-renew via cron
echo "0 3 * * * root certbot renew --quiet --post-hook 'systemctl reload nginx'" \
  > /etc/cron.d/certbot-renew

# Install systemd service
cp /tmp/argus-server.service /etc/systemd/system/argus-server.service
systemctl daemon-reload
systemctl enable argus-server

echo "[+] Server configured. Deploy the binary next."
REMOTE

echo ""
echo "[+] Setup complete."
echo "    https://$DOMAIN is live."
echo ""
echo "Next: bash deploy/deploy-binary.sh"
