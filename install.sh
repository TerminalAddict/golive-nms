#!/usr/bin/env bash
set -Eeuo pipefail

TLS_MODE=""
DOMAIN=""
ADMIN_EMAIL=""
WEB_PORT=""
ACME_PORT="18080"
FRESH=0
ASSUME_YES=0

usage() {
  cat <<'EOF'
Install GoLive NMS from the current cloned source directory.

Usage: ./install.sh [options]

  --domain NAME          DNS name (for example nms.example.com)
  --admin-email EMAIL    Initial administrator email
  --tls MODE             direct, apache, or internal
  --web-port PORT        HTTPS port (default: 443 direct, 8443 otherwise)
  --acme-port PORT       Apache-to-Caddy loopback port (default: 18080)
  --fresh                Remove this project's containers and volumes first
  --yes                  Accept defaults and do not pause before starting
  --help                  Show this help

TLS modes:
  direct    Caddy owns public ports 80 and 443 and obtains a public certificate.
  apache    Apache owns public port 80 and proxies ACME challenges to Caddy.
  internal  Caddy issues a private certificate; no public DNS is required.
EOF
}

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

while (($#)); do
  case "$1" in
    --domain) DOMAIN="${2:?Missing value for --domain}"; shift 2 ;;
    --admin-email) ADMIN_EMAIL="${2:?Missing value for --admin-email}"; shift 2 ;;
    --tls) TLS_MODE="${2:?Missing value for --tls}"; shift 2 ;;
    --web-port) WEB_PORT="${2:?Missing value for --web-port}"; shift 2 ;;
    --acme-port) ACME_PORT="${2:?Missing value for --acme-port}"; shift 2 ;;
    --fresh) FRESH=1; shift ;;
    --yes) ASSUME_YES=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) die "Unknown option: $1" ;;
  esac
done

command -v docker >/dev/null 2>&1 || die "Docker is not installed"
docker compose version >/dev/null 2>&1 || die "The Docker Compose plugin is not installed"
command -v openssl >/dev/null 2>&1 || die "openssl is required to generate secrets"
for required_file in docker-compose.yml Dockerfile deploy/Caddyfile deploy/Caddyfile.internal deploy/apache-golive-acme.conf; do
  [[ -f "$required_file" ]] || die "Missing ${required_file}. Clone the complete Git repository before running install.sh"
done

if [[ -r /dev/tty ]]; then
  prompt() {
    local message="$1" default="$2" answer
    read -r -p "${message} [${default}]: " answer </dev/tty
    printf '%s' "${answer:-$default}"
  }
else
  prompt() { printf '%s' "$2"; }
fi

if [[ -z "$DOMAIN" ]]; then
  DOMAIN="$(prompt 'Management hostname' 'localhost')"
fi
if [[ -z "$ADMIN_EMAIL" ]]; then
  ADMIN_EMAIL="$(prompt 'Initial administrator email' 'admin@example.com')"
fi

if [[ -z "$TLS_MODE" ]]; then
  if [[ "$DOMAIN" == "localhost" ]]; then
    TLS_MODE="internal"
  elif [[ -r /dev/tty && "$ASSUME_YES" -eq 0 ]]; then
    default_tls_choice="1"
    if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet apache2 2>/dev/null; then
      default_tls_choice="2"
      printf '\nActive Apache detected; the Apache TLS layout is the default.\n' >/dev/tty
    fi
    printf '\nTLS setup:\n  1) Caddy directly owns public ports 80/443\n  2) Apache owns port 80; proxy ACME to Caddy\n  3) Private/internal certificate\n' >/dev/tty
    choice="$(prompt 'Choose TLS setup' "$default_tls_choice")"
    case "$choice" in
      1) TLS_MODE="direct" ;;
      2) TLS_MODE="apache" ;;
      3) TLS_MODE="internal" ;;
      *) die "TLS choice must be 1, 2, or 3" ;;
    esac
  else
    TLS_MODE="direct"
  fi
fi

case "$TLS_MODE" in
  direct) WEB_PORT="${WEB_PORT:-443}" ;;
  apache|internal) WEB_PORT="${WEB_PORT:-8443}" ;;
  *) die "--tls must be direct, apache, or internal" ;;
esac
[[ "$WEB_PORT" =~ ^[0-9]+$ ]] && ((WEB_PORT >= 1 && WEB_PORT <= 65535)) || die "Invalid web port"
[[ "$ACME_PORT" =~ ^[0-9]+$ ]] && ((ACME_PORT >= 1 && ACME_PORT <= 65535)) || die "Invalid ACME port"
[[ "$DOMAIN" =~ ^[A-Za-z0-9.-]+$ ]] || die "Invalid hostname"
[[ "$ADMIN_EMAIL" != *$'\n'* && "$ADMIN_EMAIL" == *@* ]] || die "Invalid administrator email"

if ((FRESH)) && [[ -f docker-compose.yml ]]; then
  if ((ASSUME_YES == 0)); then
    [[ -r /dev/tty ]] || die "--fresh requires a terminal or --yes"
    printf '\nThis deletes ONLY the GoLive Docker volumes in %s.\nType DELETE to continue: ' "$PWD" >/dev/tty
    read -r confirmation </dev/tty
    [[ "$confirmation" == "DELETE" ]] || die "Fresh installation cancelled"
  fi
  POSTGRES_PASSWORD=fresh-install-reset \
    GOLIVE_ADMIN_PASSWORD=fresh-install-reset \
    GOLIVE_ENCRYPTION_KEY=fresh-install-reset \
    GOLIVE_AGENT_TOKEN=fresh-install-reset \
    GOLIVE_MONIT_PASSWORD=fresh-install-reset \
    GOLIVE_BACKUP_PASSPHRASE=fresh-install-reset \
    docker compose down -v --remove-orphans
fi

if [[ -f .env && "$FRESH" -eq 0 ]]; then
  die ".env already exists. Use the existing installation, or run with --fresh to reset it"
fi

POSTGRES_PASSWORD="$(openssl rand -hex 32)"
ENCRYPTION_KEY="$(openssl rand -hex 32)"
ADMIN_PASSWORD="$(openssl rand -hex 20)"
AGENT_TOKEN="$(openssl rand -hex 32)"
MONIT_PASSWORD="$(openssl rand -hex 32)"
BACKUP_PASSPHRASE="$(openssl rand -hex 32)"

if [[ "$WEB_PORT" == "443" ]]; then
  MANAGEMENT_URL="https://${DOMAIN}"
else
  MANAGEMENT_URL="https://${DOMAIN}:${WEB_PORT}"
fi

cat >.env <<EOF
GOLIVE_VERSION=latest
GOLIVE_DOMAIN=${DOMAIN}
GOLIVE_WEB_PORT=${WEB_PORT}
GOLIVE_COLLECTOR_PORT=9443
POSTGRES_DB=golive
POSTGRES_USER=golive
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
GOLIVE_ENCRYPTION_KEY=${ENCRYPTION_KEY}
GOLIVE_ADMIN_EMAIL=${ADMIN_EMAIL}
GOLIVE_ADMIN_PASSWORD=${ADMIN_PASSWORD}
GOLIVE_AGENT_TOKEN=${AGENT_TOKEN}
GOLIVE_MONIT_USERNAME=monit
GOLIVE_MONIT_PASSWORD=${MONIT_PASSWORD}
GOLIVE_SYSLOG_PORT=5514
GOLIVE_TRAP_PORT=1162
GOLIVE_BACKUP_PASSPHRASE=${BACKUP_PASSPHRASE}
GOLIVE_BACKUP_INTERVAL=24h
GOLIVE_BACKUP_KEEP=10
GOLIVE_RETENTION_DAYS=395
GOLIVE_OIDC_ISSUER=
GOLIVE_OIDC_CLIENT_ID=
GOLIVE_OIDC_CLIENT_SECRET=
GOLIVE_OIDC_REDIRECT_URL=${MANAGEMENT_URL}/api/v1/auth/oidc/callback
EOF
chmod 600 .env

case "$TLS_MODE" in
  direct)
    cat >compose.override.yml <<'EOF'
services:
  caddy:
    ports:
      - "80:80"
EOF
    ;;
  apache)
    cat >compose.override.yml <<EOF
services:
  caddy:
    ports:
      - "127.0.0.1:${ACME_PORT}:80"
EOF
    APACHE_GENERATED="deploy/apache-golive-acme.generated.conf"
    cp deploy/apache-golive-acme.conf "$APACHE_GENERATED"
    sed -i -e "s/@GOLIVE_DOMAIN@/${DOMAIN}/g" -e "s/@ACME_PORT@/${ACME_PORT}/g" "$APACHE_GENERATED"

    if [[ -d /etc/apache2 ]]; then
      SUDO=""
      ((EUID == 0)) || SUDO="sudo"
      command -v ${SUDO:-true} >/dev/null 2>&1 || die "sudo is required to configure Apache"
      $SUDO install -m 0644 "$APACHE_GENERATED" /etc/apache2/sites-available/golive-acme.conf
      $SUDO a2enmod proxy proxy_http
      $SUDO a2ensite golive-acme.conf
      $SUDO apachectl configtest
      $SUDO systemctl reload apache2
    else
      printf 'Apache was not configured automatically. Install %s and reload Apache.\n' "$APACHE_GENERATED"
    fi
    ;;
  internal)
    cat >compose.override.yml <<'EOF'
services:
  caddy:
    volumes:
      - ./deploy/Caddyfile.internal:/etc/caddy/Caddyfile:ro
EOF
    ;;
esac

docker compose config -q

printf '\nGoLive will use:\n  hostname: %s\n  web port: %s\n  TLS mode: %s\n' "$DOMAIN" "$WEB_PORT" "$TLS_MODE"
if ((ASSUME_YES == 0)) && [[ -r /dev/tty ]]; then
  answer="$(prompt 'Build and start GoLive now?' 'yes')"
  [[ "$answer" =~ ^[Yy]([Ee][Ss])?$ ]] || die "Configuration created; start later with docker compose up -d --build --wait"
fi

docker compose up -d --build --wait
docker compose ps

cat <<EOF

GoLive NMS is installed.

Management URL: ${MANAGEMENT_URL}
Administrator:  ${ADMIN_EMAIL}
Password:       ${ADMIN_PASSWORD}

Required inbound ports: ${WEB_PORT}/tcp (management), 9443/tcp (agents)

The remaining generated secrets are stored in .env (mode 0600). Back up .env
securely. For public TLS, DNS and NAT must direct public port 80 to this host
(${TLS_MODE} mode) before certificate issuance can complete.
EOF

if [[ "$TLS_MODE" == "internal" ]]; then
  cat <<'EOF'

This installation uses Caddy's private CA. Trust its root certificate on each
browser or initial-enrollment host; see INSTALL.md for the export command.
EOF
fi
