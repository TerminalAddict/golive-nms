# GoLive API v1

All management resources are below `/api/v1`. JSON errors have the form `{"error":{"status":400,"message":"..."}}`.

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/healthz` | Container/database health |
| `GET` | `/api/v1/summary` | Current aggregate health |
| `GET`, `POST` | `/api/v1/devices` | List or create devices |
| `DELETE` | `/api/v1/devices/{id}` | Delete a device and its checks |
| `GET`, `POST` | `/api/v1/checks` | List or create ping, HTTP, TCP, SNMP, DNS, TLS, banner, database, or RouterOS checks |
| `GET` | `/api/v1/checks/{id}/history` | Availability and latency samples |
| `GET` | `/api/v1/incidents` | List recent incidents |
| `POST` | `/api/v1/incidents/{id}/acknowledge` | Acknowledge an open incident |
| `POST` | `/api/v1/incidents/{id}/assign` | Assign/unassign the authenticated operator |
| `POST` | `/api/v1/incidents/{id}/notes` | Append an operator note |
| `GET` | `/api/v1/events` | Server-sent event stream |
| `POST` | `/api/v1/agent/report` | Authenticated agent report ingestion |
| `POST` | `/collector` | Monit-compatible XML collector |
| `GET` | `/api/v1/monit-services` | Site-scoped Monit service inventory and status |
| `GET`, `POST` | `/api/v1/users` | Administrator user management |
| `GET`, `POST` | `/api/v1/api-tokens` | Service-token management |
| `GET`, `POST` | `/api/v1/credentials` | Encrypted credentials |
| `GET`, `POST` | `/api/v1/notification-channels` | Alert destinations |
| `GET`, `POST` | `/api/v1/sites` | Sites and geographic coordinates |
| `GET`, `PUT` | `/api/v1/users/{id}/sites` | Per-user site grants |
| `POST` | `/api/v1/enrollment-tokens` | Create one-time agent/collector enrollment |
| `POST` | `/api/v1/enroll` | Submit CSR and receive client/CA certificates |
| `GET`, `DELETE` | `/api/v1/identities/{id}` | List or revoke mTLS identities |
| `GET`, `POST` | `/api/v1/collector/*` | Remote collector assignments and results |
| `PATCH` | `/api/v1/devices/{id}` | Update device metadata, site, tags, and parent |
| `GET` | `/api/v1/device-events` | Search syslog and SNMP traps |
| `GET`, `POST` | `/api/v1/config-profiles` | SSH configuration backup profiles |
| `GET` | `/api/v1/config-diff` | Authorized unified configuration diff |
| `GET` | `/api/v1/agent-inventory` | Scoped agent OS, update, and performance inventory |
| `GET`, `POST` | `/api/v1/maintenance-windows` | List or schedule alert/remediation suppression |
| `GET`, `POST` | `/api/v1/action-templates` | Controlled executable allow-list |
| `GET`, `POST` | `/api/v1/remediation-jobs` | Remediation history or manual queueing |
| `GET`, `PUT` | `/api/v1/remediation-settings` | Global remediation kill switch |

Management endpoints use secure server-side sessions or `Authorization: Bearer glv_...` service tokens. The first administrator is bootstrapped from `GOLIVE_ADMIN_EMAIL` and `GOLIVE_ADMIN_PASSWORD`. Agents and Monit use their separate configured ingestion credentials.
