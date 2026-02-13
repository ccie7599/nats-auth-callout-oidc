#!/usr/bin/env bash
set -euo pipefail

# Deploy to Linode instance
# Usage: ./scripts/deploy.sh <ip-address> [domain]

IP="${1:?Usage: deploy.sh <ip-address> [domain]}"
DOMAIN="${2:-nats-demo.connected-cloud.io}"
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
REMOTE_DIR="/opt/nats-demo"
SSH="ssh -o StrictHostKeyChecking=accept-new root@${IP}"
SCP="scp -o StrictHostKeyChecking=accept-new"

echo "==> Deploying to ${IP} (${DOMAIN})"

# Wait for cloud-init
echo "==> Waiting for cloud-init..."
for i in $(seq 1 60); do
  if $SSH "test -f /opt/nats-demo/.cloud-init-done" 2>/dev/null; then
    echo "    Cloud-init complete"
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "ERROR: Cloud-init timed out"
    exit 1
  fi
  sleep 5
done

# Sync project files
echo "==> Syncing project files..."
rsync -avz --delete \
  --exclude '.git' \
  --exclude 'certs/*.pem' \
  --exclude '.env' \
  --exclude 'infra/terraform/.terraform' \
  --exclude 'infra/terraform/terraform.tfstate*' \
  -e "ssh -o StrictHostKeyChecking=accept-new" \
  "${PROJECT_DIR}/" "root@${IP}:${REMOTE_DIR}/"

# Copy certs if they exist locally
if [ -f "${PROJECT_DIR}/certs/fullchain.pem" ]; then
  echo "==> Copying TLS certificates..."
  $SCP "${PROJECT_DIR}/certs/fullchain.pem" "root@${IP}:${REMOTE_DIR}/certs/fullchain.pem"
  $SCP "${PROJECT_DIR}/certs/privkey.pem" "root@${IP}:${REMOTE_DIR}/certs/privkey.pem"
fi

# Copy .env if it exists
if [ -f "${PROJECT_DIR}/.env" ]; then
  echo "==> Copying .env..."
  $SCP "${PROJECT_DIR}/.env" "root@${IP}:${REMOTE_DIR}/.env"
fi

# Build and start
echo "==> Building and starting services on remote..."
$SSH "cd ${REMOTE_DIR} && docker compose --profile dashboard --profile cli build"
$SSH "cd ${REMOTE_DIR} && docker compose --profile dashboard up -d"

echo ""
echo "==> Deployment complete!"
echo "    Dashboard: https://${DOMAIN}"
echo "    NATS TLS:  tls://${DOMAIN}:4222"
echo "    NATS WSS:  wss://${DOMAIN}:8443"
echo "    NATS Mon:  http://${IP}:8222"
echo ""
echo "    Run demo:  ssh root@${IP} 'cd ${REMOTE_DIR} && docker compose --profile cli run --rm demo-client --scenario all'"
echo "    Logs:      ssh root@${IP} 'cd ${REMOTE_DIR} && docker compose logs -f auth-service'"
