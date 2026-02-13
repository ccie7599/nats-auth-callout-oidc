# NATS Server Configuration

The NATS server is configured with TLS for client connections, WebSocket support for browser clients, and auth-callout to delegate all authentication to the external auth service.

## Full Configuration

```conf
# nats-server.conf

server_name: nats-auth-demo

listen: 0.0.0.0:4222

# Monitoring endpoint
http_port: 8222

# TLS for client connections (port 4222)
tls {
  cert_file: /certs/fullchain.pem
  key_file: /certs/privkey.pem
}

# WebSocket listener with TLS (port 8443)
websocket {
  port: 8443
  tls {
    cert_file: /certs/fullchain.pem
    key_file: /certs/privkey.pem
  }
  allowed_origins: ["https://your-domain.example.com"]
  compression: true
}

# Account definitions
accounts {
  AUTH: {
    users: [
      { user: "auth-service", password: $AUTH_SERVICE_PASSWORD }
    ]
  }
  APP: {}
  SYS: {}
}

system_account: SYS

# Auth callout configuration
authorization {
  auth_callout {
    issuer: $NKEY_PUBLIC
    account: AUTH
    auth_users: ["auth-service"]
  }
}

# Logging
logtime: true
debug: true
trace: false
```

## Key Sections Explained

### Accounts

Three accounts serve distinct roles:

| Account | Purpose |
|---|---|
| `AUTH` | The auth service connects here with username/password. Only this account can subscribe to `$SYS.REQ.USER.AUTH`. |
| `APP` | All OIDC-authenticated clients land in this account. The auth service sets `uc.Audience = "APP"` in the UserClaims JWT. |
| `SYS` | NATS system account for internal protocol messages. |

The `auth_users` list in the `auth_callout` block specifies which users bypass the callout and authenticate directly (with their account credentials). All other connection attempts trigger the callout.

### Auth-Callout Block

```conf
authorization {
  auth_callout {
    issuer: $NKEY_PUBLIC      # Public key of the NKey pair
    account: AUTH              # Account the auth service lives in
    auth_users: ["auth-service"]  # Users that skip callout
  }
}
```

- **`issuer`**: The public key of the account NKey pair. The auth service signs response JWTs with the corresponding private seed. The NATS server verifies signatures against this public key.
- **`account`**: Which account the auth service user belongs to. The server publishes auth requests to `$SYS.REQ.USER.AUTH` in this account's context.
- **`auth_users`**: Users that authenticate directly (username/password) without triggering the callout. The auth service must be in this list, otherwise it would trigger a callout loop.

### TLS

TLS is configured for both standard NATS clients (port 4222) and WebSocket clients (port 8443) using the same certificate. For Let's Encrypt certificates:

```conf
tls {
  cert_file: /certs/fullchain.pem   # Full certificate chain
  key_file: /certs/privkey.pem      # Private key
}
```

### WebSocket

The `websocket` block enables browser-based NATS clients via the [nats.ws](https://github.com/nats-io/nats.ws) library:

```conf
websocket {
  port: 8443
  tls {
    cert_file: /certs/fullchain.pem
    key_file: /certs/privkey.pem
  }
  allowed_origins: ["https://your-domain.example.com"]
  compression: true
}
```

- **`allowed_origins`**: Restricts which web origins can establish WebSocket connections. Set this to your dashboard's HTTPS URL.
- **`compression`**: Enables permessage-deflate for WebSocket frames.

Auth-callout applies identically to WebSocket connections — the browser client passes the token via the WebSocket CONNECT message, and the same auth service validates it.

### Environment Variable Substitution

NATS server supports environment variable substitution with `$VAR_NAME` syntax:

- `$NKEY_PUBLIC` — The NKey public key, read from the shared volume at container startup
- `$AUTH_SERVICE_PASSWORD` — Shared password between NATS server and auth service

The Docker entrypoint reads the NKey public key from file and exports it before starting the server:

```bash
export NKEY_PUBLIC=$(cat /nkeys/auth.pub)
export AUTH_SERVICE_PASSWORD=${AUTH_SERVICE_PASSWORD:-callout-secret}
exec nats-server -c /etc/nats/nats-server.conf
```

## NKey Generation

The NKey pair is generated at startup by an init container using `nk` from [nats-box](https://github.com/nats-io/nats-box):

```bash
#!/bin/sh
set -eu

NKEY_DIR="/nkeys"

# Skip if keys already exist
if [ -f "$NKEY_DIR/auth.seed" ] && [ -f "$NKEY_DIR/auth.pub" ]; then
  echo "NKeys already exist, skipping generation"
  exit 0
fi

echo "Generating auth-callout account NKey pair..."

# Generate account NKey pair
nk -gen account > "$NKEY_DIR/auth.seed"
chmod 600 "$NKEY_DIR/auth.seed"

# Extract public key
nk -inkey "$NKEY_DIR/auth.seed" -pubout > "$NKEY_DIR/auth.pub"
chmod 644 "$NKEY_DIR/auth.pub"

echo "NKey public key: $(cat "$NKEY_DIR/auth.pub")"
```

The keys are written to a shared Docker volume:
- `auth.seed` (private, `600`) — read by the auth service for JWT signing
- `auth.pub` (public, `644`) — read by the NATS server as the `issuer` value

## Internal TLS Note

When running in Docker Compose, containers connect to NATS via the hostname `nats`, but the TLS certificate is issued for the external domain (e.g., `nats-demo.example.com`). The auth service handles this with a `ServerName` override in the TLS config:

```go
tlsCfg := &tls.Config{
    RootCAs:    caCertPool,
    ServerName: "nats-demo.example.com",  // Match cert CN
}
opts = append(opts, nats.Secure(tlsCfg))
```

This avoids certificate hostname mismatch errors for internal Docker traffic while maintaining TLS encryption.
