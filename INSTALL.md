# GoLive NMS installation guide

This guide installs the NMS server with Docker Compose, enrolls Linux agents and optional remote site collectors, and lists the firewall flows required for each feature.

## 1. Decide the hostname and ports

For a production deployment, use a DNS name such as `nms.example.com` that resolves to the Docker host. The default published ports are:

| Purpose | Default host port | Protocol | Required |
| --- | ---: | --- | --- |
| Management web interface and initial enrollment | `8443` | TCP/HTTPS | Yes |
| Public ACME certificate validation | `80` | TCP/HTTP | Public TLS only |
| Agent, remote collector, and Monit collector | `9443` | TCP/HTTPS with mTLS or authentication | Yes for those integrations |
| Monit remote control | `2812` on each Monit host | TCP/HTTP(S), NMS → Monit host | Only when remote control is enabled |
| Syslog receiver | `5514` | TCP and UDP | Optional |
| SNMP trap receiver | `1162` | UDP | Optional |

You may change these in `.env`. For a normal direct-Caddy public HTTPS URL, set
`GOLIVE_WEB_PORT=443`. The direct override publishes port 80 to Caddy; the
Apache and Nginx overrides connect the existing web server to Caddy through a
loopback-only port. Public certificate validation still arrives on public port
80.

Do not expose PostgreSQL `5432`, VictoriaMetrics `8428`, VictoriaLogs `9428`, or the internal application port `8080`. They are private Docker-network services.

## 2. Clone and inspect the source

Install Git and Docker Engine with the Compose plugin, then confirm these
commands work:

```sh
git --version
docker --version
docker compose version
```

Clone the complete source repository:

```sh
sudo mkdir -p /opt/golive-nms
sudo chown "$USER":"$USER" /opt/golive-nms
git clone https://github.com/TerminalAddict/golive-nms.git /opt/golive-nms
cd /opt/golive-nms
```

The server and backup images are built locally from the checked-out Dockerfile.
You can inspect the Dockerfile, Compose file, TLS configuration, and every
command before running anything. GoLive does not require a published application
image or an installation script.

Create the local environment file:

```sh
cp .env.example .env
chmod 600 .env
nano .env
```

Replace every `change-me` value. Section 4 lists all required values and secret
generation commands.

## 3. Choose a TLS layout

Four small Compose overrides are supplied:

- `direct`: Caddy owns public ports 80 and 443.
- `apache`: Apache keeps public port 80 and proxies only ACME challenges to
  Caddy on `127.0.0.1:18080`; the UI defaults to HTTPS port 8443.
- `nginx`: Nginx keeps public port 80 and proxies only ACME challenges to the
  same loopback-only Caddy listener.
- `internal`: Caddy uses its private CA for a LAN-only or non-public hostname.

### Existing Apache on port 80

Set these values in `.env`:

```ini
GOLIVE_DOMAIN=golive.home.example.com
GOLIVE_WEB_PORT=8443
GOLIVE_ACME_PORT=18080
```

Select the Apache Compose override:

```sh
cp deploy/compose.apache.yml compose.override.yml
```

Create a host-specific Apache configuration from the supplied template:

```sh
cp deploy/apache-golive-acme.conf deploy/apache-golive-acme.generated.conf
nano deploy/apache-golive-acme.generated.conf
```

Replace `@GOLIVE_DOMAIN@` with the hostname and `@ACME_PORT@` with `18080`, then
review and install it on Debian/Ubuntu:

```sh
sudo install -m 0644 deploy/apache-golive-acme.generated.conf /etc/apache2/sites-available/golive-acme.conf
sudo a2enmod proxy proxy_http
sudo a2ensite golive-acme.conf
sudo apachectl configtest
sudo systemctl reload apache2
```

This leaves the existing default Apache vhost and dehydrated alias untouched.
Only requests for this hostname's `/.well-known/acme-challenge/` path are sent
to Caddy.

### Existing Nginx on port 80

Set these values in `.env`:

```ini
GOLIVE_DOMAIN=golive.home.example.com
GOLIVE_WEB_PORT=8443
GOLIVE_ACME_PORT=18080
```

Select the Nginx Compose override:

```sh
cp deploy/compose.nginx.yml compose.override.yml
```

Create and inspect a host-specific Nginx configuration:

```sh
cp deploy/nginx-golive-acme.conf deploy/nginx-golive-acme.generated.conf
nano deploy/nginx-golive-acme.generated.conf
```

Replace `@GOLIVE_DOMAIN@` with the hostname, `@ACME_PORT@` with `18080`, and
`@WEB_PORT@` with `8443`.

On Debian/Ubuntu, install and enable the server block explicitly:

```sh
sudo install -m 0644 deploy/nginx-golive-acme.generated.conf /etc/nginx/sites-available/golive-acme.conf
sudo ln -s /etc/nginx/sites-available/golive-acme.conf /etc/nginx/sites-enabled/golive-acme.conf
sudo nginx -t
sudo systemctl reload nginx
```

On distributions that load `/etc/nginx/conf.d/*.conf`, use:

```sh
sudo install -m 0644 deploy/nginx-golive-acme.generated.conf /etc/nginx/conf.d/golive-acme.conf
sudo nginx -t
sudo systemctl reload nginx
```

The challenge path is proxied to Caddy with the original hostname. Other HTTP
requests for the GoLive hostname are redirected to HTTPS port 8443. Existing
Nginx sites for other hostnames are unaffected.

### Caddy directly on public ports 80 and 443

```sh
cp deploy/compose.direct.yml compose.override.yml
```

Set `GOLIVE_WEB_PORT=443` and the public hostname in `.env`. Ensure no Apache,
Nginx, or other service already owns host ports 80 or 443.

### Private or LAN-only TLS

```sh
cp deploy/compose.internal.yml compose.override.yml
```

Set `GOLIVE_WEB_PORT=8443` and the desired hostname in `.env`.

#### Trust the private certificate authority

For the internal TLS layout, export Caddy's root after startup:

```sh
docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt ./golive-caddy-root.crt
sudo cp ./golive-caddy-root.crt /usr/local/share/ca-certificates/golive-caddy-root.crt
sudo update-ca-certificates
```

Restart the browser after trusting it. Firefox may require importing the file
under Settings → Privacy & Security → Certificates → Authorities. Trust this
root on hosts that use the management URL for initial agent enrollment.

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

Inspect the selected files and rendered configuration, then build the local
images and start everything:

```sh
docker compose config -q
docker compose up -d --build --wait
docker compose ps
```

To see the entire resolved configuration rather than only validating it, run
`docker compose config` without `-q`.

### Deliberately start again with empty data

The following command irreversibly removes only this Compose project's database,
metrics, logs, Caddy certificates, and backup volume. It does not remove the
source checkout or containers belonging to other projects:

```sh
cd /opt/golive-nms
docker compose down -v --remove-orphans
```

Review `.env` and `compose.override.yml`, then create the empty installation:

```sh
docker compose config
docker compose up -d --build --wait
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

The tarball includes a static binary, example configuration, and service files.
Install them explicitly so each privileged filesystem change is visible:

```sh
tar -xzf golive-agent_VERSION_linux_amd64.tar.gz
cd golive-agent_VERSION_linux_amd64
sudo install -m 0755 golive-agent /usr/bin/golive-agent
sudo install -d -m 0700 /var/lib/golive-agent
sudo install -m 0600 golive-agent.env.example /etc/golive-agent.env
sudo install -m 0644 deploy/golive-agent.service /etc/systemd/system/golive-agent.service
sudoedit /etc/golive-agent.env
sudo systemctl daemon-reload
sudo systemctl enable --now golive-agent
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
The first valid report creates the host automatically. In **Devices**, select
the host name to edit its address, site, parent relationship, type, and tags and
to inspect every service reported by Monit.

### Remote control of Monit services

GoLive supports five Monit service commands: **start**, **stop**, **restart**, **monitor**, and **unmonitor**. This traffic travels in the opposite direction to reports: the GoLive `app` container connects to TCP port `2812` on the monitored host.

On each Monit host, enable the HTTP interface with a dedicated account:

```monit
set httpd port 2812 and
    allow golive:REPLACE_WITH_A_LONG_UNIQUE_PASSWORD
    allow localhost
```

Then restrict TCP `2812` at the host or network firewall to the NMS server's source IP. For UFW, run this on the **monitored host**, replacing the example address with the address that host sees for your NMS server:

```bash
sudo ufw allow proto tcp from 103.123.165.253 to any port 2812 comment 'GoLive Monit control'
sudo monit -t
sudo systemctl reload monit
```

Do not expose `2812` to the whole internet. Basic authentication over plain HTTP does not encrypt the password or command. Use a private network/VPN between GoLive and the host, or configure TLS on Monit's HTTP interface and use an `https://host:2812` endpoint with a certificate trusted by the GoLive container.

Configure GoLive:

1. Open **Settings → Network credentials** and add a **Monit remote-control credential** using the same username and password.
2. Open **Devices**, click the device name, and find **Monit remote control**.
3. Enter the URL reachable from the GoLive container, such as `http://10.0.0.12:2812`, select the credential, and save.
4. Select **Test connection**. A successful result confirms network access, authentication, and a working Monit HTTP interface.
5. Use the action buttons beside a reported Monit service. GoLive immediately shows whether Monit accepted the command and keeps recent successes and failures visible. The service status itself is confirmed when the next Monit report arrives.

The `set mmonit .../collector` credentials remain separate from the port-2812 credentials. `register without credentials` is fine for reporting; it does not supply credentials for remote commands.

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
| NMS → Monit host | `2812/tcp` HTTP(S) | Optional start/stop/restart/monitor/unmonitor commands; restrict source to the NMS |
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
