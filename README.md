# GoLive NMS

GoLive NMS is a modern, self-hosted network management server with site-scoped inventory, service/SNMP/RouterOS monitoring, latency and host metrics, dependency-aware incidents, Monit, encrypted credentials, alerts, maps, syslog/traps, configuration versions, local/OIDC authentication, mTLS collectors, and portable Linux agents.

For a complete deployment and firewall walkthrough, see [INSTALL.md](INSTALL.md).

## Quick start

```sh
git clone https://github.com/TerminalAddict/golive-nms.git
cd golive-nms
cp .env.example .env
chmod 600 .env
nano .env
```

Replace every `change-me` value in `.env`; generate independent secrets with
`openssl rand -hex 32`. Then select exactly one readable Compose override.

For Caddy on public ports 80 and 443:

```sh
cp deploy/compose.direct.yml compose.override.yml
```

For an existing Apache on public port 80:

```sh
cp deploy/compose.apache.yml compose.override.yml
```

For an existing Nginx on public port 80:

```sh
cp deploy/compose.nginx.yml compose.override.yml
```

For private/LAN TLS:

```sh
cp deploy/compose.internal.yml compose.override.yml
```

Review the rendered configuration and build locally:

```sh
docker compose config
docker compose up -d --build --wait
docker compose ps
```

The Apache and Nginx layouts each need one small, explicit host configuration
before certificate issuance. See [INSTALL.md](INSTALL.md) for those commands,
firewall rules, private CA trust, clean resets, and agent setup. No installation
script or published GoLive Docker image is used.

## Safe updates

Do not stop every container on the Docker host. Update only this project:

```sh
cd golive-nms
docker compose run --rm backup backup
git pull --ff-only
docker compose up -d --build --wait
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

The agent reports identity, OS/package manager and pending updates, CPU, load, memory/swap, root filesystem, aggregate network counters, process count, and uptime. Version tags produce static amd64/arm64 tarballs plus `.deb`, `.rpm`, and `.apk` packages with systemd and OpenRC definitions. Private keys are generated locally during one-time mTLS enrollment. Normal pushes and pull requests run no GitHub Actions; only `v*` release tags trigger automation.

## Monit integration

Set the Monit collector credentials in `.env`, then add this to each Monit installation:

```monit
set eventqueue basedir /var/lib/monit/events slots 1000
set mmonit https://monit:YOUR_MONIT_PASSWORD@nms.example.com:9443/collector
```

GoLive accepts Monit v2 status and event XML, including gzip compression, maintains host/service state, and turns failing Monit events into GoLive incidents.

GoLive can also start, stop, restart, monitor, and unmonitor reported services through Monit's HTTP interface. Create a **Monit remote-control credential** under **Settings → Network credentials**, open the device's **Manage** dialog, and set its endpoint (for example `http://10.0.0.12:2812`). The Monit host must allow inbound TCP `2812` from the NMS server only. See [INSTALL.md](INSTALL.md#remote-control-of-monit-services) for the complete and security-conscious configuration.

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
- Role- and site-scoped Monit start, stop, restart, monitor, and unmonitor controls with encrypted credentials and an action audit trail.
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

## Publishing a release

Normal commits do not run GitHub Actions. To publish a release, tag the commit and push that tag:

```sh
git switch main
git pull --ff-only
git tag -a v0.1.0 -m "GoLive NMS v0.1.0"
git push origin v0.1.0
```

The tag workflow creates the GitHub Release with agent and collector packages,
checksums, and SBOMs. Server Docker images are not published; server installs
continue to build them locally from the cloned source tree.

## License

GoLive NMS is licensed under the [GNU Affero General Public License v3.0](LICENSE).
