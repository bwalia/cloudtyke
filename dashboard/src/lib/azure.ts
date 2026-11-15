/** Types and helpers for the Azure objects the dashboard shows. */

export interface Subscription {
  id: string;
  subscriptionId: string;
  displayName: string;
  state: string;
  tenantId: string;
}

export interface ResourceGroup {
  id: string;
  name: string;
  location: string;
  tags?: Record<string, string>;
  properties?: { provisioningState?: string };
  managedBy?: string;
}

export interface Resource {
  id: string;
  name: string;
  type: string;
  location?: string;
  tags?: Record<string, string>;
}

export interface Location {
  name: string;
  displayName: string;
  regionalDisplayName?: string;
  metadata?: { geography?: string; geographyGroup?: string; regionCategory?: string; latitude?: string; longitude?: string };
  availabilityZoneMappings?: { logicalZone: string; physicalZone: string }[];
}

export interface RoleAssignment {
  id: string;
  name: string;
  properties: {
    principalId: string;
    principalType?: string;
    roleDefinitionId: string;
    scope: string;
    description?: string;
    createdOn?: string;
    createdBy?: string;
  };
}

export interface RoleDefinition {
  id: string;
  name: string;
  properties: {
    roleName: string;
    description?: string;
    type: string;
    assignableScopes?: string[];
    permissions?: { actions?: string[]; notActions?: string[]; dataActions?: string[]; notDataActions?: string[] }[];
  };
}

export interface ActivityEvent {
  eventDataId: string;
  eventTimestamp: string;
  caller: string;
  correlationId: string;
  level: string;
  resourceGroupName?: string;
  resourceId?: string;
  operationName?: { value: string; localizedValue?: string };
  status?: { value: string };
  subStatus?: { value: string };
  httpRequest?: { method?: string; clientIpAddress?: string };
  authorization?: { action?: string; scope?: string };
}

export interface GraphObject {
  id: string;
  displayName?: string;
  "@odata.type"?: string;
  [k: string]: unknown;
}

export interface User extends GraphObject {
  userPrincipalName: string;
  accountEnabled?: boolean;
  mail?: string | null;
  jobTitle?: string | null;
  department?: string | null;
  userType?: string;
  createdDateTime?: string;
}

export interface Group extends GraphObject {
  description?: string | null;
  mailNickname?: string;
  securityEnabled?: boolean;
  mailEnabled?: boolean;
  groupTypes?: string[];
  createdDateTime?: string;
}

export interface Application extends GraphObject {
  appId: string;
  signInAudience?: string;
  createdDateTime?: string;
  passwordCredentials?: { keyId: string; displayName?: string; endDateTime?: string; hint?: string }[];
  appRoles?: { id: string; value: string; displayName?: string }[];
}

export interface ServicePrincipal extends GraphObject {
  appId: string;
  servicePrincipalType?: string;
  accountEnabled?: boolean;
  appOwnerOrganizationId?: string;
  appRoles?: { id: string; value: string; displayName?: string; description?: string }[];
}

export interface DirectoryRole extends GraphObject {
  description?: string;
  roleTemplateId: string;
}

export interface RequestLogEntry {
  time: string;
  requestId: string;
  service: string;
  method: string;
  host: string;
  path: string;
  status: number;
  durationMs: number;
  error?: string;
}

export interface SelfTestCall {
  method: string;
  url: string;
  status: number;
  error?: string;
  step?: string;
  expect?: string;
  verified?: string[];
  request?: string;
  response?: string;
}

export interface SelfTestResult {
  section: "ARM" | "Token" | "Graph" | "?";
  service: string;
  title: string;
  notes?: string;
  status: "pass" | "fail" | "missing" | "stale";
  error?: string;
  durationMs: number;
  calls: SelfTestCall[] | null;
}

export interface SelfTestReport {
  started: string;
  durationMs: number;
  passed: number;
  failed: number;
  results: SelfTestResult[];
}

export interface CoverageRow {
  type: string;
  apiVersions?: string[];
  notes?: string;
  phase?: number;
}

/** Parses an ARM id into its parts. */
export function parseId(id: string) {
  const parts = id.split("/").filter(Boolean);
  const get = (k: string) => {
    const i = parts.findIndex((p) => p.toLowerCase() === k.toLowerCase());
    return i >= 0 ? parts[i + 1] : undefined;
  };
  return { subscription: get("subscriptions"), resourceGroup: get("resourceGroups"), name: parts[parts.length - 1] };
}

export const shortId = (id: string) => (id.length > 13 ? id.slice(0, 8) + "…" + id.slice(-4) : id);

export function ago(iso?: string) {
  if (!iso) return "";
  const s = Math.round((Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 5) return "just now";
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

/** How an assignment's scope relates to the scope being viewed. */
export function scopeRelation(assignment: string, viewing: string): "this" | "inherited" | "child" {
  const a = assignment.toLowerCase().replace(/\/$/, "") || "/";
  const v = viewing.toLowerCase().replace(/\/$/, "") || "/";
  if (a === v) return "this";
  if (a === "/" || v.startsWith(a + "/")) return "inherited";
  return "child";
}

export function roleId(def: string) {
  return def.split("/").pop() || def;
}

/** Well-known built-in role GUIDs, so assignments render without a lookup. */
export const BUILTIN_ROLES: Record<string, string> = {
  "8e3af657-a8ff-443c-a75c-2fe8c4bcb635": "Owner",
  "b24988ac-6180-42a0-ab88-20f7382dd24c": "Contributor",
  "acdd72a7-3385-48ef-bd42-f606fba81ae7": "Reader",
  "18d7d88d-d35e-4fb5-a5c3-7773c20a72d9": "User Access Administrator",
};

export const SECTION_LABEL: Record<string, string> = {
  ARM: "Azure Resource Manager",
  Token: "Entra ID token service",
  Graph: "Microsoft Graph",
  "?": "Stale checks",
};
