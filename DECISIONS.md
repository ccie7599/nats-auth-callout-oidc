# Architecture Decision Records

## ADR-001: Use synadia-io/callout.go Helper Library

**Date:** 2026-02-13
**Status:** Accepted
**Context:** NATS auth-callout requires subscribing to `$SYS.REQ.USER.AUTH`, decoding JWT requests, building signed response JWTs, and handling encryption. We can implement this from scratch using `nats-io/jwt/v2` or use the official Synadia helper library.
**Decision:** Use `github.com/synadia-io/callout.go` which wraps the callout protocol, handles request decoding, response signing, and error wrapping.
**Alternatives Considered:** Raw subscription to `$SYS.REQ.USER.AUTH` with manual JWT handling — more code, more surface area for protocol bugs.
**Consequences:** Additional dependency, but maintained by the NATS team. Reduces auth-callout boilerplate to a single `Authorizer` function.

## ADR-002: NKey Generation via Init Container

**Date:** 2026-02-13
**Status:** Accepted
**Context:** Auth-callout requires a shared NKey pair — the public key goes in NATS server config (`issuer`), the private seed goes to the auth service for signing response JWTs. We need to manage this without committing secrets to source.
**Decision:** Generate NKeys at Docker Compose startup via an init container (`natsio/nats-box`), write to a shared Docker volume. Both NATS server and auth-service read from the volume.
**Alternatives Considered:** Pre-generated keys in `.env` file (simpler but puts secrets in source); Vault-managed keys (overkill for Tier 1 demo).
**Consequences:** Each `docker compose up` gets fresh keys. No secrets in git. Adds startup dependency on init container.

## ADR-003: Token Transport via nats.Token()

**Date:** 2026-02-13
**Status:** Accepted
**Context:** NATS clients need to pass the OIDC bearer token to the server. Multiple fields in CONNECT options could carry it: `auth_token`, `user`/`pass`, or a custom JWT.
**Decision:** Primary: `nats.Token(bearerToken)` which maps to `ConnectOptions.Token` (JSON: `auth_token`). Fallback: also check `ConnectOptions.Password` for clients that use `nats.UserInfo("", bearerToken)`.
**Alternatives Considered:** NATS JWT-based auth (requires nsc tooling, more complex for OIDC integration); custom headers (not supported by NATS protocol).
**Consequences:** Works across all NATS client libraries. Simple for demo clients.

## ADR-004: Multi-Issuer OIDC Verification

**Date:** 2026-02-13
**Status:** Superseded by ADR-004a
**Context:** The demo needs to validate tokens from both the local mock OIDC provider and (optionally) a live PingOne tenant simultaneously.
**Decision:** Auth-service accepts comma-separated `OIDC_ISSUER_URL` values, initializes a verifier for each, and tries them in order during token validation.
**Alternatives Considered:** Single issuer with environment swap (simpler but can't demo both simultaneously); audience-based routing (requires consistent audience claims across providers).
**Consequences:** Both mock and PingOne tokens work against the same NATS cluster. Slight validation overhead for multi-issuer try-each approach.

## ADR-004a: PingOne as Sole Identity Provider

**Date:** 2026-02-13
**Status:** Accepted (supersedes ADR-004)
**Context:** After validating the auth-callout flow with a mock OIDC provider, we migrated to PingOne as the sole identity provider for a more compelling enterprise demo. Three PingOne OIDC Web Apps provide separate client credentials for admin, publisher, and subscriber roles.
**Decision:** Remove mock OIDC provider entirely. Configure PingOne as the only `OIDC_ISSUER_URL`. Use three separate PingOne applications with distinct scopes (`nats:admin`, `nats:publish`, `nats:subscribe`) via a custom `nats-api` resource. Nginx proxies token requests with server-side Basic Auth to keep client secrets off the browser.
**Alternatives Considered:** Keep mock OIDC for local dev (adds complexity, less impressive for demo); single PingOne app with all scopes (can't demonstrate per-app scope restrictions).
**Consequences:** Requires a PingOne trial tenant. Demo is more compelling for enterprise audiences. Three apps clearly show scope-based access control.

## ADR-005: TLS via Let's Encrypt + Akamai DNS-01

**Date:** 2026-02-13
**Status:** Accepted
**Context:** Enterprise demo needs TLS everywhere — NATS client connections, WebSocket, and dashboard HTTPS. Domain `nats-demo.connected-cloud.io` is managed via Akamai Edge DNS.
**Decision:** Use certbot with the `edgedns` authenticator plugin for DNS-01 challenge against Akamai Edge DNS. Same cert used for NATS TLS (port 4222), NATS WSS (port 8443), and Nginx HTTPS (port 443). Certs stored in `./certs/` (gitignored), shared into containers via bind mounts.
**Alternatives Considered:** Self-signed certs (browser warnings, less credible); HTTP-01 challenge (requires port 80 open during cert issuance); manual cert management.
**Consequences:** Valid browser-trusted TLS for all endpoints. Cert renewal requires re-running `make certs`. Internal Docker connections use `ServerName` override since container hostname `nats` doesn't match cert CN.

## ADR-006: Interactive Browser Dashboard over WSS

**Date:** 2026-02-13
**Status:** Accepted
**Context:** Need a visual, click-through demo for pre-sales presentations. CLI demo is useful but not as engaging for non-technical stakeholders.
**Decision:** Single-file HTML dashboard (`web/index.html`) served by Nginx. Uses `nats.ws` ES module from CDN for WebSocket connections to NATS on port 8443. Dark theme (GitHub dark palette) for infosec audience. Five scenario buttons trigger token fetch → WSS connect → pub/sub tests → result display. Audit panel subscribes to `auth.audit.>` for real-time auth decisions.
**Alternatives Considered:** React/Vue SPA (build step, overkill for demo); separate Node.js backend (adds complexity); CLI-only (less engaging).
**Consequences:** Zero build step. Single file to maintain. Requires WSS port 8443 exposed with valid TLS.

## ADR-007: Auth Audit Trail via NATS Pub/Sub

**Date:** 2026-02-13
**Status:** Accepted
**Context:** Dashboard needs to display real-time auth decisions (success/failure) as they happen. This provides visibility into the auth-callout flow during demos.
**Decision:** Auth-service publishes JSON events to `auth.audit.success` and `auth.audit.failure` after each authorization decision. Payload includes timestamp, subject, client IP, scopes, decision, and permissions granted. Dashboard subscribes to `auth.audit.>` via an admin-scoped WSS connection. Fire-and-forget — no JetStream, events are dropped if nobody is subscribed.
**Alternatives Considered:** JetStream with durable consumers (persistent but overkill for real-time demo display); external logging system (adds infrastructure).
**Consequences:** Lightweight, zero additional infrastructure. Audit events only visible when a subscriber is connected. Sufficient for live demo purposes.
