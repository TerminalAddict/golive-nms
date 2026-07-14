# Configuration examples

Use the web interface for normal configuration. These payloads are useful for automation with a `glv_...` API token:

```sh
BASE=https://nms.example.com/api/v1
AUTH="Authorization: Bearer glv_replace_me"
```

## Devices and relationships

Create a router, then use its returned `ID` as `ParentID` for a server or virtual host:

```sh
curl -H "$AUTH" -H 'Content-Type: application/json' "$BASE/devices" \
  -d '{"SiteID":"SITE_UUID","Name":"edge-01","Address":"10.0.0.1","Kind":"router","Tags":["core"]}'
curl -H "$AUTH" -H 'Content-Type: application/json' "$BASE/devices" \
  -d '{"SiteID":"SITE_UUID","ParentID":"ROUTER_UUID","Name":"mail-01","Address":"10.0.1.20","Kind":"server","Tags":["production","mail"]}'
```

Parent failures suppress child alerts and show descendants as dependency failures.

## Service checks

```json
{"DeviceID":"DEVICE_UUID","Name":"ICMP reachability","Type":"ping","Target":"10.0.1.20","IntervalSeconds":30,"TimeoutSeconds":3,"Enabled":true}
{"DeviceID":"DEVICE_UUID","Name":"Home Assistant","Type":"http","Target":"https://home.example.com/api/","IntervalSeconds":30,"TimeoutSeconds":5,"Enabled":true}
{"DeviceID":"DEVICE_UUID","Name":"SSH","Type":"ssh","Target":"10.0.1.20:22","IntervalSeconds":60,"TimeoutSeconds":5,"Enabled":true}
{"DeviceID":"DEVICE_UUID","Name":"MariaDB","Type":"mysql","Target":"10.0.1.20:3306","IntervalSeconds":30,"TimeoutSeconds":3,"Enabled":true}
{"DeviceID":"DEVICE_UUID","Name":"Postfix SMTP","Type":"smtp","Target":"10.0.1.20:25","IntervalSeconds":30,"TimeoutSeconds":5,"Enabled":true}
{"DeviceID":"DEVICE_UUID","Name":"Public DNS","Type":"dns","Target":"mail.example.com","IntervalSeconds":60,"TimeoutSeconds":5,"Enabled":true}
{"DeviceID":"DEVICE_UUID","Name":"Certificate","Type":"tls","Target":"mail.example.com:443","Config":{"minimumDays":21},"IntervalSeconds":3600,"TimeoutSeconds":10,"Enabled":true}
```

POST any example to `/checks`. SNMP and RouterOS checks additionally reference an encrypted `CredentialID`; create these from Settings so secrets never appear in check definitions.

## Monit

```monit
set eventqueue basedir /var/lib/monit/events slots 1000
set mmonit https://monit:REPLACE_PASSWORD@nms.example.com:9443/collector
```

Optional remote control (also restrict TCP `2812` to the NMS source address in the host firewall):

```monit
set httpd port 2812 and
    allow golive:REPLACE_WITH_A_DIFFERENT_LONG_PASSWORD
    allow localhost
```

Create a Monit credential in GoLive, then set the device's remote-control URL to the private/VPN address, for example `http://10.0.0.12:2812`. Prefer Monit HTTPS when traffic cannot stay on a trusted private network.

## Site maintenance

Times use RFC 3339 UTC values:

```json
{"Name":"Datacentre power work","SiteID":"SITE_UUID","DeviceID":"","StartsAt":"2026-08-01T09:00:00Z","EndsAt":"2026-08-01T11:00:00Z"}
```

POST to `/maintenance-windows`. Checks and samples continue; incidents, alerts, and automatic remediation are suppressed.

## Safe remediation

An administrator first creates a fixed executable template:

```json
{"name":"Restart Postfix","executable":"/usr/bin/systemctl","arguments":["restart","postfix"],"timeoutSeconds":30,"autoCheckType":"smtp"}
```

The agent executes the binary directly without a shell after verifying the server signature. Shells and general interpreters are rejected, automatic actions have cooldowns, and Settings includes a global kill switch.

## Alert routing

Alert channels can apply globally or to one site, send failures and/or recoveries, and repeat while an incident remains active. A repeat value of `0` disables reminders. SMTP passwords and webhook URLs are stored in encrypted credentials.
