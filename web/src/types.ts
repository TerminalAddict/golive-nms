export type Status = "up" | "down" | "degraded" | "unknown" | "dependency";
export interface Summary {
  Total: number;
  Up: number;
  Down: number;
  Degraded: number;
  Unknown: number;
  OpenIncidents: number;
}
export interface Device {
  ID: string;
  SiteID: string;
  SiteName: string;
  ParentID: string;
  Name: string;
  Address: string;
  Kind: string;
  Status: Status;
  Tags: string[];
  LastSeenAt: string | null;
}
export interface Check {
  ID: string;
  DeviceID: string;
  DeviceName: string;
  Name: string;
  Type:
    | "ping"
    | "http"
    | "tcp"
    | "snmp"
    | "dns"
    | "tls"
    | "ssh"
    | "smtp"
    | "mysql"
    | "postgres"
    | "routeros";
  Target: string;
  CredentialID: string;
  Config: Record<string, unknown>;
  IntervalSeconds: number;
  TimeoutSeconds: number;
  Enabled: boolean;
  Status: "up" | "down" | "unknown";
  LastError: string;
  LastRunAt: string | null;
}
export interface Incident {
  ID: string;
  CheckID: string;
  DeviceID: string;
  DeviceName: string;
  Title: string;
  Severity: string;
  State: "open" | "acknowledged" | "resolved";
  OpenedAt: string;
  AssignedTo: string;
  AssignedName: string;
  Notes: string;
}
