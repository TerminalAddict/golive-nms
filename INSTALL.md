# GoLive NMS installation guide

This guide installs the NMS server with Docker Compose, enrolls Linux agents and optional remote site collectors, and lists the firewall flows required for each feature.

## 1. Decide the hostname and ports

For a production deployment, use a DNS name such as `nms.example.com` that resolves to the Docker host. The default published ports are:

| Purpose | Default host port | Protocol | Required |
| --- | ---: | --- | --- |
| Management web interface and initial enrollment | `8443` | TCP/HTTPS | Yes |
| Public ACME certificate validation | `80` | TCP/HTTP | Public TLS only |
| Agent, remote collector, and Monit collector | `9443` | TCP/HTTPS with mTLS or authentication | Yes for those integrations |
| Syslog receiver | `5514` | TCP and UDP | Optional |
| SNMP trap receiver | `1162` | UDP | Optional |

You may change these in `.env`. For a normal direct-Caddy public HTTPS URL, set
`GOLIVE_WEB_PORT=443`. The guided installer publishes port 80 directly in
`direct` mode or connects Apache to Caddy through a loopback-only port in
`apache` mode. Public certificate validation still arrives on public port 80.

Do not expose PostgreSQL `5432`, VictoriaMetrics `8428`, VictoriaLogs `9428`, or the internal application port `8080`. They are private Docker-network services.

## 2. Guided installation (recommended)

Install Git and Docker Engine with the Compose plugin, then confirm these
commands work:

```sh
git --version
docker --version
docker compose version
```

Clone the complete source repository and run the bootstrap installer:

```sh
sudo mkdir -p /opt/golive-nms
sudo chown "$USER":"$USER" /opt/golive-nms
git clone https://github.com/TerminalAddict/golive-nms.git /opt/golive-nms
cd /opt/golive-nms
./install.sh
```

The installer generates strong secrets, writes `.env` with mode `0600`,
configures the chosen TLS layout, validates Compose, builds the GoLive app and
backup images locally, and starts the services. It offers these TLS modes:

- `direct`: Caddy owns public ports 80 and 443.
- `apache`: Apache keeps public port 80 and proxies only ACME challenges to
  Caddy on `127.0.0.1:18080`; the UI defaults to HTTPS port 8443.
- `internal`: Caddy uses its private CA for a LAN-only or non-public hostname.

For a host like `golive.home.example.com` where Apache already owns port 80:

```sh
./install.sh \
  --domain golive.home.example.com \
  --admin-email admin@example.com \
  --tls apache
```

On Debian/Ubuntu the Apache site and required proxy modules are enabled
automatically. On other Apache layouts, the generated file
`deploy/apache-golive-acme.generated.conf` is retained for manual installation.

For an unattended direct-Caddy installation:

```sh
./install.sh \
  --domain nms.example.com \
  --admin-email admin@example.com \
  --tls direct \
  --yes
```

To deliberately discard an existing GoLive database and all other GoLive Docker
volumes before reinstalling, use:

```sh
./install.sh --fresh
```

The installer requires typing `DELETE` before removing volumes unless `--yes`
is also supplied. It never removes containers or volumes belonging to other
Compose projects.

### Private/internal certificate trust

For `--tls internal`, export Caddy's root after startup:

```sh
docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt ./golive-caddy-root.crt
sudo cp ./golive-caddy-root.crt /usr/local/share/ca-certificates/golive-caddy-root.crt
sudo update-ca-certificates
```

Restart the browser after trusting it. Firefox may require importing the file
under Settings → Privacy & Security → Certificates → Authorities. Trust this
root on hosts that use the management URL for initial agent enrollment.

## 3. Manual installation

If you prefer to manage every file yourself, clone the same repository and copy
the environment template:

```sh
git clone https://github.com/TerminalAddict/golive-nms.git ~/golive-nms
cd ~/golive-nms
cp .env.example .env
chmod 600 .env
```

## 4. Configure `.env` manually

Edit the file:

```sh
nano .env
```

At minimum, change every `change-me` value and set the hostname:

```ini
GOLIVE_DOMAIN=nms.example.com
GOLIVE_WEB_PORT=443
GOLIVE_COLLECTOR_PORT=9443

POSTGRES_PASSWORD=use-a-long-random-password
GOLIVE_ENCRYPTION_KEY=exactly-32-random-characters
GOLIVE_ADMIN_EMAIL=admin@example.com
GOLIVE_ADMIN_PASSWORD=use-a-different-long-password
GOLIVE_AGENT_TOKEN=use-a-long-random-emergency-ingestion-token
GOLIVE_MONIT_PASSWORD=use-a-long-random-monit-password
GOLIVE_BACKUP_PASSPHRASE=use-a-long-random-backup-passphrase
```

Generate secrets with tools such as:

```sh
openssl rand -hex 24
openssl rand -base64 32
```

`GOLIVE_ENCRYPTION_KEY` protects stored credentials and the internal certificate authority. Back it up securely; losing it makes encrypted secrets unusable. The backup passphrase is separately required to restore backup archives.

If you keep the default nonstandard web port, use:

```ini
GOLIVE_WEB_PORT=8443
GOLIVE_DOMAIN=nms.example.com
```

and browse to `https://nms.example.com:8443`.

## 5. Open the server firewall

For a server using the default ports and UFW:

```sh
sudo ufw allow 8443/tcp comment 'GoLive NMS web and enrollment'
sudo ufw allow 9443/tcp comment 'GoLive agents collectors and Monit'
```

Only add the optional receivers when used:

```sh
sudo ufw allow 5514/tcp comment 'GoLive syslog TCP'
sudo ufw allow 5514/udp comment 'GoLive syslog UDP'
sudo ufw allow 1162/udp comment 'GoLive SNMP traps'
```

For firewalld:

```sh
sudo firewall-cmd --permanent --add-port=8443/tcp
sudo firewall-cmd --permanent --add-port=9443/tcp
sudo firewall-cmd --reload
```

Prefer source-restricted rules. For example, permit `9443/tcp` only from managed server and site networks, and permit syslog/trap ports only from network devices. If the NMS is behind NAT, forward the selected public ports to the same ports on the Docker host.

## 6. Start the NMS manually

Validate the rendered configuration, build the local images, and start everything:

```sh
docker compose config -q
docker compose up -d --build --wait
docker compose ps
```

All services should be running and the application, database, and proxy should report healthy. Inspect failures with:

```sh
docker compose logs --tail=200 app
docker compose logs --tail=200 caddy
docker compose logs --tail=200 postgres
```

Open the management URL and sign in using `GOLIVE_ADMIN_EMAIL` and `GOLIVE_ADMIN_PASSWORD`.

## 7. Initial web configuration

After signing in:

1. Open **Settings** and create the required sites/locations.
2. Create manager, site-manager, and viewer accounts as needed.
3. Assign site grants to site managers and scoped viewers.
4. Add encrypted SMTP, Slack, Teams, SNMP, RouterOS, or SSH credentials.
5. Create alert channels and select their site, failure/recovery behavior, and repeat interval.
6. Add devices, their parent relationships, and service checks.
7. Test notification delivery before relying on it operationally.

## 8. Install a Linux agent

The agent is a static binary and does not require a language runtime or shared-library packages. It initiates all connections; no inbound firewall port is required on the monitored host.

### Generate an enrollment token

In GoLive, open **Settings → Agents and collectors**, select **Linux agent**, choose its site, and generate a one-time token. It expires after 15 minutes and can be used once.

The two relevant URLs are:

- Enrollment: the management URL, such as `https://nms.example.com` or `https://nms.example.com:8443`.
- Reports/actions: the collector URL, normally `https://nms.example.com:9443`.

The agent generates its private key locally. The private key is never sent to the NMS.

### Debian or Ubuntu package

Download the release package matching the host architecture, then install it:

```sh
uname -m
wget https://github.com/TerminalAddict/golive-nms/releases/download/VERSION/golive-agent_VERSION_linux_amd64.deb
sudo dpkg -i ./golive-agent_VERSION_linux_amd64.deb
```

For ARM64, use the `arm64` package. Edit `/etc/golive-agent.env`:

```ini
GOLIVE_SERVER=https://nms.example.com:9443
GOLIVE_ENROLL_URL=https://nms.example.com
GOLIVE_ENROLLMENT_TOKEN=PASTE_THE_ONE_TIME_TOKEN
```

Then start the service:

```sh
sudo systemctl enable --now golive-agent
sudo journalctl -u golive-agent -f
```

### RPM-based distributions

```sh
sudo rpm -Uvh ./golive-agent_VERSION_linux_amd64.rpm
# or: sudo dnf install ./golive-agent_VERSION_linux_amd64.rpm
sudoedit /etc/golive-agent.env
sudo systemctl enable --now golive-agent
sudo journalctl -u golive-agent -f
```

### Alpine Linux

```sh
sudo apk add --allow-untrusted ./golive-agent_VERSION_linux_amd64.apk
sudo vi /etc/golive-agent.env
sudo rc-update add golive-agent default
sudo rc-service golive-agent start
tail -f /var/log/golive-agent.log
```

### Portable tarball or source-based distribution

```sh
tar -xzf golive-agent_VERSION_linux_amd64.tar.gz
cd golive-agent_VERSION_linux_amd64
sudo ./install.sh
sudoedit /etc/golive-agent.env
sudo systemctl start golive-agent
```

Alternatively, install only the static binary and run the generated command directly:

```sh
sudo install -m 0755 golive-agent /usr/bin/golive-agent
sudo install -d -m 0700 /var/lib/golive-agent
sudo /usr/bin/golive-agent \
  -server https://nms.example.com:9443 \
  -enroll-url https://nms.example.com \
  -enroll-token ONE_TIME_TOKEN \
  -state-dir /var/lib/golive-agent \
  -once
```

After successful enrollment, remove `GOLIVE_ENROLL_URL` and `GOLIVE_ENROLLMENT_TOKEN` from `/etc/golive-agent.env`, leaving only:

```ini
GOLIVE_SERVER=https://nms.example.com:9443
```

Restart and verify:

```sh
sudo systemctl restart golive-agent
sudo systemctl status golive-agent
```

The host should appear in GoLive with a recent **last seen** time and agent inventory. If it does not, inspect the journal and test connectivity:

```sh
getent hosts nms.example.com
curl -v https://nms.example.com/
openssl s_client -connect nms.example.com:9443 -servername nms.example.com </dev/null
```

The collector listener requires a client certificate for agent endpoints, so an unauthenticated HTTP request may be rejected; a completed TLS connection still confirms routing and firewall reachability.

## 9. Install a remote site collector

A remote collector runs checks from another site and is useful when devices are behind a site firewall or private address space. It needs no inbound port. It connects outbound to the NMS and then connects to monitored targets within its assigned site.

In **Settings → Agents and collectors**, select **Remote site collector**, assign the site, and generate a token. Install the matching `golive-collector` package or tarball, then configure:

```ini
GOLIVE_SERVER=https://nms.example.com:9443
GOLIVE_ENROLL_URL=https://nms.example.com
GOLIVE_ENROLLMENT_TOKEN=PASTE_THE_ONE_TIME_TOKEN
```

Start and verify it:

```sh
sudo systemctl enable --now golive-collector
sudo journalctl -u golive-collector -f
```

Remove the enrollment URL/token after the certificate has been issued. The central scheduler automatically resumes a site's checks if its collector is unavailable or revoked.

## 10. Configure Monit

Monit sends outbound HTTPS to the collector port; it needs no inbound firewall rule on the Monit host. Add:

```monit
set eventqueue basedir /var/lib/monit/events slots 1000
set mmonit https://monit:YOUR_MONIT_PASSWORD@nms.example.com:9443/collector
```

Then validate and reload:

```sh
sudo monit -t
sudo monit reload
sudo monit status
```

Use the username and password configured by `GOLIVE_MONIT_USERNAME` and `GOLIVE_MONIT_PASSWORD`.

## 11. Complete firewall communication matrix

“Source → destination” describes the initiating connection.

| Source → destination | Port/protocol | Reason |
| --- | --- | --- |
| Administrator browser → NMS | Web port, TCP/HTTPS (`8443` default or `443`) | UI, API, and initial enrollment |
| Linux agent → NMS | Web port, TCP/HTTPS | One-time CSR enrollment only |
| Linux agent → NMS | `9443/tcp` HTTPS | Ongoing mTLS reports, action polling, and results |
| Remote collector → NMS | Web port, TCP/HTTPS | One-time CSR enrollment only |
| Remote collector → NMS | `9443/tcp` HTTPS | Assignments and check results |
| Monit host → NMS | `9443/tcp` HTTPS | Monit XML collector |
| Device/syslog sender → NMS | `5514/tcp` or `5514/udp` | Optional syslog ingestion |
| SNMP device → NMS | `1162/udp` | Optional SNMP traps |
| NMS or remote collector → target | ICMP echo/reply | Ping checks |
| NMS or remote collector → target | Configured TCP port | HTTP, TCP, TLS, SSH, SMTP, MariaDB, PostgreSQL, and other service checks |
| NMS or remote collector → SNMP target | `161/udp` by default | SNMP polling |
| NMS or remote collector → MikroTik | `8728/tcp` or `8729/tcp` | RouterOS API/API-SSL checks |
| NMS → managed device | `22/tcp` | Optional SSH configuration backup |
| NMS → SMTP server | Configured SMTP port, commonly `25`, `465`, or `587` TCP | Email alerts |
| NMS → Slack/Teams | `443/tcp` HTTPS | Webhook alerts |
| NMS → OIDC provider | `443/tcp` HTTPS | SSO discovery and authentication |
| Docker host → Docker Hub/GitHub | `443/tcp` HTTPS | Third-party base images, source updates, and release packages |
| All participating hosts → DNS/NTP | `53` TCP/UDP and `123/udp`, as locally required | Name resolution and correct certificate/time validation |

Stateful firewalls automatically permit reply traffic. Agents and collectors do not listen for NMS-initiated connections. When using a remote collector, allow its host—not necessarily the central NMS—to reach the site's monitored devices.

## 12. Backups

The backup container creates encrypted archives in the `backup-data` Docker volume. Create an immediate backup before upgrades:

```sh
cd /opt/golive-nms
docker compose run --rm backup backup
```

Copy encrypted archives off the Docker host as part of the normal backup policy. Test restoration periodically; the backup passphrase is mandatory.

## 13. Upgrade safely

Update only this Compose project; do not stop every container on the host:

```sh
cd /opt/golive-nms
docker compose run --rm backup backup
git pull --ff-only
docker compose up -d --build --wait
docker image prune
docker compose ps
```

Upgrade an agent by installing the newer package over the existing one. Its state in `/var/lib/golive-agent` is retained, so it does not need a new enrollment token:

```sh
sudo dpkg -i ./golive-agent_NEW_VERSION_linux_amd64.deb
sudo systemctl restart golive-agent
```

## 14. Publish a release

The repository intentionally runs no CI for normal pushes or pull requests. A GitHub release is created only when you push a tag beginning with `v`:

```sh
git switch main
git pull --ff-only
git tag -a v0.1.0 -m "GoLive NMS v0.1.0"
git push origin v0.1.0
```

The release workflow publishes:

- Agent and remote collector tarballs for Linux amd64 and arm64.
- DEB, RPM, and APK packages.
- Checksums and software bills of materials.
- A GitHub Release containing the downloadable assets.

The server and backup Docker images are deliberately not published. Each server
installation builds them locally from its cloned source tree.

## 15. Troubleshooting checklist

1. Confirm DNS resolves to the expected address from the agent/collector host.
2. Confirm clocks are synchronized; certificate validation is time-sensitive.
3. Confirm the enrollment web port and ongoing collector port are not confused.
4. Confirm the one-time token is unused and less than 15 minutes old.
5. Confirm the public web certificate is trusted by the enrolling host.
6. Check `docker compose ps` and application logs on the NMS.
7. Check `journalctl -u golive-agent` or `journalctl -u golive-collector` remotely.
8. Check NAT forwarding and both network and host firewalls.
9. Verify the identity has not been revoked in Settings.
10. For monitoring failures, verify the NMS or assigned collector can reach the target protocol and port directly.
