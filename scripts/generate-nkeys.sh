#!/bin/sh
set -eu

NKEY_DIR="/nkeys"

if [ -f "$NKEY_DIR/auth.seed" ] && [ -f "$NKEY_DIR/auth.pub" ]; then
  echo "NKeys already exist, skipping generation"
  cat "$NKEY_DIR/auth.pub"
  exit 0
fi

echo "Generating auth-callout account NKey pair..."

nk -gen account > "$NKEY_DIR/auth.seed"
chmod 600 "$NKEY_DIR/auth.seed"

nk -inkey "$NKEY_DIR/auth.seed" -pubout > "$NKEY_DIR/auth.pub"
chmod 644 "$NKEY_DIR/auth.pub"

echo "NKey public key: $(cat "$NKEY_DIR/auth.pub")"
