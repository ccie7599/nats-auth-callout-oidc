#!/usr/bin/env bash
set -euo pipefail

DEMO_DOMAIN="${DEMO_DOMAIN:-nats-demo.connected-cloud.io}"
EDGERC_PATH="${EDGERC_PATH:-$HOME/.edgerc}"
EDGERC_SECTION="${EDGERC_SECTION:-default}"
CERT_DIR="$(cd "$(dirname "$0")/.." && pwd)/certs"
CREDENTIALS_FILE="/tmp/edgedns-credentials.ini"

echo "==> Requesting certificate for ${DEMO_DOMAIN}"

command -v certbot >/dev/null 2>&1 || {
  echo "ERROR: certbot not found. Install: pip install certbot certbot-plugin-edgedns"
  exit 1
}

if [ ! -f "$EDGERC_PATH" ]; then
  echo "ERROR: Akamai EdgeGrid credentials not found at $EDGERC_PATH"
  exit 1
fi

cat > "$CREDENTIALS_FILE" <<EOF
edgedns_edgerc_path = ${EDGERC_PATH}
edgedns_edgerc_section = ${EDGERC_SECTION}
EOF
chmod 600 "$CREDENTIALS_FILE"

certbot certonly \
  --authenticator edgedns \
  --edgedns-credentials "$CREDENTIALS_FILE" \
  --edgedns-propagation-seconds 120 \
  --server https://acme-v02.api.letsencrypt.org/directory \
  --agree-tos \
  --non-interactive \
  --email "brian@connected-cloud.io" \
  -d "$DEMO_DOMAIN" \
  --cert-name "$DEMO_DOMAIN"

LETSENCRYPT_LIVE="/etc/letsencrypt/live/${DEMO_DOMAIN}"

mkdir -p "$CERT_DIR"
cp "$LETSENCRYPT_LIVE/fullchain.pem" "$CERT_DIR/fullchain.pem"
cp "$LETSENCRYPT_LIVE/privkey.pem"   "$CERT_DIR/privkey.pem"
chmod 644 "$CERT_DIR/fullchain.pem"
chmod 600 "$CERT_DIR/privkey.pem"

rm -f "$CREDENTIALS_FILE"

echo "==> Certificates installed to ${CERT_DIR}/"
openssl x509 -in "$CERT_DIR/fullchain.pem" -noout -dates -subject
