#!/bin/bash
# Runs on first boot via EC2 user data.
set -euo pipefail

# Update and install nginx + certbot
dnf update -y
dnf install -y nginx python3-certbot-nginx

# Create unprivileged service user
useradd -r -s /sbin/nologin argus 2>/dev/null || true

# Create directories
mkdir -p /opt/argus
mkdir -p /var/www/certbot/.well-known/acme-challenge
chown -R argus:argus /opt/argus

# Drop initial nginx config (HTTP only — certbot will upgrade to HTTPS)
cat > /etc/nginx/conf.d/argus.conf << 'NGINXCONF'
server {
    listen 80;
    server_name your-domain-here.com;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        return 200 'Service Unavailable';
        add_header Content-Type text/plain;
    }
}
NGINXCONF

systemctl enable nginx
systemctl start nginx
