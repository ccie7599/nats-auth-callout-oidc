# Deployment

Docker Compose orchestrates all components: NKey generation, NATS server, auth service, web dashboard, and demo CLI client.

## Service Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Docker Compose                                                 │
│                                                                 │
│  ┌──────────┐    ┌──────────┐    ┌──────────────┐              │
│  │ nkey-init │──▶│  nkeys   │◀──│  NATS Server  │ :4222 (TLS) │
│  │ (init)    │   │ (volume) │   │               │ :8443 (WSS) │
│  └──────────┘    └──────────┘   │               │ :8222 (mon) │
│                       ▲         └───────┬───────┘              │
│                       │                 │                       │
│                       │        $SYS.REQ.USER.AUTH              │
│                       │                 │                       │
│                       │         ┌───────▼───────┐              │
│                       └────────│  Auth Service  │              │
│                                │  (Go)          │              │
│                                └────────────────┘              │
│                                                                 │
│  ┌──────────────┐    ┌──────────────┐                          │
│  │  Dashboard   │    │  Demo Client │                          │
│  │  (Nginx)     │    │  (Go CLI)    │                          │
│  │  :443/:80    │    │  profile:cli │                          │
│  │  profile:    │    └──────────────┘                          │
│  │  dashboard   │                                              │
│  └──────────────┘                                              │
└─────────────────────────────────────────────────────────────────┘
```

## Docker Compose Configuration

```yaml
services:
  # --- NKey Generator (init) ---
  nkey-init:
    image: natsio/nats-box:latest
    entrypoint: ["/bin/sh", "/scripts/generate-nkeys.sh"]
    volumes:
      - nkeys:/nkeys
      - ./scripts:/scripts:ro

  # --- NATS Server ---
  nats:
    image: nats:2.10-alpine
    ports:
      - "4222:4222"    # TLS client connections
      - "8443:8443"    # WebSocket (TLS)
      - "8222:8222"    # Monitoring
    volumes:
      - ./nats-config:/etc/nats:ro
      - ./certs:/certs:ro
      - nkeys:/nkeys:ro
    entrypoint: ["/bin/sh", "-c"]
    command:
      - |
        export NKEY_PUBLIC=$$(cat /nkeys/auth.pub)
        export AUTH_SERVICE_PASSWORD=$${AUTH_SERVICE_PASSWORD:-callout-secret}
        exec nats-server -c /etc/nats/nats-server.conf
    environment:
      AUTH_SERVICE_PASSWORD: ${AUTH_SERVICE_PASSWORD:-callout-secret}
    depends_on:
      nkey-init:
        condition: service_completed_successfully
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8222/healthz"]
      interval: 5s
      timeout: 3s
      retries: 10

  # --- Auth Callout Service ---
  auth-service:
    build:
      context: ./auth-service
      dockerfile: Dockerfile
    environment:
      NATS_URL: tls://nats:4222
      NATS_USER: auth-service
      NATS_PASSWORD: ${AUTH_SERVICE_PASSWORD:-callout-secret}
      OIDC_ISSUER_URL: https://auth.pingone.com/<your-env-id>/as
      NKEY_SEED_FILE: /nkeys/auth.seed
      TLS_CA_FILE: /certs/fullchain.pem
      TLS_SERVER_NAME: your-domain.example.com
    volumes:
      - nkeys:/nkeys:ro
      - ./certs:/certs:ro
    depends_on:
      nats:
        condition: service_healthy

  # --- Web Dashboard (Nginx) ---
  dashboard:
    image: nginx:1.27-alpine
    ports:
      - "443:443"
      - "80:80"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./web:/usr/share/nginx/html:ro
      - ./certs:/certs:ro
    depends_on:
      nats:
        condition: service_healthy
    profiles:
      - dashboard

  # --- Demo Client (CLI) ---
  demo-client:
    build:
      context: ./demo-client
      dockerfile: Dockerfile
    environment:
      NATS_URL: tls://nats:4222
      PING_ISSUER_URL: https://auth.pingone.com/<your-env-id>/as
      PING_CLIENT_ID: ${PING_CLIENT_ID}
      PING_CLIENT_SECRET: ${PING_CLIENT_SECRET}
      PING_PUB_CLIENT_ID: ${PING_PUB_CLIENT_ID}
      PING_PUB_CLIENT_SECRET: ${PING_PUB_CLIENT_SECRET}
      PING_SUB_CLIENT_ID: ${PING_SUB_CLIENT_ID}
      PING_SUB_CLIENT_SECRET: ${PING_SUB_CLIENT_SECRET}
    volumes:
      - ./certs:/certs:ro
    depends_on:
      auth-service:
        condition: service_started
    profiles:
      - cli

volumes:
  nkeys:
    driver: local
```

## Startup Order

```
1. nkey-init       Generate NKey pair → write to shared volume
2. nats            Read NKey public key → start NATS server with auth-callout
3. auth-service    Read NKey seed → connect to NATS → subscribe to $SYS.REQ.USER.AUTH
4. dashboard       Start Nginx → serve web UI + proxy token requests
```

Docker Compose `depends_on` with `condition: service_completed_successfully` (nkey-init) and `condition: service_healthy` (nats) ensures correct ordering.

## TLS Certificates

### Let's Encrypt via DNS-01

For production demos with a real domain, use Let's Encrypt with DNS-01 challenge:

```bash
# Using certbot with Akamai Edge DNS plugin
sudo certbot certonly \
  --authenticator dns-edgedns \
  --dns-edgedns-credentials ~/.edgerc \
  -d "your-domain.example.com" \
  --cert-path ./certs/fullchain.pem \
  --key-path ./certs/privkey.pem
```

### Self-Signed (Development)

For local development, generate self-signed certs:

```bash
mkdir -p certs
openssl req -x509 -newkey rsa:4096 -keyout certs/privkey.pem \
  -out certs/fullchain.pem -days 365 -nodes \
  -subj "/CN=localhost"
```

Note: Browsers will show a warning for self-signed certs. The nats.ws WebSocket client may need additional configuration to accept them.

## Profiles

Docker Compose profiles control which services start:

| Command | Services Started |
|---|---|
| `docker compose up -d` | nkey-init, nats, auth-service |
| `docker compose --profile dashboard up -d` | + dashboard (Nginx) |
| `docker compose --profile cli run --rm demo-client` | + demo-client (one-shot) |

## Environment File

Copy `.env.example` to `.env`:

```bash
# PingOne OIDC Configuration
PING_ISSUER_URL=https://auth.pingone.com/<environment-id>/as

# Admin app
PING_CLIENT_ID=<admin-client-id>
PING_CLIENT_SECRET=<admin-client-secret>

# Publisher app
PING_PUB_CLIENT_ID=<publisher-client-id>
PING_PUB_CLIENT_SECRET=<publisher-client-secret>

# Subscriber app
PING_SUB_CLIENT_ID=<subscriber-client-id>
PING_SUB_CLIENT_SECRET=<subscriber-client-secret>

# Auth service password
AUTH_SERVICE_PASSWORD=callout-secret
```

## Ports

| Port | Protocol | Service | Purpose |
|---|---|---|---|
| 4222 | TLS | NATS | Client connections (Go, CLI) |
| 8443 | WSS | NATS | WebSocket connections (browser) |
| 8222 | HTTP | NATS | Monitoring endpoint |
| 443 | HTTPS | Nginx | Dashboard + token proxy |
| 80 | HTTP | Nginx | Redirect to HTTPS |
