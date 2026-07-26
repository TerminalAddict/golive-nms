export type SettingsPage = "access" | "sites" | "credentials" | "agents" | "notifications";
export type SettingsPageDefinition = {
  id: SettingsPage;
  label: string;
  description: string;
};

export function settingsPagesForRole(role: string): SettingsPageDefinition[] {
  const canConfigure = role === "administrator" || role === "manager";
  return [
    { id: "access", label: "Access", description: "Users and API tokens" },
    ...(role !== "viewer"
      ? [{ id: "sites" as const, label: "Sites & maintenance", description: "Locations and maintenance windows" }]
      : []),
    ...(canConfigure
      ? [
          { id: "credentials" as const, label: "Credentials", description: "SNMP, RouterOS, and Monit secrets" },
          { id: "agents" as const, label: "Agents & collectors", description: "Enrollment and connected nodes" },
          { id: "notifications" as const, label: "Notifications", description: "Email, Slack, and Teams alerts" },
        ]
      : []),
  ];
}
