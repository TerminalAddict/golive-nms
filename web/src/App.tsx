import { useCallback, useEffect, useState } from "react";
import {
  Activity,
  AlertTriangle,
  Bell,
  Boxes,
  LayoutDashboard,
  Map,
  Menu,
  Plus,
  Radio,
  Server,
  Settings,
  ScrollText,
  FileDiff,
  ShieldCheck,
} from "lucide-react";
import { api } from "./api";
import type {
  APIToken,
  CheckSample,
  Credential,
  NotificationChannel,
  Site,
  Identity,
  DeviceEvent,
  ConfigProfile,
  ConfigSnapshot,
  ActionTemplate,
  RemediationJob,
  User,
} from "./api";
import type { Check, Device, Incident, MonitService, Summary } from "./types";
import { CircleMarker, MapContainer, Popup, TileLayer } from "react-leaflet";

type View =
  | "overview"
  | "devices"
  | "incidents"
  | "events"
  | "configuration"
  | "remediation"
  | "topology"
  | "settings";
const emptySummary: Summary = {
  Total: 0,
  Up: 0,
  Down: 0,
  Degraded: 0,
  Unknown: 0,
  OpenIncidents: 0,
};

export function App() {
  const [user, setUser] = useState<User | null>(null);
  const [checking, setChecking] = useState(true);
  useEffect(() => {
    api
      .me()
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setChecking(false));
  }, []);
  if (checking)
    return (
      <div className="login">
        <Brand />
        <p>Loading network operations…</p>
      </div>
    );
  if (!user) return <Login onLogin={setUser} />;
  return <Console user={user} />;
}
function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [email, setEmail] = useState("admin@example.com");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [oidc, setOIDC] = useState(false);
  useEffect(() => {
    api.authConfig().then((v) => setOIDC(v.oidcEnabled));
  }, []);
  return (
    <div className="login">
      <div className="loginCard card">
        <Brand />
        <h1>Welcome back</h1>
        <p>Sign in to manage your network.</p>
        {error && (
          <div className="error">
            <AlertTriangle />
            {error}
          </div>
        )}
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            try {
              onLogin(await api.login(email, password));
            } catch (x) {
              setError(x instanceof Error ? x.message : "Login failed");
            }
          }}
        >
          <label>
            Email
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </label>
          <label>
            Password
            <input
              type="password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </label>
          <button className="primary">Sign in</button>
        </form>
        {oidc && (
          <>
            <div className="loginDivider">or</div>
            <a className="ssoButton" href="/api/v1/auth/oidc/start">
              Sign in with SSO
            </a>
          </>
        )}
      </div>
    </div>
  );
}
function Console({ user }: { user: User }) {
  const [view, setView] = useState<View>("overview");
  const [summary, setSummary] = useState(emptySummary);
  const [devices, setDevices] = useState<Device[]>([]);
  const [checks, setChecks] = useState<Check[]>([]);
  const [monitServices, setMonitServices] = useState<MonitService[]>([]);
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [deviceEvents, setDeviceEvents] = useState<DeviceEvent[]>([]);
  const [modal, setModal] = useState<"device" | "check" | null>(null);
  const [editingDevice, setEditingDevice] = useState<Device | null>(null);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    try {
      const [s, d, c, ms, i, ev] = await Promise.all([
        api.summary(),
        api.devices(),
        api.checks(),
        api.monitServices(),
        api.incidents(),
        api.deviceEvents(),
      ]);
      setSummary(s);
      setDevices(d);
      setChecks(c);
      setMonitServices(ms);
      setIncidents(i);
      setDeviceEvents(ev);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to load data");
    }
  }, []);
  useEffect(() => {
    load();
    const events = new EventSource("/api/v1/events");
    events.onmessage = () => load();
    const timer = setInterval(load, 30000);
    return () => {
      events.close();
      clearInterval(timer);
    };
  }, [load]);
  return (
    <div className="shell">
      <aside>
        <Brand />
        <nav>
          <Nav
            icon={<LayoutDashboard />}
            label="Overview"
            active={view === "overview"}
            onClick={() => setView("overview")}
          />
          <Nav
            icon={<Server />}
            label="Devices"
            count={summary.Total}
            active={view === "devices"}
            onClick={() => setView("devices")}
          />
          <Nav
            icon={<Bell />}
            label="Incidents"
            count={summary.OpenIncidents}
            danger
            active={view === "incidents"}
            onClick={() => setView("incidents")}
          />
          <Nav
            icon={<Map />}
            label="Topology"
            active={view === "topology"}
            onClick={() => setView("topology")}
          />
          <Nav
            icon={<ScrollText />}
            label="Events"
            active={view === "events"}
            onClick={() => setView("events")}
          />
          <Nav
            icon={<FileDiff />}
            label="Configuration"
            active={view === "configuration"}
            onClick={() => setView("configuration")}
          />
          <Nav
            icon={<ShieldCheck />}
            label="Remediation"
            active={view === "remediation"}
            onClick={() => setView("remediation")}
          />
        </nav>
        <div className="asideBottom">
          <button onClick={() => setView("settings")}>
            <Settings />
            Settings
          </button>
          <div className="user">
            <span>GN</span>
            <div>
              <b>{user.displayName}</b>
              <small>{user.role.replace("_", " ")}</small>
            </div>
          </div>
        </div>
      </aside>
      <main>
        <header>
          <button className="mobile">
            <Menu />
          </button>
          <div>
            <small>NETWORK OPERATIONS</small>
            <h1>{view[0].toUpperCase() + view.slice(1)}</h1>
          </div>
          <div className="live">
            <i /> Live
          </div>
          <button className="primary" onClick={() => setModal("device")}>
            <Plus /> Add device
          </button>
        </header>
        {error && (
          <div className="error">
            <AlertTriangle />
            {error}
          </div>
        )}
        {view === "overview" && (
          <Overview
            summary={summary}
            devices={devices}
            checks={checks}
            incidents={incidents}
          />
        )}{" "}
        {view === "devices" && (
          <Devices
            devices={devices}
            checks={checks}
            monitServices={monitServices}
            canManage={user.role === "administrator" || user.role === "manager" || user.role === "site_manager"}
            onAddCheck={() => setModal("check")}
            onEdit={setEditingDevice}
          />
        )}{" "}
        {view === "incidents" && (
          <Incidents
            incidents={incidents}
            user={user}
            refresh={load}
            onAck={async (id) => {
              await api.acknowledge(id);
              load();
            }}
          />
        )}{" "}
        {view === "topology" && <Topology devices={devices} />}
        {view === "events" && <EventLog initial={deviceEvents} />}
        {view === "configuration" && (
          <Configuration
            devices={devices}
            canManage={user.role === "administrator" || user.role === "manager"}
          />
        )}
        {view === "remediation" && (
          <Remediation devices={devices} user={user} />
        )}
        {view === "settings" && <IdentitySettings current={user} />}
      </main>
      {modal === "device" && (
        <DeviceModal
          devices={devices}
          monitServices={monitServices}
          onClose={() => setModal(null)}
          onSaved={() => {
            setModal(null);
            load();
          }}
        />
      )}
      {editingDevice && (
        <DeviceModal
          device={editingDevice}
          devices={devices}
          monitServices={monitServices}
          onClose={() => setEditingDevice(null)}
          onSaved={() => {
            setEditingDevice(null);
            load();
          }}
        />
      )}
      {modal === "check" && (
        <CheckModal
          devices={devices}
          onClose={() => setModal(null)}
          onSaved={() => {
            setModal(null);
            load();
          }}
        />
      )}
    </div>
  );
}

function Brand() {
  return (
    <div className="brand">
      <div className="mark">
        <Activity />
      </div>
      <div>
        <b>GoLive</b>
        <small>NETWORK MANAGEMENT</small>
      </div>
    </div>
  );
}
function Nav(p: {
  icon: React.ReactNode;
  label: string;
  count?: number;
  danger?: boolean;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button className={p.active ? "active" : ""} onClick={p.onClick}>
      {p.icon}
      <span>{p.label}</span>
      {p.count !== undefined && (
        <em className={p.danger && p.count ? "danger" : ""}>{p.count}</em>
      )}
    </button>
  );
}
function HealthRing({ s }: { s: Summary }) {
  const good = s.Total === 0 ? 100 : Math.round((s.Up / s.Total) * 100);
  const color = s.Down
    ? "var(--red)"
    : s.Degraded
      ? "var(--amber)"
      : "var(--green)";
  return (
    <div
      className="ring"
      style={{ background: `conic-gradient(${color} ${good}%,var(--line) 0)` }}
    >
      <div>
        <strong>{good}%</strong>
        <span>HEALTHY</span>
      </div>
    </div>
  );
}
function Overview({
  summary: s,
  devices,
  checks,
  incidents,
}: {
  summary: Summary;
  devices: Device[];
  checks: Check[];
  incidents: Incident[];
}) {
  type Widget = "summary" | "stats" | "performance" | "devices" | "incidents";
  const [widgets, setWidgets] = useState<Widget[]>(() => {
    try { const value = JSON.parse(localStorage.getItem("golive-dashboard-widgets") ?? "null"); if (Array.isArray(value)) return value; } catch { /* use defaults */ }
    return ["summary", "stats", "performance", "devices", "incidents"];
  });
  const toggleWidget = (widget: Widget) => setWidgets((current) => {
    const next = current.includes(widget) ? current.filter((x) => x !== widget) : [...current, widget];
    localStorage.setItem("golive-dashboard-widgets", JSON.stringify(next)); return next;
  });
  return (
    <section className="content">
      <div className="toolbar"><p>Dashboard widgets</p><div className="rowActions">{(["summary", "stats", "performance", "devices", "incidents"] as Widget[]).map((widget) => <button key={widget} className={widgets.includes(widget) ? "secondary active" : "secondary"} onClick={() => toggleWidget(widget)}>{widget}</button>)}</div></div>
      {widgets.includes("summary") && <div className="hero card">
        <div>
          <p className="eyebrow">
            <Radio /> SYSTEM STATUS
          </p>
          <h2>
            {s.Down
              ? "Attention required"
              : s.Total
                ? "All systems operational"
                : "Ready to monitor"}
          </h2>
          <p>
            {s.Total
              ? `${s.Up} of ${s.Total} devices are responding normally.`
              : "Add your first device and service check to begin."}
          </p>
          <div className="legend">
            <span>
              <i className="up" />
              {s.Up} Up
            </span>
            <span>
              <i className="down" />
              {s.Down} Down
            </span>
            <span>
              <i className="unknown" />
              {s.Unknown} Unknown
            </span>
          </div>
        </div>
        <HealthRing s={s} />
      </div>}
      {widgets.includes("stats") && <div className="stats">
        <Stat label="Monitored devices" value={s.Total} icon={<Server />} />
        <Stat label="Active checks" value={checks.length} icon={<Activity />} />
        <Stat
          label="Open incidents"
          value={s.OpenIncidents}
          icon={<Bell />}
          bad={s.OpenIncidents > 0}
        />
        <Stat
          label="Sites online"
          value={new Set(devices.map((d) => d.SiteName)).size}
          icon={<Boxes />}
        />
      </div>}
      {widgets.includes("performance") && <PerformanceChart checks={checks} />}
      <div className="grid">
        {widgets.includes("devices") && <div className="card panel">
          <Title text="Device health" />
          <DeviceRows devices={devices.slice(0, 6)} />
        </div>}
        {widgets.includes("incidents") && <div className="card panel">
          <Title text="Recent incidents" />
          <IncidentRows incidents={incidents.slice(0, 5)} />
        </div>}
      </div>
    </section>
  );
}
function PerformanceChart({ checks }: { checks: Check[] }) {
  const [check, setCheck] = useState(checks[0]?.ID ?? "");
  const [samples, setSamples] = useState<CheckSample[]>([]);
  useEffect(() => {
    if (!check && checks.length) setCheck(checks[0].ID);
  }, [check, checks]);
  useEffect(() => {
    if (check) api.history(check).then(setSamples);
  }, [check]);
  const points = samples.slice(-120);
  const max = Math.max(1, ...points.map((x) => x.latencyMs));
  const coords = points
    .map(
      (x, i) =>
        `${points.length < 2 ? 0 : (i / (points.length - 1)) * 100},${100 - (x.latencyMs / max) * 88}`,
    )
    .join(" ");
  const uptime = points.length
    ? Math.round((points.filter((x) => x.up).length / points.length) * 10000) /
      100
    : 100;
  return (
    <div className="card performance">
      <div className="title">
        <h3>Availability and latency</h3>
        <select value={check} onChange={(e) => setCheck(e.target.value)}>
          {checks.map((c) => (
            <option key={c.ID} value={c.ID}>
              {c.DeviceName} · {c.Name}
            </option>
          ))}
        </select>
      </div>
      <div className="chartBody">
        <div className="chartMetric">
          <strong>{uptime}%</strong>
          <small>sample uptime</small>
        </div>
        {points.length ? (
          <svg
            viewBox="0 0 100 100"
            preserveAspectRatio="none"
            role="img"
            aria-label="Latency line chart"
          >
            <defs>
              <linearGradient id="latencyFill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0" stopColor="var(--green)" stopOpacity=".35" />
                <stop offset="1" stopColor="var(--green)" stopOpacity="0" />
              </linearGradient>
            </defs>
            <polygon
              points={`0,100 ${coords} 100,100`}
              fill="url(#latencyFill)"
            />
            <polyline
              points={coords}
              fill="none"
              stroke="var(--green)"
              strokeWidth="1.5"
              vectorEffect="non-scaling-stroke"
            />
          </svg>
        ) : (
          <Empty text="Latency samples will appear after checks run" />
        )}
        <div className="chartMetric right">
          <strong>
            {points.length
              ? points[points.length - 1].latencyMs.toFixed(1)
              : "—"}{" "}
            ms
          </strong>
          <small>latest latency</small>
        </div>
      </div>
    </div>
  );
}
function Stat({
  label,
  value,
  icon,
  bad,
}: {
  label: string;
  value: number;
  icon: React.ReactNode;
  bad?: boolean;
}) {
  return (
    <div className="stat card">
      <div className={bad ? "bad icon" : "icon"}>{icon}</div>
      <div>
        <strong>{value}</strong>
        <span>{label}</span>
      </div>
    </div>
  );
}
function Title({ text }: { text: string }) {
  return (
    <div className="title">
      <h3>{text}</h3>
      <small>Live status</small>
    </div>
  );
}
function Status({ value }: { value: string }) {
  return (
    <span className={`status ${value}`}>
      <i />
      {value}
    </span>
  );
}
function DeviceRows({ devices }: { devices: Device[] }) {
  return devices.length ? (
    <div className="rows">
      {devices.map((d) => (
        <div className="row" key={d.ID}>
          <div className="deviceIcon">
            <Server />
          </div>
          <div className="grow">
            <b>{d.Name}</b>
            <small>
              {d.Address} · {d.SiteName || "Unassigned"}
            </small>
          </div>
          <Status value={d.Status} />
        </div>
      ))}
    </div>
  ) : (
    <Empty text="No devices yet" />
  );
}
function IncidentRows({ incidents }: { incidents: Incident[] }) {
  return incidents.length ? (
    <div className="rows">
      {incidents.map((i) => (
        <div className="row" key={i.ID}>
          <div className="deviceIcon alert">
            <AlertTriangle />
          </div>
          <div className="grow">
            <b>{i.Title}</b>
            <small>
              {i.DeviceName} · {new Date(i.OpenedAt).toLocaleString()}
            </small>
          </div>
          <Status value={i.State} />
        </div>
      ))}
    </div>
  ) : (
    <Empty text="No incidents — looking good" />
  );
}
function Empty({ text }: { text: string }) {
  return (
    <div className="empty">
      <Activity />
      <p>{text}</p>
    </div>
  );
}
function Devices({
  devices,
  checks,
  monitServices,
  canManage,
  onAddCheck,
  onEdit,
}: {
  devices: Device[];
  checks: Check[];
  monitServices: MonitService[];
  canManage: boolean;
  onAddCheck: () => void;
  onEdit: (device: Device) => void;
}) {
  return (
    <section className="content">
      <div className="toolbar">
        <p>{devices.length} managed devices</p>
        <button className="secondary" onClick={onAddCheck}>
          <Plus /> Add check
        </button>
      </div>
      <div className="card table">
        <div className="thead">
          <span>Device</span>
          <span>Type</span>
          <span>Checks</span>
          <span>Status</span>
        </div>
        {devices.map((d) => (
          <div className="trow" key={d.ID}>
            <div>
              {canManage ? (
                <button className="deviceLink" onClick={() => onEdit(d)}>{d.Name}</button>
              ) : (
                <b>{d.Name}</b>
              )}
              <small>{d.Address}</small>
            </div>
            <span>{d.Kind}</span>
            <span className="serviceCount">
              {checks.filter((c) => c.DeviceID === d.ID).length} checks
              <small>{monitServices.filter((s) => s.DeviceID === d.ID).length} Monit</small>
            </span>
            <Status value={d.Status} />
          </div>
        ))}
        {!devices.length && <Empty text="Add a device to start monitoring" />}
      </div>
    </section>
  );
}
function Incidents({
  incidents,
  onAck,
  user,
  refresh,
}: {
  incidents: Incident[];
  onAck: (id: string) => void;
  user: User;
  refresh: () => void;
}) {
  return (
    <section className="content">
      <div className="card table">
        <div className="thead incident">
          <span>Incident</span>
          <span>Device</span>
          <span>Opened</span>
          <span>State</span>
          <span />
        </div>
        {incidents.map((i) => (
          <div className="trow incident" key={i.ID}>
            <b>{i.Title}</b>
            <span title={i.Notes}>{i.DeviceName}{i.AssignedName ? ` · ${i.AssignedName}` : ""}</span>
            <span>{new Date(i.OpenedAt).toLocaleString()}</span>
            <Status value={i.State} />
            {i.State !== "resolved" && user.role !== "viewer" ? (
              <div className="rowActions">
                {i.State === "open" && <button className="secondary" onClick={() => onAck(i.ID)}>Acknowledge</button>}
                <button className="secondary" onClick={async () => { await api.assignIncident(i.ID, i.AssignedTo !== user.id); refresh(); }}>{i.AssignedTo === user.id ? "Unassign" : i.AssignedTo ? "Take over" : "Assign to me"}</button>
                <button className="secondary" onClick={async () => { const note = window.prompt("Operator note"); if (note) { await api.noteIncident(i.ID, note); refresh(); } }}>Add note</button>
              </div>
            ) : (
              <span />
            )}
          </div>
        ))}
        {!incidents.length && <Empty text="No incidents recorded" />}
      </div>
    </section>
  );
}
function Remediation({ devices, user }: { devices: Device[]; user: User }) {
  const [templates, setTemplates] = useState<ActionTemplate[]>([]);
  const [jobs, setJobs] = useState<RemediationJob[]>([]);
  const [enabled, setEnabled] = useState(true);
  const refresh = useCallback(async () => {
    const [t, j, s] = await Promise.all([
      api.actionTemplates(),
      api.remediationJobs(),
      api.remediationSettings(),
    ]);
    setTemplates(t);
    setJobs(j);
    setEnabled(s.enabled);
  }, []);
  useEffect(() => {
    refresh();
    const timer = setInterval(refresh, 10000);
    return () => clearInterval(timer);
  }, [refresh]);
  return (
    <section className="content remediationGrid">
      <div className="card panel">
        <div className="title">
          <h3>Allowlisted actions</h3>
          <span className={`status ${enabled ? "up" : "down"}`}>
            <i />
            {enabled ? "enabled" : "disabled"}
          </span>
          {user.role === "administrator" && (
            <button
              className="secondary"
              onClick={async () => {
                await api.setRemediation(!enabled);
                refresh();
              }}
            >
              {enabled ? "Kill switch" : "Enable"}
            </button>
          )}
        </div>
        <div className="rows">
          {templates
            .filter((t) => t.enabled)
            .map((t) => (
              <div className="row" key={t.id}>
                <div className="grow">
                  <b>{t.name}</b>
                  <small>
                    {t.executable} {t.arguments.join(" ")} · {t.timeoutSeconds}s
                    {t.autoCheckType
                      ? ` · automatic on ${t.autoCheckType} failure`
                      : " · manual only"}
                  </small>
                </div>
              </div>
            ))}
        </div>
        {user.role === "administrator" && (
          <form
            className="settingsForm"
            onSubmit={async (e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              await api.createActionTemplate({
                name: String(f.get("name")),
                executable: String(f.get("executable")),
                arguments: String(f.get("arguments"))
                  .split("\n")
                  .map((x) => x.trim())
                  .filter(Boolean),
                timeoutSeconds: Number(f.get("timeout")),
                autoCheckType: String(f.get("autoCheckType")),
              });
              e.currentTarget.reset();
              refresh();
            }}
          >
            <h3>Create signed action template</h3>
            <input required name="name" placeholder="Restart web service" />
            <input
              required
              name="executable"
              placeholder="/usr/bin/systemctl"
            />
            <textarea
              name="arguments"
              placeholder={"One argument per line\nrestart\nnginx.service"}
            />
            <input
              required
              name="timeout"
              type="number"
              min="1"
              max="300"
              defaultValue="30"
            />
            <select name="autoCheckType">
              <option value="">Manual confirmation only</option>
              <option value="http">Automatic on HTTP failure</option>
              <option value="tcp">Automatic on TCP failure</option>
              <option value="ping">Automatic on ping failure</option>
              <option value="ssh">Automatic on SSH failure</option>
            </select>
            <button className="primary">Create allowlisted template</button>
          </form>
        )}
      </div>
      <div className="card panel">
        <Title text="Run and audit" />
        {user.role !== "viewer" && (
          <form
            className="inlineAction"
            onSubmit={async (e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              await api.queueRemediation(
                String(f.get("templateId")),
                String(f.get("deviceId")),
              );
              refresh();
            }}
          >
            <select required name="templateId">
              <option value="">Choose action</option>
              {templates
                .filter((t) => t.enabled)
                .map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.name}
                  </option>
                ))}
            </select>
            <select required name="deviceId">
              <option value="">Choose agent host</option>
              {devices.map((d) => (
                <option key={d.ID} value={d.ID}>
                  {d.Name}
                </option>
              ))}
            </select>
            <button className="primary" disabled={!enabled}>
              Queue action
            </button>
          </form>
        )}
        <div className="rows jobRows">
          {jobs.map((j) => (
            <div className="row" key={j.id}>
              <div className="grow">
                <b>
                  {j.templateName} · {j.deviceName}
                </b>
                <small>
                  {new Date(j.queuedAt).toLocaleString()} ·{" "}
                  {j.automatic ? "automatic" : "manual"}
                  {j.error ? ` · ${j.error}` : ""}
                </small>
                {j.output && <pre>{j.output}</pre>}
              </div>
              <Status value={j.state} />
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function Configuration({
  devices,
  canManage,
}: {
  devices: Device[];
  canManage: boolean;
}) {
  const [profiles, setProfiles] = useState<ConfigProfile[]>([]);
  const [selected, setSelected] = useState("");
  const [snapshots, setSnapshots] = useState<ConfigSnapshot[]>([]);
  const [diff, setDiff] = useState("");
  const refresh = useCallback(async () => {
    const p = await api.configProfiles();
    setProfiles(p);
    if (!selected && p.length) setSelected(p[0].deviceId);
  }, [selected]);
  useEffect(() => {
    refresh();
  }, [refresh]);
  useEffect(() => {
    if (selected) api.configSnapshots(selected).then(setSnapshots);
  }, [selected]);
  const compare = async () => {
    if (snapshots.length >= 2)
      setDiff((await api.configDiff(snapshots[1].id, snapshots[0].id)).diff);
  };
  return (
    <section className="content configGrid">
      <div className="card panel">
        <Title text="Configuration backups" />
        <div className="rows">
          {profiles.map((p) => (
            <div
              className={`row ${selected === p.deviceId ? "selected" : ""}`}
              key={p.id}
              onClick={() => setSelected(p.deviceId)}
            >
              <div className="grow">
                <b>{p.deviceName}</b>
                <small>
                  {p.lastRunAt
                    ? `Last backup ${new Date(p.lastRunAt).toLocaleString()}`
                    : "Waiting for first backup"}
                  {p.lastError ? ` · ${p.lastError}` : ""}
                </small>
              </div>
              {canManage && (
                <button
                  className="secondary"
                  onClick={async (e) => {
                    e.stopPropagation();
                    await api.triggerConfig(p.id);
                    setTimeout(
                      () => api.configSnapshots(p.deviceId).then(setSnapshots),
                      6000,
                    );
                  }}
                >
                  Back up now
                </button>
              )}
            </div>
          ))}
        </div>
        {canManage && (
          <form
            className="settingsForm"
            onSubmit={async (e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              const credential = await api.createCredential({
                name: `${f.get("deviceId")} SSH`,
                kind: "ssh",
                secret: {
                  username: String(f.get("username")),
                  password: String(f.get("password")),
                  privateKey: String(f.get("privateKey")),
                  hostKeySHA256: String(f.get("hostKey")),
                },
              });
              await api.createConfigProfile({
                deviceId: String(f.get("deviceId")),
                credentialId: credential.id,
                command: String(f.get("command")),
                intervalSeconds: 86400,
              });
              e.currentTarget.reset();
              refresh();
            }}
          >
            <h3>Add SSH backup</h3>
            <select required name="deviceId">
              <option value="">Choose device</option>
              {devices.map((d) => (
                <option key={d.ID} value={d.ID}>
                  {d.Name}
                </option>
              ))}
            </select>
            <input required name="username" placeholder="SSH username" />
            <input
              name="password"
              type="password"
              placeholder="SSH password (or private key below)"
            />
            <textarea name="privateKey" placeholder="PEM private key" />
            <input
              required
              name="hostKey"
              placeholder="SHA256:... host key fingerprint"
            />
            <input
              required
              name="command"
              defaultValue="/export hide-sensitive"
              placeholder="Read-only backup command"
            />
            <button className="primary">Create encrypted backup profile</button>
          </form>
        )}
      </div>
      <div className="card panel">
        <div className="title">
          <h3>Version history</h3>
          <button
            className="secondary"
            disabled={snapshots.length < 2}
            onClick={compare}
          >
            Compare latest
          </button>
        </div>
        <div className="rows">
          {snapshots.map((s) => (
            <div className="row" key={s.id}>
              <div className="grow">
                <b>{new Date(s.capturedAt).toLocaleString()}</b>
                <small>{s.contentHash.slice(0, 16)}…</small>
              </div>
            </div>
          ))}
        </div>
        {diff ? (
          <pre className="configDiff">{diff}</pre>
        ) : (
          <Empty
            text={
              snapshots.length < 2
                ? "Two versions are needed for a diff"
                : "Select Compare latest to view changes"
            }
          />
        )}
      </div>
    </section>
  );
}

function EventLog({ initial }: { initial: DeviceEvent[] }) {
  const [events, setEvents] = useState(initial);
  const [protocol, setProtocol] = useState("");
  const [query, setQuery] = useState("");
  useEffect(() => setEvents(initial), [initial]);
  const search = async () => setEvents(await api.deviceEvents(protocol, query));
  return (
    <section className="content">
      <div className="toolbar eventFilters">
        <select value={protocol} onChange={(e) => setProtocol(e.target.value)}>
          <option value="">All protocols</option>
          <option value="syslog">Syslog</option>
          <option value="snmp_trap">SNMP traps</option>
        </select>
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && search()}
          placeholder="Search source or message"
        />
        <button className="secondary" onClick={search}>
          Search
        </button>
      </div>
      <div className="card table">
        <div className="thead eventRow">
          <span>Received</span>
          <span>Protocol</span>
          <span>Source</span>
          <span>Severity</span>
          <span>Message</span>
        </div>
        {events.map((e) => (
          <div className="trow eventRow" key={e.id}>
            <span>{new Date(e.receivedAt).toLocaleString()}</span>
            <b>{e.protocol.replace("_", " ")}</b>
            <span>{e.source}</span>
            <span>{e.severity == null ? "—" : e.severity}</span>
            <span title={e.message}>{e.message}</span>
          </div>
        ))}
        {!events.length && <Empty text="No matching network events" />}
      </div>
    </section>
  );
}

function Topology({ devices }: { devices: Device[] }) {
  const [mode, setMode] = useState<"topology" | "geographic">("topology");
  const [sites, setSites] = useState<Site[]>([]);
  useEffect(() => {
    api.sites().then(setSites);
  }, []);
  const located = sites.filter(
    (s) => s.latitude != null && s.longitude != null,
  );
  const position = (i: number) => ({
    x: 12 + (i % 4) * 23 + 6,
    y: 18 + Math.floor(i / 4) * 30 + 6,
  });
  return (
    <section className="content">
      <div className="mapTabs">
        <button
          className={mode === "topology" ? "active" : ""}
          onClick={() => setMode("topology")}
        >
          Network topology
        </button>
        <button
          className={mode === "geographic" ? "active" : ""}
          onClick={() => setMode("geographic")}
        >
          Geographic sites
        </button>
      </div>
      {mode === "topology" ? (
        <div className="card topology">
          <svg
            className="topologyLinks"
            viewBox="0 0 100 100"
            preserveAspectRatio="none"
          >
            {devices.map((d, i) => {
              const parentIndex = devices.findIndex((p) => p.ID === d.ParentID);
              if (parentIndex < 0) return null;
              const a = position(parentIndex),
                b = position(i);
              return (
                <line
                  key={d.ID}
                  x1={a.x}
                  y1={a.y}
                  x2={b.x}
                  y2={b.y}
                  className={d.Status}
                />
              );
            })}
          </svg>
          {devices.length ? (
            devices.map((d, i) => (
              <div
                key={d.ID}
                className={`node ${d.Status}`}
                style={{
                  left: `${12 + (i % 4) * 23}%`,
                  top: `${18 + Math.floor(i / 4) * 30}%`,
                }}
              >
                <Server />
                <b>{d.Name}</b>
                <small>{d.Address}</small>
              </div>
            ))
          ) : (
            <Empty text="Topology appears as devices are added" />
          )}
        </div>
      ) : (
        <div className="card geoMap">
          {located.length ? (
            <MapContainer
              center={[located[0].latitude!, located[0].longitude!]}
              zoom={located.length === 1 ? 8 : 3}
              scrollWheelZoom
            >
              <TileLayer
                attribution="&copy; OpenStreetMap contributors"
                url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
              />
              {located.map((site) => {
                const siteDevices = devices.filter((d) => d.SiteID === site.id);
                const bad = siteDevices.some((d) => d.Status === "down");
                return (
                  <CircleMarker
                    key={site.id}
                    center={[site.latitude!, site.longitude!]}
                    radius={12}
                    pathOptions={{
                      color: bad ? "#ff5c68" : "#30d88b",
                      fillColor: bad ? "#ff5c68" : "#30d88b",
                      fillOpacity: 0.75,
                    }}
                  >
                    <Popup>
                      <b>{site.name}</b>
                      <br />
                      {siteDevices.length} devices ·{" "}
                      {bad ? "attention required" : "operational"}
                    </Popup>
                  </CircleMarker>
                );
              })}
            </MapContainer>
          ) : (
            <Empty text="Add coordinates to a site to show the geographic map" />
          )}
        </div>
      )}
    </section>
  );
}
function UserSiteEditor({ user, sites }: { user: User; sites: Site[] }) {
  const [selected, setSelected] = useState<string[]>([]);
  useEffect(() => {
    api.userSites(user.id).then((v) => setSelected(v.siteIds));
  }, [user.id]);
  return (
    <div className="siteGrant">
      <select
        multiple
        aria-label={`Sites for ${user.displayName}`}
        value={selected}
        onChange={(e) =>
          setSelected(
            Array.from(e.currentTarget.selectedOptions, (o) => o.value),
          )
        }
      >
        {sites.map((s) => (
          <option key={s.id} value={s.id}>
            {s.name}
          </option>
        ))}
      </select>
      <button
        className="secondary"
        onClick={() => api.setUserSites(user.id, selected)}
      >
        Save sites
      </button>
    </div>
  );
}

function IdentitySettings({ current }: { current: User }) {
  const [users, setUsers] = useState<User[]>([]);
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [sites, setSites] = useState<Site[]>([]);
  const [identities, setIdentities] = useState<Identity[]>([]);
  const [agentInventory, setAgentInventory] = useState<import("./api").AgentInventory[]>([]);
  const [maintenance, setMaintenance] = useState<import("./api").MaintenanceWindow[]>([]);
  const [enrollment, setEnrollment] = useState("");
  const [channelKind, setChannelKind] = useState<"email" | "slack" | "teams">(
    "email",
  );
  const [createdToken, setCreatedToken] = useState("");
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("viewer");
  const refresh = useCallback(async () => {
    setTokens(await api.tokens());
    setCredentials(await api.credentials());
    setChannels(await api.channels());
    setSites(await api.sites());
    setAgentInventory(await api.agentInventory());
    setMaintenance(await api.maintenanceWindows());
    if (current.role === "administrator" || current.role === "manager")
      setIdentities(await api.identities());
    if (current.role === "administrator") setUsers(await api.users());
  }, [current.role]);
  useEffect(() => {
    refresh();
  }, [refresh]);
  return (
    <section className="content identityGrid">
      {current.role === "administrator" && (
        <div className="card panel">
          <Title text="Users" />
          <div className="rows">
            {users.map((u) => (
              <div className="row" key={u.id}>
                <div className="grow">
                  <b>{u.displayName}</b>
                  <small>
                    {u.email} · {u.role.replace("_", " ")}
                  </small>
                </div>
                {(u.role === "site_manager" || u.role === "viewer") && (
                  <UserSiteEditor user={u} sites={sites} />
                )}
                <select
                  className="roleSelect"
                  value={u.role}
                  onChange={async (e) => {
                    await api.updateUser(u.id, {
                      displayName: u.displayName,
                      role: e.target.value,
                    });
                    refresh();
                  }}
                >
                  <option value="viewer">Viewer</option>
                  <option value="site_manager">Site manager</option>
                  <option value="manager">Manager</option>
                  <option value="administrator">Administrator</option>
                </select>
                {u.id !== current.id && (
                  <button
                    className="secondary"
                    onClick={async () => {
                      await api.deleteUser(u.id);
                      refresh();
                    }}
                  >
                    Disable
                  </button>
                )}
              </div>
            ))}
          </div>
          <form
            className="settingsForm"
            onSubmit={async (e) => {
              e.preventDefault();
              await api.createUser({
                email,
                displayName: name,
                password,
                role,
              });
              setEmail("");
              setName("");
              setPassword("");
              refresh();
            }}
          >
            <h3>Add user</h3>
            <input
              required
              type="email"
              placeholder="Email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <input
              required
              placeholder="Display name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <input
              required
              minLength={12}
              type="password"
              placeholder="Temporary password (12+ characters)"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <select value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="viewer">Viewer</option>
              <option value="site_manager">Site manager</option>
              <option value="manager">Manager</option>
              <option value="administrator">Administrator</option>
            </select>
            <button className="primary">Create user</button>
          </form>
        </div>
      )}
      <div className="card panel">
        <Title text="API tokens" />
        {createdToken && (
          <div className="tokenReveal">
            <b>Copy this token now</b>
            <code>{createdToken}</code>
          </div>
        )}
        <div className="rows">
          {tokens.map((t) => (
            <div className="row" key={t.id}>
              <div className="grow">
                <b>{t.name}</b>
                <small>
                  Created {new Date(t.createdAt).toLocaleDateString()}
                  {t.lastUsedAt
                    ? ` · Used ${new Date(t.lastUsedAt).toLocaleString()}`
                    : " · Never used"}
                </small>
              </div>
              <button
                className="secondary"
                onClick={async () => {
                  await api.deleteToken(t.id);
                  refresh();
                }}
              >
                Revoke
              </button>
            </div>
          ))}
        </div>
        <form
          className="settingsForm"
          onSubmit={async (e) => {
            e.preventDefault();
            const field = new FormData(e.currentTarget).get("name") as string;
            const token = await api.createToken(field);
            setCreatedToken(token.token ?? "");
            e.currentTarget.reset();
            refresh();
          }}
        >
          <h3>Create service token</h3>
          <input name="name" required placeholder="Automation name" />
          <button className="primary">Create token</button>
        </form>
      </div>
      {(current.role === "administrator" || current.role === "manager") && (
        <div className="card panel">
          <Title text="Network credentials" />
          <div className="rows">
            {credentials
              .filter((c) => c.kind === "snmp" || c.kind === "routeros")
              .map((c) => (
                <div className="row" key={c.id}>
                  <div className="grow">
                    <b>{c.name}</b>
                    <small>Encrypted {c.kind} credential</small>
                  </div>
                  <button
                    className="secondary"
                    onClick={async () => {
                      await api.deleteCredential(c.id);
                      refresh();
                    }}
                  >
                    Delete
                  </button>
                </div>
              ))}
          </div>
          <form
            className="settingsForm"
            onSubmit={async (e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              const version = String(f.get("version"));
              const secret: Record<string, string> = { version };
              if (version === "2c")
                secret.community = String(f.get("community"));
              else {
                secret.username = String(f.get("username"));
                secret.authPassword = String(f.get("authPassword"));
                secret.privPassword = String(f.get("privPassword"));
              }
              await api.createCredential({
                name: String(f.get("name")),
                kind: "snmp",
                secret,
              });
              e.currentTarget.reset();
              refresh();
            }}
          >
            <h3>Add SNMP credential</h3>
            <input required name="name" placeholder="Credential name" />
            <select name="version">
              <option value="2c">SNMP v2c</option>
              <option value="3">SNMP v3 SHA/AES</option>
            </select>
            <input name="community" placeholder="v2c community" />
            <input name="username" placeholder="v3 username" />
            <input
              name="authPassword"
              type="password"
              placeholder="v3 authentication password"
            />
            <input
              name="privPassword"
              type="password"
              placeholder="v3 privacy password (optional)"
            />
            <button className="primary">Save encrypted credential</button>
          </form>
          <form
            className="settingsForm"
            onSubmit={async (e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              await api.createCredential({
                name: String(f.get("name")),
                kind: "routeros",
                secret: {
                  username: String(f.get("username")),
                  password: String(f.get("password")),
                  tls: String(f.get("tls") ?? "false"),
                  serverName: String(f.get("serverName")),
                  caCertificate: String(f.get("caCertificate")),
                },
              });
              e.currentTarget.reset();
              refresh();
            }}
          >
            <h3>Add MikroTik RouterOS API credential</h3>
            <input required name="name" placeholder="Credential name" />
            <input required name="username" placeholder="RouterOS username" />
            <input
              required
              name="password"
              type="password"
              placeholder="RouterOS password"
            />
            <select name="tls">
              <option value="true">API-SSL (recommended)</option>
              <option value="false">Plain API</option>
            </select>
            <input
              name="serverName"
              placeholder="TLS certificate server name"
            />
            <textarea
              name="caCertificate"
              placeholder="Optional CA certificate PEM"
            />
            <button className="primary">Save RouterOS credential</button>
          </form>
        </div>
      )}
      {(current.role === "administrator" || current.role === "manager") && (
        <div className="card panel">
          <Title text="Sites" />
          <div className="rows">
            {sites.map((s) => (
              <div className="row" key={s.id}>
                <div className="grow">
                  <b>{s.name}</b>
                  <small>
                    {s.latitude == null
                      ? "No map coordinates"
                      : `${s.latitude}, ${s.longitude}`}
                  </small>
                </div>
              </div>
            ))}
          </div>
          <form
            className="settingsForm"
            onSubmit={async (e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              const lat = String(f.get("latitude"));
              const lon = String(f.get("longitude"));
              await api.createSite({
                name: String(f.get("name")),
                latitude: lat ? Number(lat) : null,
                longitude: lon ? Number(lon) : null,
              });
              e.currentTarget.reset();
              refresh();
            }}
          >
            <h3>Add site or location</h3>
            <input required name="name" placeholder="Site name" />
            <input
              name="latitude"
              type="number"
              step="any"
              placeholder="Latitude"
            />
            <input
              name="longitude"
              type="number"
              step="any"
              placeholder="Longitude"
            />
            <button className="primary">Create site</button>
          </form>
        </div>
      )}
      {(current.role === "administrator" || current.role === "manager") && (
        <div className="card panel">
          <Title text="Agents and collectors" />
          {enrollment && (
            <div className="tokenReveal">
              <b>One-time enrollment command (expires in 15 minutes)</b>
              <code>{enrollment}</code>
            </div>
          )}
          <div className="rows">
            {identities.map((i) => (
              <div className="row" key={i.id}>
                <div className="grow">
                  <b>{i.name}</b>
                  <small>
                    {i.kind} ·{" "}
                    {i.lastSeenAt
                      ? `Seen ${new Date(i.lastSeenAt).toLocaleString()}`
                      : "Never connected"}
                    {i.revokedAt ? " · Revoked" : ""}
                  </small>
                </div>
                {!i.revokedAt && (
                  <button
                    className="secondary"
                    onClick={async () => {
                      await api.revokeIdentity(i.id);
                      refresh();
                    }}
                  >
                    Revoke
                  </button>
                )}
              </div>
            ))}
          </div>
          {agentInventory.length > 0 && (
            <div className="rows">
              {agentInventory.map((agent) => (
                <div className="row" key={agent.AgentID}>
                  <div className="grow">
                    <b>{agent.DeviceName}</b>
                    <small>
                      {String(agent.Metrics.osName ?? "Linux")} · agent {agent.Version || "dev"} · {String(agent.Metrics.packageManager ?? "unknown")} · {Number(agent.Metrics.pendingUpdates ?? 0)} pending updates · reported {new Date(agent.ReportedAt).toLocaleString()}
                    </small>
                  </div>
                  <Status value={Number(agent.Metrics.pendingUpdates ?? 0) > 0 ? "degraded" : "up"} />
                </div>
              ))}
            </div>
          )}
          <form
            className="settingsForm"
            onSubmit={async (e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              const kind = String(f.get("kind")) as "agent" | "collector";
              const result = await api.createEnrollment(
                kind,
                String(f.get("siteId")),
              );
              const binary =
                kind === "agent" ? "golive-agent" : "golive-collector";
              setEnrollment(
                `${binary} -server https://${location.hostname}:9443 -enroll-url ${location.origin} -enroll-token ${result.token}`,
              );
              refresh();
            }}
          >
            <h3>Enroll a node</h3>
            <select name="kind">
              <option value="agent">Linux agent</option>
              <option value="collector">Remote site collector</option>
            </select>
            <select name="siteId">
              <option value="">No site assignment</option>
              {sites.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
            <button className="primary">Generate one-time token</button>
          </form>
        </div>
      )}
      {current.role !== "viewer" && (
        <div className="card panel">
          <Title text="Maintenance windows" />
          <div className="rows">
            {maintenance.map((window) => (
              <div className="row" key={window.ID}>
                <div className="grow"><b>{window.Name}</b><small>{new Date(window.StartsAt).toLocaleString()} – {new Date(window.EndsAt).toLocaleString()}</small></div>
                <button className="secondary" onClick={async () => { await api.deleteMaintenanceWindow(window.ID); refresh(); }}>Delete</button>
              </div>
            ))}
          </div>
          <form className="settingsForm" onSubmit={async (e) => {
            e.preventDefault(); const f = new FormData(e.currentTarget);
            await api.createMaintenanceWindow({ Name: String(f.get("name")), SiteID: String(f.get("siteId")), DeviceID: "", StartsAt: new Date(String(f.get("startsAt"))).toISOString(), EndsAt: new Date(String(f.get("endsAt"))).toISOString() });
            e.currentTarget.reset(); refresh();
          }}>
            <h3>Schedule site maintenance</h3>
            <input required name="name" placeholder="Core switch upgrade" />
            <select required name="siteId"><option value="">Select site</option>{sites.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}</select>
            <label>Starts<input required name="startsAt" type="datetime-local" /></label>
            <label>Ends<input required name="endsAt" type="datetime-local" /></label>
            <button className="primary">Schedule maintenance</button>
          </form>
        </div>
      )}
      {current.role !== "viewer" && (
        <div className="card panel">
          <Title text="Alert channels" />
          <div className="rows">
            {channels.map((c) => (
              <div className="row" key={c.id}>
                <div className="grow">
                  <b>{c.name}</b>
                  <small>
                    {c.kind} · {c.siteId ? sites.find((s) => s.id === c.siteId)?.name ?? "Site" : "All sites"} · {c.notifyOpened ? "failures" : ""}{c.notifyOpened && c.notifyResolved ? " + " : ""}{c.notifyResolved ? "recoveries" : ""}{c.repeatMinutes ? ` · repeat ${c.repeatMinutes}m` : ""}
                  </small>
                </div>
                <button
                  className="secondary"
                  onClick={async () => {
                    await api.deleteChannel(c.id);
                    refresh();
                  }}
                >
                  Delete
                </button>
              </div>
            ))}
          </div>
          <form
            className="settingsForm"
            onSubmit={async (e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              const name = String(f.get("name"));
              const kind = channelKind;
              const secret: Record<string, string> =
                kind === "email"
                  ? {
                      host: String(f.get("host")),
                      port: String(f.get("port")),
                      username: String(f.get("username")),
                      password: String(f.get("password")),
                      from: String(f.get("from")),
                      to: String(f.get("to")),
                    }
                  : { url: String(f.get("url")) };
              const cred = await api.createCredential({
                name: `${name} delivery`,
                kind: kind === "email" ? "smtp" : "webhook",
                secret,
              });
              await api.createChannel({ name, kind, credentialId: cred.id, siteId: String(f.get("siteId")), notifyOpened: f.has("notifyOpened"), notifyResolved: f.has("notifyResolved"), repeatMinutes: Number(f.get("repeatMinutes") || 0) });
              e.currentTarget.reset();
              refresh();
            }}
          >
            <h3>Add alert destination</h3>
            <input required name="name" placeholder="Channel name" />
            <select name="siteId"><option value="">All sites</option>{sites.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}</select>
            <label><input name="notifyOpened" type="checkbox" defaultChecked /> New failures</label>
            <label><input name="notifyResolved" type="checkbox" defaultChecked /> Recoveries</label>
            <input name="repeatMinutes" type="number" min="0" max="1440" placeholder="Repeat minutes (0 disables)" />
            <select
              value={channelKind}
              onChange={(e) =>
                setChannelKind(e.target.value as "email" | "slack" | "teams")
              }
            >
              <option value="email">Email</option>
              <option value="slack">Slack webhook</option>
              <option value="teams">Teams webhook</option>
            </select>
            {channelKind === "email" ? (
              <>
                <input required name="host" placeholder="SMTP host" />
                <input name="port" placeholder="587" />
                <input name="username" placeholder="SMTP username" />
                <input
                  name="password"
                  type="password"
                  placeholder="SMTP password"
                />
                <input
                  required
                  name="from"
                  type="email"
                  placeholder="From address"
                />
                <input
                  required
                  name="to"
                  type="email"
                  placeholder="Recipient"
                />
              </>
            ) : (
              <input
                required
                name="url"
                type="url"
                placeholder="Incoming webhook URL"
              />
            )}
            <button className="primary">Create channel</button>
          </form>
        </div>
      )}
    </section>
  );
}

function Modal({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
}) {
  return (
    <div
      className="overlay"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div className="modal">
        <div className="title">
          <h3>{title}</h3>
          <button onClick={onClose}>×</button>
        </div>
        {children}
      </div>
    </div>
  );
}
function DeviceModal({
  device,
  devices,
  monitServices,
  onClose,
  onSaved,
}: {
  device?: Device;
  devices: Device[];
  monitServices: MonitService[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(device?.Name ?? "");
  const [address, setAddress] = useState(device?.Address ?? "");
  const [kind, setKind] = useState(device?.Kind ?? "server");
  const [sites, setSites] = useState<Site[]>([]);
  const [site, setSite] = useState(device?.SiteID ?? "");
  const [parent, setParent] = useState(device?.ParentID ?? "");
  const [tags, setTags] = useState((device?.Tags ?? []).join(", "));
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    api.sites().then((v) => {
      setSites(v);
      if (v.length && !device?.SiteID) setSite(v[0].id);
    });
  }, [device?.SiteID]);
  const reported = device ? monitServices.filter((s) => s.DeviceID === device.ID) : [];
  return (
    <Modal title={device ? `Manage ${device.Name}` : "Add a monitored device"} onClose={onClose}>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          try {
            const value = {
              Name: name,
              Address: address,
              Kind: kind,
              SiteID: site,
              ParentID: parent,
              Tags: tags.split(",").map((v) => v.trim()).filter(Boolean),
            };
            if (device) await api.updateDevice(device.ID, value);
            else await api.createDevice(value);
            onSaved();
          } finally {
            setBusy(false);
          }
        }}
      >
        <label>
          Name
          <input
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="mail-01"
          />
        </label>
        <label>
          Address
          <input
            required
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            placeholder="10.0.0.12"
          />
        </label>
        <label>
          Site
          <select
            required
            value={site}
            onChange={(e) => { setSite(e.target.value); setParent(""); }}
          >
            {sites.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Parent device
          <select value={parent} onChange={(e) => setParent(e.target.value)}>
            <option value="">No parent</option>
            {devices.filter((d) => d.ID !== device?.ID && d.SiteID === site).map((d) => (
              <option key={d.ID} value={d.ID}>{d.Name}</option>
            ))}
          </select>
        </label>
        <label>
          Device type
          <select value={kind} onChange={(e) => setKind(e.target.value)}>
            <option>server</option>
            <option>router</option>
            <option>switch</option>
            <option>other</option>
          </select>
        </label>
        <label>
          Tags
          <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="production, mail, customer-a" />
        </label>
        {device && (
          <div className="monitList">
            <div><b>Monit services</b><small>{reported.length} reported</small></div>
            {reported.map((service) => (
              <div className="monitService" key={service.Name}>
                <span>
                  <b>{service.Name}</b>
                  <small>{monitType(service.Type)} · {service.Monitor ? "monitored" : "not monitored"}</small>
                </span>
                <Status value={service.Monitor === 0 ? "unknown" : service.Status === 0 ? "up" : "down"} />
              </div>
            ))}
            {!reported.length && <small>No Monit services have been reported yet.</small>}
          </div>
        )}
        <button className="primary" disabled={busy}>
          {device ? "Save device" : "Add device"}
        </button>
      </form>
    </Modal>
  );
}
function monitType(value: number) {
  return ({ 0: "filesystem", 1: "directory", 2: "file", 3: "process", 4: "host", 5: "system", 6: "fifo", 7: "program", 8: "network" } as Record<number, string>)[value] ?? `type ${value}`;
}
function CheckModal({
  devices,
  onClose,
  onSaved,
}: {
  devices: Device[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [device, setDevice] = useState(devices[0]?.ID ?? "");
  const [name, setName] = useState("Availability");
  const [type, setType] = useState<Check["Type"]>("ping");
  const [target, setTarget] = useState("");
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [credential, setCredential] = useState("");
  const [oid, setOID] = useState(".1.3.6.1.2.1.1.3.0");
  useEffect(() => {
    api.credentials().then((v) => {
      setCredentials(v);
      if (v.length) setCredential(v.find((c) => c.kind === "snmp")?.id ?? "");
    });
  }, []);
  return (
    <Modal title="Add a service check" onClose={onClose}>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          await api.createCheck({
            DeviceID: device,
            Name: name,
            Type: type,
            Target: target,
            IntervalSeconds: 30,
            TimeoutSeconds: 5,
            CredentialID:
              type === "snmp" || type === "routeros" ? credential : "",
            Config:
              type === "snmp"
                ? { oid }
                : type === "tls"
                  ? { minimumDays: 30 }
                  : {},
          });
          onSaved();
        }}
      >
        <label>
          Device
          <select
            required
            value={device}
            onChange={(e) => setDevice(e.target.value)}
          >
            {devices.map((d) => (
              <option value={d.ID} key={d.ID}>
                {d.Name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Check name
          <input
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <label>
          Type
          <select
            value={type}
            onChange={(e) => setType(e.target.value as Check["Type"])}
          >
            <option value="ping">Ping</option>
            <option value="http">HTTP(S)</option>
            <option value="tcp">TCP port</option>
            <option value="snmp">SNMP OID</option>
            <option value="dns">DNS resolution</option>
            <option value="tls">TLS certificate</option>
            <option value="ssh">SSH banner</option>
            <option value="smtp">SMTP banner</option>
            <option value="mysql">MariaDB/MySQL port</option>
            <option value="postgres">PostgreSQL port</option>
            <option value="routeros">MikroTik RouterOS API</option>
          </select>
        </label>
        <label>
          Target
          <input
            required
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder={
              type === "ping"
                ? "host.example.com"
                : type === "http"
                  ? "https://example.com"
                  : type === "snmp"
                    ? "router.example.com:161"
                    : "host.example.com:443"
            }
          />
        </label>
        {(type === "snmp" || type === "routeros") && (
          <>
            <label>
              {type === "snmp" ? "SNMP" : "RouterOS"} credential
              <select
                required
                value={credential}
                onChange={(e) => setCredential(e.target.value)}
              >
                <option value="">Select credential</option>
                {credentials
                  .filter(
                    (c) => c.kind === (type === "snmp" ? "snmp" : "routeros"),
                  )
                  .map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.name}
                    </option>
                  ))}
              </select>
            </label>
            {type === "snmp" && (
              <label>
                OID
                <input
                  required
                  value={oid}
                  onChange={(e) => setOID(e.target.value)}
                />
              </label>
            )}
          </>
        )}
        <button className="primary">Create check</button>
      </form>
    </Modal>
  );
}
