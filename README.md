# GoLive NMS

GoLive NMS is a modern, self-hosted network management server with site-scoped inventory, service/SNMP/RouterOS monitoring, latency and host metrics, dependency-aware incidents, Monit, encrypted credentials, alerts, maps, syslog/traps, configuration versions, local/OIDC authentication, mTLS collectors, and portable Linux agents.

For a complete deployment and firewall walkthrough, see [INSTALL.md](INSTALL.md).

## Quick start

```sh
mkdir golive-nms && cd golive-nms
mkdir -p deploy
wget -O docker-compose.yml https://raw.githubusercontent.com/TerminalAddict/golive-nms/main/docker-compose.yml
wget -O .env https://raw.githubusercontent.com/TerminalAddict/golive-nms/main/.env.example
wget -O deploy/Caddyfile https://raw.githubusercontent.com/TerminalAddict/golive-nms/main/deploy/Caddyfile
```

Edit `.env` and replace every `change-me` value, then run:

```sh
docker compose up -d --wait
```

Open `https://localhost:8443`. The development Compose file uses Caddy's internal CA for localhost, so the browser will require a one-time certificate exception. For a public hostname, set `GOLIVE_DOMAIN` and expose standard ports 80/443 at the host or upstream proxy.

## Safe updates

Do not stop every container on the Docker host. Update only this project:

```sh
cd golive-nms
docker compose pull
docker compose up -d --wait
docker image prune
```

Database migrations are additive and run during application startup. The bundled backup service creates encrypted archives on its configured schedule; take an on-demand backup before major upgrades.

## Current monitoring workflow

1. Add a device in the web interface.
2. Open Devices and add an HTTP(S) check such as `https://example.com`, or a TCP check such as `router.example.com:22`.
3. Checks run every 30 seconds by default. Failure creates one active incident; recovery resolves it automatically.
4. Dashboard and incident views update through a live event stream.

## Standalone Linux agent

The agent is a statically compiled Go binary with no runtime dependencies. Build it in the Go builder container or with Go 1.25+:

```sh
CGO_ENABLED=0 go build -trimpath -o golive-agent ./cmd/golive-agent
```

Install the binary at `/usr/bin/golive-agent`, copy [`deploy/golive-agent.service`](deploy/golive-agent.service), and create `/etc/golive-agent.env`:

```ini
GOLIVE_SERVER=https://nms.example.com:9443
GOLIVE_AGENT_TOKEN=the-token-from-your-env
```

Then enable it:

```sh
systemctl daemon-reload
systemctl enable --now golive-agent
```

The agent reports identity, OS/package manager and pending updates, CPU, load, memory/swap, root filesystem, aggregate network counters, process count, and uptime. The included GoReleaser configuration can produce static amd64/arm64 tarballs plus `.deb`, `.rpm`, and `.apk` packages with systemd and OpenRC definitions. Private keys are generated locally during one-time mTLS enrollment. GitHub Actions are intentionally disabled; releases and container images are built manually.

## Monit integration

Set the Monit collector credentials in `.env`, then add this to each Monit installation:

```monit
set eventqueue basedir /var/lib/monit/events slots 1000
set mmonit https://monit:YOUR_MONIT_PASSWORD@nms.example.com:9443/collector
```

GoLive accepts Monit v2 status and event XML, including gzip compression, maintains host/service state, and turns failing Monit events into GoLive incidents.

## Remote collectors and secure enrollment

Generate a 15-minute enrollment command in Settings. Agents and collectors create their key locally, submit a CSR, and use the issued certificate on the collector port. A site collector receives only its site's checks; central polling resumes after an outage or revocation.

## Encrypted backups

The backup service creates encrypted database, metric, and log archives. Manual operations are:

```sh
docker compose run --rm backup backup
docker compose stop app victoriametrics victorialogs
docker compose run --rm backup restore /backups/golive-YYYYMMDDTHHMMSSZ.tar.age
docker compose up -d --wait
```

## Development

The application image builds both the React frontend and Go backend:

```sh
docker compose build
docker compose up -d --wait
```

Frontend checks can run independently:

```sh
cd web
npm ci
npm run check
npm test
npm run build
```

## Implemented and upcoming

Implemented now:

- Docker Compose deployment with PostgreSQL, VictoriaMetrics, VictoriaLogs, Caddy, and container health checks.
- Responsive dark GoLive interface with health ring, device inventory, incidents, and topology canvas.
- Versioned REST endpoints under `/api/v1` and server-sent live events.
- Ping, HTTP, TCP, and SNMP v2c/v3 checks, durable scheduling, latency samples, dependency suppression, incident deduplication, acknowledgement, and recovery.
- Encrypted SNMP/SMTP/webhook credentials and email, Slack, and Teams notification channels.
- Monit `/collector` compatibility for complete status snapshots and state-change events.
- Local users, four roles, hashed sessions, service API tokens, mutation auditing, and identity-management UI.
- Enforced per-site visibility and mutation boundaries for site managers and scoped viewers, including their API tokens.
- VictoriaMetrics availability, latency, and host-performance series with an interactive historical dashboard chart.
- Static amd64/arm64 agent builds and deb/rpm/apk/tar packaging through the included GoReleaser configuration.
- One-time mTLS enrollment, revocation, site collectors, and automatic central failback.
- TCP/UDP syslog and SNMP trap ingestion with scoped search.
- Dependency topology and geographic OpenStreetMap site views.
- Host-key-pinned SSH configuration capture, encrypted versions, diffs, and change incidents.
- Scheduled age-encrypted system backups with tested isolated restore.
- Local login plus optional OpenID Connect SSO.
- Basic standalone amd64/arm64-capable Linux agent source and authenticated ingestion.
- DNS, TLS-expiry, SSH/SMTP banner, database-port, and MikroTik RouterOS checks.
- Signed, allow-listed manual and automatic remediation with cooldowns, execution leases, audit history, and a global kill switch.
- Site/device maintenance windows, incident ownership and notes, site-routed alerts, recovery selection, and repeat reminders.
- Linux OS/update inventory and configurable retention for operational PostgreSQL data.

Before an Internet-facing production rollout, operators should still perform capacity/load testing sized to their own polling intervals and device count, validate SMTP and network-vendor checks against their infrastructure, and configure off-host replication of `/backups`.

## License

GoLive NMS is licensed under the [GNU Affero General Public License v3.0](LICENSE).
