import type { Check, Device, Incident, MonitService, Summary } from "./types";
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new Error(
      body?.error?.message ?? `Request failed (${response.status})`,
    );
  }
  if (response.status === 204) return undefined as T;
  return response.json();
}
export interface User {
  id: string;
  email: string;
  displayName: string;
  role: string;
}
export interface APIToken {
  id: string;
  name: string;
  expiresAt: string | null;
  lastUsedAt: string | null;
  createdAt: string;
  token?: string;
}
export interface Credential {
  id: string;
  name: string;
  kind: string;
  secret?: Record<string, string>;
}
export interface MonitControl {
  DeviceID: string;
  URL: string;
  CredentialID: string;
  CredentialName?: string;
  UpdatedAt?: string;
  Enabled?: boolean;
}
export interface MonitAction {
  ID: string;
  DeviceID: string;
  Service: string;
  Action: string;
  Success: boolean;
  Message: string;
  RequestedAt: string;
}
export interface NotificationChannel {
  id: string;
  name: string;
  kind: "email" | "slack" | "teams";
  credentialId: string;
  enabled: boolean;
  siteId: string;
  notifyOpened: boolean;
  notifyResolved: boolean;
  repeatMinutes: number;
}
export interface AgentInventory {
  DeviceID: string;
  DeviceName: string;
  SiteID: string;
  AgentID: string;
  Version: string;
  Metrics: Record<string, string | number>;
  ReportedAt: string;
}
export interface MaintenanceWindow {
  ID: string;
  SiteID: string;
  DeviceID: string;
  Name: string;
  StartsAt: string;
  EndsAt: string;
}
export interface Site {
  id: string;
  name: string;
  latitude: number | null;
  longitude: number | null;
}
export interface CheckSample {
  observedAt: string;
  up: boolean;
  latencyMs: number;
  message: string;
}
export interface Identity {
  id: string;
  kind: "agent" | "collector";
  siteId: string;
  name: string;
  serial: string;
  expiresAt: string;
  revokedAt: string | null;
  lastSeenAt: string | null;
}
export interface DeviceEvent {
  id: number;
  deviceId: string;
  protocol: "syslog" | "snmp_trap";
  source: string;
  facility: number | null;
  severity: number | null;
  message: string;
  fields: Record<string, unknown>;
  receivedAt: string;
  siteId: string;
}
export interface ConfigProfile {
  id: string;
  deviceId: string;
  deviceName: string;
  address: string;
  siteId: string;
  credentialId: string;
  command: string;
  intervalSeconds: number;
  enabled: boolean;
  lastRunAt: string | null;
  lastError: string;
}
export interface ConfigSnapshot {
  id: string;
  deviceId: string;
  contentHash: string;
  capturedAt: string;
}
export interface ActionTemplate {
  id: string;
  name: string;
  executable: string;
  arguments: string[];
  timeoutSeconds: number;
  autoCheckType: string;
  enabled: boolean;
}
export interface RemediationJob {
  id: string;
  templateId: string;
  templateName: string;
  deviceId: string;
  deviceName: string;
  siteId: string;
  automatic: boolean;
  state: string;
  output: string;
  error: string;
  queuedAt: string;
  startedAt: string | null;
  finishedAt: string | null;
}
export const api = {
  authConfig: () => request<{ oidcEnabled: boolean }>("/auth/config"),
  me: () => request<User>("/auth/me"),
  login: (email: string, password: string) =>
    request<User>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),
  logout: () => request<void>("/auth/logout", { method: "POST" }),
  users: () => request<User[]>("/users"),
  createUser: (v: {
    email: string;
    displayName: string;
    password: string;
    role: string;
  }) => request<User>("/users", { method: "POST", body: JSON.stringify(v) }),
  deleteUser: (id: string) =>
    request<void>(`/users/${id}`, { method: "DELETE" }),
  updateUser: (
    id: string,
    v: { displayName: string; role: string; password?: string },
  ) =>
    request<User>(`/users/${id}`, { method: "PATCH", body: JSON.stringify(v) }),
  tokens: () => request<APIToken[]>("/api-tokens"),
  createToken: (name: string) =>
    request<APIToken>("/api-tokens", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  deleteToken: (id: string) =>
    request<void>(`/api-tokens/${id}`, { method: "DELETE" }),
  credentials: () => request<Credential[]>("/credentials"),
  createCredential: (v: {
    name: string;
    kind: string;
    secret: Record<string, string>;
  }) =>
    request<Credential>("/credentials", {
      method: "POST",
      body: JSON.stringify(v),
    }),
  deleteCredential: (id: string) =>
    request<void>(`/credentials/${id}`, { method: "DELETE" }),
  channels: () => request<NotificationChannel[]>("/notification-channels"),
  createChannel: (v: Omit<NotificationChannel, "id" | "enabled">) =>
    request<NotificationChannel>("/notification-channels", {
      method: "POST",
      body: JSON.stringify(v),
    }),
  deleteChannel: (id: string) =>
    request<void>(`/notification-channels/${id}`, { method: "DELETE" }),
  summary: () => request<Summary>("/summary"),
  devices: () => request<Device[]>("/devices"),
  monitServices: () => request<MonitService[]>("/monit-services"),
  monitControl: (deviceId: string) =>
    request<MonitControl>(`/devices/${deviceId}/monit-control`),
  setMonitControl: (deviceId: string, URL: string, CredentialID: string) =>
    request<MonitControl>(`/devices/${deviceId}/monit-control`, {
      method: "PUT",
      body: JSON.stringify({ URL, CredentialID }),
    }),
  testMonitControl: (deviceId: string) =>
    request<{ ok: boolean; message: string }>(`/devices/${deviceId}/monit-control/test`, {
      method: "POST",
    }),
  monitActions: (deviceId: string) =>
    request<MonitAction[]>(`/devices/${deviceId}/monit-actions`),
  runMonitAction: (deviceId: string, Service: string, Action: string) =>
    request<MonitAction>(`/devices/${deviceId}/monit-actions`, {
      method: "POST",
      body: JSON.stringify({ Service, Action }),
    }),
  checks: () => request<Check[]>("/checks"),
  history: (id: string) => request<CheckSample[]>(`/checks/${id}/history`),
  incidents: () => request<Incident[]>("/incidents"),
  deviceEvents: (protocol = "", q = "") =>
    request<DeviceEvent[]>(
      `/device-events?protocol=${encodeURIComponent(protocol)}&q=${encodeURIComponent(q)}`,
    ),
  configProfiles: () => request<ConfigProfile[]>("/config-profiles"),
  createConfigProfile: (v: {
    deviceId: string;
    credentialId: string;
    command: string;
    intervalSeconds: number;
  }) =>
    request<ConfigProfile>("/config-profiles", {
      method: "POST",
      body: JSON.stringify(v),
    }),
  triggerConfig: (id: string) =>
    request<void>(`/config-profiles/${id}/trigger`, { method: "POST" }),
  configSnapshots: (deviceId: string) =>
    request<ConfigSnapshot[]>(`/devices/${deviceId}/config-snapshots`),
  configDiff: (from: string, to: string) =>
    request<{ diff: string }>(
      `/config-diff?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
    ),
  actionTemplates: () => request<ActionTemplate[]>("/action-templates"),
  createActionTemplate: (v: Omit<ActionTemplate, "id" | "enabled">) =>
    request<ActionTemplate>("/action-templates", {
      method: "POST",
      body: JSON.stringify(v),
    }),
  remediationJobs: () => request<RemediationJob[]>("/remediation-jobs"),
  agentInventory: () => request<AgentInventory[]>("/agent-inventory"),
  maintenanceWindows: () => request<MaintenanceWindow[]>("/maintenance-windows"),
  createMaintenanceWindow: (value: Omit<MaintenanceWindow, "ID">) =>
    request<MaintenanceWindow>("/maintenance-windows", { method: "POST", body: JSON.stringify(value) }),
  deleteMaintenanceWindow: (id: string) => request<void>(`/maintenance-windows/${id}`, { method: "DELETE" }),
  queueRemediation: (templateId: string, deviceId: string) =>
    request<RemediationJob>("/remediation-jobs", {
      method: "POST",
      body: JSON.stringify({ templateId, deviceId }),
    }),
  remediationSettings: () =>
    request<{ enabled: boolean }>("/remediation-settings"),
  setRemediation: (enabled: boolean) =>
    request<void>("/remediation-settings", {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    }),
  createDevice: (value: Partial<Device>) =>
    request<Device>("/devices", {
      method: "POST",
      body: JSON.stringify(value),
    }),
  updateDevice: (id: string, value: Partial<Device>) =>
    request<Device>(`/devices/${id}`, {
      method: "PATCH",
      body: JSON.stringify(value),
    }),
  sites: () => request<Site[]>("/sites"),
  createSite: (v: {
    name: string;
    latitude: number | null;
    longitude: number | null;
  }) => request<Site>("/sites", { method: "POST", body: JSON.stringify(v) }),
  deleteSite: (id: string) =>
    request<void>(`/sites/${id}`, { method: "DELETE" }),
  userSites: (id: string) =>
    request<{ siteIds: string[] }>(`/users/${id}/sites`),
  setUserSites: (id: string, siteIds: string[]) =>
    request<void>(`/users/${id}/sites`, {
      method: "PUT",
      body: JSON.stringify({ siteIds }),
    }),
  createEnrollment: (kind: "agent" | "collector", siteId: string) =>
    request<{ token: string; expiresAt: string }>("/enrollment-tokens", {
      method: "POST",
      body: JSON.stringify({ kind, siteId, ttlMinutes: 15 }),
    }),
  identities: () => request<Identity[]>("/identities"),
  revokeIdentity: (id: string) =>
    request<void>(`/identities/${id}`, { method: "DELETE" }),
  createCheck: (value: Partial<Check>) =>
    request<Check>("/checks", { method: "POST", body: JSON.stringify(value) }),
  acknowledge: (id: string) =>
    request<void>(`/incidents/${id}/acknowledge`, { method: "POST" }),
  assignIncident: (id: string, assigned: boolean) => request<void>(`/incidents/${id}/assign`, { method: "POST", body: JSON.stringify({ assigned }) }),
  noteIncident: (id: string, note: string) => request<void>(`/incidents/${id}/notes`, { method: "POST", body: JSON.stringify({ note }) }),
};
