# GoLive NMS installation guide

This guide installs the NMS server with Docker Compose, enrolls Linux agents and optional remote site collectors, and lists the firewall flows required for each feature.

## 1. Decide the hostname and ports

For a production deployment, use a DNS name such as `nms.example.com` that resolves to the Docker host. The default published ports are:

| Purpose | Default host port | Protocol | Required |
| --- | ---: | --- | --- |
| Management web interface and initial enrollment | `8443` | TCP/HTTPS | Yes |
| Agent, remote collector, and Monit collector | `9443` | TCP/HTTPS with mTLS or authentication | Yes for those integrations |
| Syslog receiver | `5514` | TCP and UDP | Optional |
| SNMP trap receiver | `1162` | UDP | Optional |

You may change these in `.env`. For a normal public HTTPS URL, set `GOLIVE_WEB_PORT=443`. Port 80 is optional for ACME HTTP validation or redirection and is not published by the supplied Compose file; Caddy can normally validate on 443, or you can terminate TLS at an upstream reverse proxy.

Do not expose PostgreSQL `5432`, VictoriaMetrics `8428`, VictoriaLogs `9428`, or the internal application port `8080`. They are private Docker-network services.

## 2. Prepare the Docker host

Install Docker Engine with the Compose plugin, then confirm both commands work:

```sh
docker --version
docker compose version
```

Create the installation directory:

```sh
sudo mkdir -p /opt/golive-nms/deploy
sudo chown -R "$USER":"$USER" /opt/golive-nms
cd /opt/golive-nms
```

Download the deployment files:

```sh
wget -O docker-compose.yml https://raw.githubusercontent.com/golive-nms/golive-nms/main/docker-compose.yml
wget -O .env https://raw.githubusercontent.com/golive-nms/golive-nms/main/.env.example
wget -O deploy/Caddyfile https://raw.githubusercontent.com/golive-nms/golive-nms/main/deploy/Caddyfile
```

If the repository is private or the images have not yet been published, clone the repository instead and let Compose build the images locally:

```sh
git clone https://github.com/golive-nms/golive-nms.git /opt/golive-nms
cd /opt/golive-nms
```

## 3. Configure `.env`

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

## 4. Open the server firewall

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

## 5. Start the NMS

Validate the rendered configuration, pull/build images, and start everything:

```sh
docker compose config -q
docker compose pull
docker compose up -d --wait
docker compose ps
```

If published images are not available and you cloned the source, use:

```sh
docker compose up -d --build --wait
```

All services should be running and the application, database, and proxy should report healthy. Inspect failures with:

```sh
docker compose logs --tail=200 app
docker compose logs --tail=200 caddy
docker compose logs --tail=200 postgres
```

Open the management URL and sign in using `GOLIVE_ADMIN_EMAIL` and `GOLIVE_ADMIN_PASSWORD`.

## 6. Initial web configuration

After signing in:

1. Open **Settings** and create the required sites/locations.
2. Create manager, site-manager, and viewer accounts as needed.
3. Assign site grants to site managers and scoped viewers.
4. Add encrypted SMTP, Slack, Teams, SNMP, RouterOS, or SSH credentials.
5. Create alert channels and select their site, failure/recovery behavior, and repeat interval.
6. Add devices, their parent relationships, and service checks.
7. Test notification delivery before relying on it operationally.

## 7. Install a Linux agent

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
wget https://github.com/golive-nms/golive-nms/releases/download/VERSION/golive-agent_VERSION_linux_amd64.deb
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

## 8. Install a remote site collector

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

## 9. Configure Monit

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

## 10. Complete firewall communication matrix

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
| Docker host → registries/GitHub | `443/tcp` HTTPS | Image pulls and upgrades |
| All participating hosts → DNS/NTP | `53` TCP/UDP and `123/udp`, as locally required | Name resolution and correct certificate/time validation |

Stateful firewalls automatically permit reply traffic. Agents and collectors do not listen for NMS-initiated connections. When using a remote collector, allow its host—not necessarily the central NMS—to reach the site's monitored devices.

## 11. Backups

The backup container creates encrypted archives in the `backup-data` Docker volume. Create an immediate backup before upgrades:

```sh
cd /opt/golive-nms
docker compose run --rm backup backup
```

Copy encrypted archives off the Docker host as part of the normal backup policy. Test restoration periodically; the backup passphrase is mandatory.

## 12. Upgrade safely

Update only this Compose project; do not stop every container on the host:

```sh
cd /opt/golive-nms
docker compose run --rm backup backup
docker compose pull
docker compose up -d --wait
docker image prune
docker compose ps
```

When installing from a cloned source tree:

```sh
git pull --ff-only
docker compose up -d --build --wait
```

Upgrade an agent by installing the newer package over the existing one. Its state in `/var/lib/golive-agent` is retained, so it does not need a new enrollment token:

```sh
sudo dpkg -i ./golive-agent_NEW_VERSION_linux_amd64.deb
sudo systemctl restart golive-agent
```

## 13. Troubleshooting checklist

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
