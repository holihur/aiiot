// Typed API client for the aiiot core.
const BASE = "/api/v1";

export interface User {
  id: number;
  username: string;
  email: string;
  displayName?: string;
  systemRole: string;
  status: string;
}

export interface Project {
  id: number;
  key: string;
  name: string;
  description?: string;
  ownerId: number;
  owner?: User;
  members?: ProjectMember[];
  myRole?: string;
}

export interface ProjectMember {
  id: number;
  projectId: number;
  userId: number;
  role: string;
  user?: User;
}

export interface Workspace {
  id: number;
  projectId: number;
  key: string;
  name: string;
  description?: string;
}

export interface Product {
  id: number;
  key: string;
  name: string;
  category?: string;
  protocol: "mqtt" | "coap" | "custom" | "modbus" | "opcua";
  dataFormat: string;
  description?: string;
  status: string;
  createdBy?: number;
  thingModel?: ThingModel;
}

export interface ModelParam {
  name: string;
  dataType: string;
  required?: boolean;
  unit?: string;
  min?: number;
  max?: number;
  enumValues?: string[];
  description?: string;
}

export interface ThingModelElement {
  id?: number;
  thingModelId?: number;
  type: "property" | "service" | "event";
  identifier: string;
  name?: string;
  dataType: string;
  eventType?: string;
  accessMode?: string;
  unit?: string;
  min?: number | null;
  max?: number | null;
  step?: number | null;
  specs?: Record<string, unknown>;
  required?: boolean;
  description?: string;
}

export interface ThingModel {
  id?: number;
  productId: number;
  version: string;
  status?: string;
  description?: string;
  elements: ThingModelElement[];
}

export interface Device {
  id: number;
  projectId: number;
  workspaceId: number;
  productId: number;
  key: string;
  name: string;
  status: string;
  online: boolean;
  lastOnlineAt?: string | null;
  lastSeenAt?: string | null;
  tags?: Record<string, unknown>;
  product?: Product;
  workspace?: Workspace;
}

export interface LatestValue {
  deviceId: number;
  identifier: string;
  dataType: string;
  numValue?: number | null;
  boolValue?: boolean | null;
  strValue?: string | null;
  jsonValue?: Record<string, unknown> | null;
  updatedAt: string;
}

export interface TelemetryPoint {
  time?: string;
  bucket?: string;
  identifier?: string;
  numValue?: number | null;
  boolValue?: boolean | null;
  strValue?: string | null;
  jsonValue?: unknown;
  avg?: number | null;
  min?: number | null;
  max?: number | null;
  count?: number;
}

export interface DeviceEvent {
  id: number;
  identifier: string;
  type: string;
  payload: Record<string, unknown>;
  occurredAt: string;
}

export interface DeviceDownlinkLog {
  id: number;
  deviceId: number;
  kind: string;
  identifier?: string;
  payload?: Record<string, unknown>;
  source: string;
  status: "sent" | "failed";
  error?: string;
  target?: string;
  occurredAt: string;
}

export interface TimelineItem {
  at: string;
  type: "device" | "event" | "shadow" | "downlink" | "ota";
  title: string;
  identifier?: string;
  source?: string;
  status?: string;
  detail?: Record<string, unknown>;
}

export interface Rule {
  id: number;
  projectId: number;
  productId?: number | null;
  workspaceId?: number | null;
  name: string;
  description?: string;
  enabled: boolean;
  triggerType: string;
  triggerSource: string;
  cron?: string;
  condition: string;
  actions: Record<string, unknown>[];
  priority: number;
  lastTriggeredAt?: string | null;
  triggerCount: number;
}

export interface RuleLog {
  id: number;
  ruleId: number;
  deviceId: number;
  identifier: string;
  matched: boolean;
  success: boolean;
  error?: string;
  actionsResult?: Record<string, unknown>[];
  occurredAt: string;
}

export interface DeviceShadow {
  deviceId: number;
  desired: Record<string, unknown>;
  reported: Record<string, unknown>;
  delta: Record<string, unknown>;
  version: number;
  updatedAt: string;
}

export interface ShadowLog {
  id: number;
  deviceId: number;
  projectId: number;
  source: string;
  ruleId?: number | null;
  desired: Record<string, unknown>;
  reported: Record<string, unknown>;
  delta: Record<string, unknown>;
  occurredAt: string;
}

export interface RollupInfo {
  rows: number;
  sizeBytes: number;
  lastRefresh: string | null;
}

export interface StorageInfo {
  retentionDays: number;
  defaultDays: number;
  partitions: PartitionInfo[];
  totalRows: number;
  totalBytes: number;
  rollup: RollupInfo;
}

export interface PartitionInfo {
  name: string;
  from: string;
  to: string;
  rows: number;
  sizeBytes: number;
}

export interface Firmware {
  id: number;
  productId: number;
  version: string;
  name?: string;
  description?: string;
  fileName: string;
  fileSize: number;
  checksum: string;
  status: string;
}

export interface OTATaskDevice {
  id: number;
  taskId: number;
  deviceId: number;
  status: string;
  progress: number;
  message?: string;
  wave: number;
  device?: Device;
}

export interface OTATask {
  id: number;
  projectId: number;
  productId: number;
  firmwareId: number;
  name: string;
  status: string;
  total: number;
  succeeded: number;
  failed: number;
  inProgress: number;
  batchSize: number;
  currentWave: number;
  haltOnFailure: boolean;
  createdAt?: string;
  firmware?: Firmware;
  devices?: OTATaskDevice[];
}

export interface NotifyChannel {
  id: number;
  projectId: number;
  name: string;
  type: "webhook" | "dingtalk" | "email";
  enabled: boolean;
  config: Record<string, unknown>;
  description?: string;
}

export interface NotificationLog {
  id: number;
  channelId: number;
  success: boolean;
  error?: string;
  title: string;
  body: string;
  createdAt: string;
}

export interface Alert {
  id: number;
  projectId: number;
  ruleId: number;
  deviceId: number;
  deviceKey: string;
  identifier?: string;
  title: string;
  level: "info" | "warning" | "critical";
  status: "firing" | "acknowledged" | "resolved";
  message?: string;
  startsAt: string;
  lastFiredAt: string;
  fireCount: number;
  resolvedAt?: string;
  resolveReason?: string;
  ackedAt?: string;
  ackedBy?: number;
  escalations?: number;
  escalatedAt?: string;
  ruleName?: string;
  createdAt: string;
}

export interface DashboardPanel {
  type: "stat" | "trend" | "alerts" | "devices" | "map" | "text";
  title?: string;
  config?: Record<string, unknown>;
}

export interface Geofence {
  id: number;
  projectId: number;
  workspaceId: number;
  name: string;
  enabled: boolean;
  polygon: { type: "Polygon"; coordinates: unknown[][][] };
}

export interface DashboardBoard {
  id: number;
  projectId: number;
  workspaceId: number;
  name: string;
  panels: DashboardPanel[];
}

export interface DeviceGroup {
  id: number;
  projectId: number;
  name: string;
  description?: string;
  deviceCount: number;
}

export interface AuditLog {
  id: number;
  userId: number;
  username: string;
  projectId: number;
  method: string;
  path: string;
  status: number;
  ip: string;
  createdAt: string;
}

export interface DeviceConnection {
  protocol: string;
  host: string;
  username: string;
  password: string;
  productKey: string;
  deviceKey: string;
  workspace: string;
  mqtt?: {
    tcpPort: number;
    wsPort: number;
    uplinkTopic: string;
    downlinkTopic: string;
    eventTopic: string;
    peerTopic: string;
    samplePayload: string;
  };
  coap?: {
    udpPort: number;
    propertyPath: string;
    eventPath: string;
    samplePayload: string;
    secretQueryKey: string;
  };
  custom?: {
    tcpPort: number;
    authFrame: Record<string, unknown>;
    propertyFrame: Record<string, unknown>;
  };
}

export interface GatewayInstance {
  instanceId: string;
  protocol: string;
  version: string;
  downlinkUrl: string;
  registeredAt: string;
  lastHeartbeat: string;
  activeDevices: number;
  uptimeSeconds: number;
  healthy: boolean;
}

export interface NATSJetStreamStats {
  stream: string;
  messages: number;
  bytes: number;
  firstSeq: number;
  lastSeq: number;
  consumer: {
    name: string;
    created: string;
    deliveredStreamSeq: number;
    ackFloorStreamSeq: number;
    numPending: number;
    numAckPending: number;
    numRedelivered: number;
    numWaiting: number;
    pushBound: boolean;
  };
}

export interface NATSCounters {
  natsConsumedTotal: number;
  natsConsumeErrors: number;
  natsRedelivered: number;
  ingestTotal: number;
  ingestErrors: number;
  ruleEvaluations: number;
  ruleTriggers: number;
  devicesOnline: number;
  gatewaysHealthy: number;
}

export interface NATSStats {
  jetstream: NATSJetStreamStats | null;
  counters: NATSCounters;
}

export interface RuleReference {
  variables: { name: string; type: string; description: string }[];
  examples: { name: string; condition: string }[];
  triggers: string[];
  actions: string[];
  kinds: string[];
}

export function getToken(): string | null {
  return localStorage.getItem("aiiot_token");
}

export function setToken(token: string | null) {
  if (token) localStorage.setItem("aiiot_token", token);
  else localStorage.removeItem("aiiot_token");
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 401 && !path.startsWith("/auth/login") && !path.startsWith("/auth/register")) {
    setToken(null);
    if (!window.location.pathname.startsWith("/login")) {
      window.location.href = "/login";
    }
    throw new Error("unauthorized");
  }
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    throw new Error(data?.error || `request failed (${res.status})`);
  }
  return data as T;
}

const get = <T>(p: string) => request<T>("GET", p);

// getPaged reads the X-Total-Count header alongside a list response.
async function getPaged<T>(path: string): Promise<{ items: T[]; total: number }> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(`${BASE}${path}`, { headers });
  const text = await res.text();
  const data = text ? JSON.parse(text) : [];
  if (!res.ok) throw new Error(data?.error || `request failed (${res.status})`);
  return { items: data as T[], total: Number(res.headers.get("X-Total-Count") || (data as T[]).length) };
}
const post = <T>(p: string, b?: unknown) => request<T>("POST", p, b);
const put = <T>(p: string, b?: unknown) => request<T>("PUT", p, b);
const patch = <T>(p: string, b?: unknown) => request<T>("PATCH", p, b);
const del = <T>(p: string) => request<T>("DELETE", p);

export const api = {
  login: (username: string, password: string) =>
    post<{ token: string; user: User }>("/auth/login", { username, password }),
  register: (payload: { username: string; email: string; password: string; displayName?: string }) =>
    post<{ token: string; user: User }>("/auth/register", payload),
  me: () => get<User>("/auth/me"),

  listProjects: () => get<Project[]>("/projects"),
  createProject: (b: { key: string; name: string; description?: string }) => post<Project>("/projects", b),
  getProject: (id: number) => get<Project>(`/projects/${id}`),
  updateProject: (id: number, b: { name: string; description?: string }) => put<Project>(`/projects/${id}`, b),
  deleteProject: (id: number) => del<{ ok: boolean }>(`/projects/${id}`),

  listMembers: (id: number) => get<ProjectMember[]>(`/projects/${id}/members`),
  addMember: (id: number, b: { username?: string; email?: string; role: string }) =>
    post<ProjectMember>(`/projects/${id}/members`, b),
  updateMember: (id: number, memberId: number, role: string) =>
    put<{ ok: boolean }>(`/projects/${id}/members/${memberId}`, { role }),
  removeMember: (id: number, memberId: number) => del<{ ok: boolean }>(`/projects/${id}/members/${memberId}`),

  listWorkspaces: (projectId: number) => get<Workspace[]>(`/projects/${projectId}/workspaces`),
  createWorkspace: (projectId: number, b: { key: string; name: string; description?: string }) =>
    post<Workspace>(`/projects/${projectId}/workspaces`, b),
  updateWorkspace: (id: number, b: { name: string; description?: string }) => put<Workspace>(`/workspaces/${id}`, b),
  deleteWorkspace: (id: number) => del<{ ok: boolean }>(`/workspaces/${id}`),

  listProducts: (filters?: { keyword?: string; protocol?: string }) => {
    const qs = new URLSearchParams();
    if (filters?.keyword) qs.set("keyword", filters.keyword);
    if (filters?.protocol) qs.set("protocol", filters.protocol);
    const q = qs.toString();
    return get<Product[]>(`/products${q ? `?${q}` : ""}`);
  },
  createProduct: (b: Partial<Product>) => post<Product>("/products", b),
  getProduct: (id: number) => get<Product>(`/products/${id}`),
  updateProduct: (id: number, b: Partial<Product>) => put<Product>(`/products/${id}`, b),
  deleteProduct: (id: number) => del<{ ok: boolean }>(`/products/${id}`),
  getThingModel: (productId: number) => get<ThingModel>(`/products/${productId}/thing-model`),
  putThingModel: (productId: number, b: { version: string; description?: string; elements: ThingModelElement[] }) =>
    put<ThingModel>(`/products/${productId}/thing-model`, b),
  publishThingModel: (productId: number) =>
    post<{ ok: boolean; version: string; status: string; snapshotId: number }>(
      `/products/${productId}/thing-model/publish`,
    ),
  listThingModelVersions: (productId: number) =>
    get<{ id: number; version: string; createdBy: number; publishedAt: string }[]>(
      `/products/${productId}/thing-model/versions`,
    ),
  rollbackThingModelVersion: (productId: number, versionId: number) =>
    post<{ ok: boolean; elements: number }>(
      `/products/${productId}/thing-model/versions/${versionId}/rollback`,
    ),

  listDevices: (projectId: number, filters?: { productId?: number; workspaceId?: number; keyword?: string }) => {
    const qs = new URLSearchParams();
    if (filters?.productId) qs.set("productId", String(filters.productId));
    if (filters?.workspaceId) qs.set("workspaceId", String(filters.workspaceId));
    if (filters?.keyword) qs.set("keyword", filters.keyword);
    const q = qs.toString();
    return get<Device[]>(`/projects/${projectId}/devices${q ? `?${q}` : ""}`);
  },
  listDevicesPaged: (
    projectId: number,
    params: {
      productId?: number;
      workspaceId?: number;
      keyword?: string;
      page?: number;
      pageSize?: number;
      sort?: string;
      order?: "asc" | "desc";
    },
  ) => {
    const qs = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== "") qs.set(k, String(v));
    });
    return getPaged<Device>(`/projects/${projectId}/devices?${qs.toString()}`);
  },
  createDevice: (productId: number, b: { name: string; workspaceId: number; key?: string; secret?: string }) =>
    post<{ device: Device; secret: string }>(`/products/${productId}/devices`, b),
  getDevice: (id: number) => get<Device>(`/devices/${id}`),
  updateDevice: (id: number, b: { name?: string; workspaceId?: number; tags?: Record<string, unknown> }) => put<Device>(`/devices/${id}`, b),
  deleteDevice: (id: number) => del<{ ok: boolean }>(`/devices/${id}`),
  setDeviceStatus: (id: number, status: "enabled" | "disabled") =>
    put<Device>(`/devices/${id}/status`, { status }),
  rotateSecret: (id: number) => post<{ deviceId: number; secret: string }>(`/devices/${id}/secret`),
  getDeviceConnection: (id: number) => get<DeviceConnection>(`/devices/${id}/connection`),
  latest: (id: number) => get<LatestValue[]>(`/devices/${id}/latest`),
  telemetry: (
    id: number,
    p: { identifier?: string; identifiers?: string[]; from?: string; to?: string; interval?: string; limit?: number },
  ) => {
    const qs = new URLSearchParams();
    if (p.identifiers && p.identifiers.length > 0) {
      qs.set("identifier", p.identifiers.join(","));
    } else if (p.identifier) {
      qs.set("identifier", p.identifier);
    }
    Object.entries(p).forEach(([k, v]) => {
      if (k === "identifiers" || v === undefined || v === null || v === "") return;
      qs.set(k, String(v));
    });
    return get<{ mode: string; source?: string; points: TelemetryPoint[] }>(`/devices/${id}/telemetry?${qs.toString()}`);
  },
  compare: (b: { deviceIds: number[]; identifiers: string[]; from?: string; to?: string; interval?: string }) => {
    return post<{
      interval: string;
      series: { deviceId: number; deviceKey: string; identifier: string; points: { t: string; v: number | null }[] }[];
    }>(`/telemetry/compare`, b);
  },
  events: (id: number, opts?: { q?: string; limit?: number }) => {
    const qs = new URLSearchParams();
    if (opts?.q) qs.set("q", opts.q);
    if (opts?.limit) qs.set("limit", String(opts.limit));
    const q = qs.toString();
    return get<DeviceEvent[]>(`/devices/${id}/events${q ? `?${q}` : ""}`);
  },
  downlinks: (id: number) => get<DeviceDownlinkLog[]>(`/devices/${id}/downlinks`),
  timeline: (id: number) => get<TimelineItem[]>(`/devices/${id}/timeline`),
  command: (id: number, b: { kind?: string; identifier?: string; payload?: unknown }) =>
    post<{ ok: boolean }>(`/devices/${id}/command`, b),
  peer: (id: number, b: { target?: string; broadcast?: boolean; payload?: unknown }) =>
    post<{ sent: number }>(`/devices/${id}/peer`, b),

  batchCommand: (
    projectId: number,
    b: {
      deviceIds?: number[];
      groupId?: number;
      action?: string;
      kind?: string;
      identifier?: string;
      payload?: unknown;
      desired?: Record<string, unknown>;
    },
  ) =>
    post<{ total: number; succeeded: number; failed: number; results: { deviceId: number; ok: boolean; error?: string }[] }>(
      `/projects/${projectId}/devices/batch`,
      b,
    ),

  // Device groups
  listDeviceGroups: (projectId: number) => get<DeviceGroup[]>(`/projects/${projectId}/device-groups`),
  createDeviceGroup: (projectId: number, b: { name: string; description?: string }) =>
    post<DeviceGroup>(`/projects/${projectId}/device-groups`, b),
  deleteDeviceGroup: (id: number) => del<{ ok: boolean }>(`/device-groups/${id}`),
  listGroupDevices: (id: number) => get<Device[]>(`/device-groups/${id}/devices`),
  addGroupDevices: (id: number, deviceIds: number[]) =>
    post<{ added: number }>(`/device-groups/${id}/devices`, { deviceIds }),
  removeGroupDevice: (id: number, deviceId: number) =>
    del<{ ok: boolean }>(`/device-groups/${id}/devices/${deviceId}`),

  getShadow: (id: number) => get<DeviceShadow>(`/devices/${id}/shadow`),
  patchShadowDesired: (id: number, desired: Record<string, unknown>) =>
    patch<DeviceShadow>(`/devices/${id}/shadow/desired`, desired),
  clearShadowDesired: (id: number) => del<DeviceShadow>(`/devices/${id}/shadow/desired`),
  getShadowHistory: (id: number) => get<ShadowLog[]>(`/devices/${id}/shadow/history?limit=50`),

  listRules: (projectId: number) => get<Rule[]>(`/projects/${projectId}/rules`),
  createRule: (projectId: number, b: Partial<Rule>) => post<Rule>(`/projects/${projectId}/rules`, b),
  updateRule: (id: number, b: Partial<Rule>) => put<Rule>(`/rules/${id}`, b),
  deleteRule: (id: number) => del<{ ok: boolean }>(`/rules/${id}`),
  toggleRule: (id: number) => post<Rule>(`/rules/${id}/toggle`),
  ruleLogs: (id: number) => get<RuleLog[]>(`/rules/${id}/logs`),
  validateRule: (condition: string) => post<{ valid: boolean; error?: string }>("/rules/validate", { condition }),
  ruleReference: () => get<RuleReference>("/rules/reference"),

  // Firmware / OTA
  listFirmwares: (productId: number) => get<Firmware[]>(`/products/${productId}/firmwares`),
  deleteFirmware: (id: number) => del<{ ok: boolean }>(`/firmwares/${id}`),
  listOtaTasks: (projectId: number) => get<OTATask[]>(`/projects/${projectId}/ota-tasks`),
  getOtaTask: (id: number) => get<OTATask>(`/ota-tasks/${id}`),
  createOtaTask: (
    projectId: number,
    body: {
      firmwareId: number;
      name?: string;
      deviceIds?: number[];
      workspaceId?: number;
      productId?: number;
      batchSize?: number;
      haltOnFailure?: boolean;
    },
  ) => post<OTATask>(`/projects/${projectId}/ota-tasks`, body),
  cancelOtaTask: (id: number) => post<{ ok: boolean }>(`/ota-tasks/${id}/cancel`),
  rollbackOtaTask: (id: number) => post<OTATask>(`/ota-tasks/${id}/rollback`),

  // Notification channels
  listChannels: (projectId: number) => get<NotifyChannel[]>(`/projects/${projectId}/channels`),
  createChannel: (projectId: number, b: Partial<NotifyChannel>) =>
    post<NotifyChannel>(`/projects/${projectId}/channels`, b),
  updateChannel: (id: number, b: Partial<NotifyChannel>) => put<NotifyChannel>(`/channels/${id}`, b),
  deleteChannel: (id: number) => del<{ ok: boolean }>(`/channels/${id}`),
  testChannel: (id: number) => post<{ ok: boolean }>(`/channels/${id}/test`),
  channelLogs: (id: number) => get<NotificationLog[]>(`/channels/${id}/logs`),
  listAlerts: (projectId: number, opts?: { status?: string; deviceId?: number; limit?: number }) => {
    const qs = new URLSearchParams();
    if (opts?.status) qs.set("status", opts.status);
    if (opts?.deviceId) qs.set("deviceId", String(opts.deviceId));
    if (opts?.limit) qs.set("limit", String(opts.limit));
    const q = qs.toString();
    return get<{ items: Alert[]; counts: Record<string, number> }>(
      `/projects/${projectId}/alerts${q ? `?${q}` : ""}`,
    );
  },
  listGeofences: (projectId: number, workspaceId = 0) =>
    get<Geofence[]>(`/projects/${projectId}/geofences${workspaceId ? `?workspaceId=${workspaceId}` : ""}`),
  createGeofence: (projectId: number, b: { name: string; polygon: Record<string, unknown>; workspaceId?: number; enabled?: boolean }) =>
    post<Geofence>(`/projects/${projectId}/geofences`, b),
  updateGeofence: (id: number, b: { name: string; polygon: Record<string, unknown>; workspaceId?: number; enabled?: boolean }) =>
    put<Geofence>(`/geofences/${id}`, b),
  deleteGeofence: (id: number) => del<{ ok: boolean }>(`/geofences/${id}`),
  devicesGeoJson: (projectId: number) =>
    get<{ type: string; features: Record<string, unknown>[] }>(`/projects/${projectId}/devices/geojson`),
  listDashboards: (projectId: number) => get<DashboardBoard[]>(`/projects/${projectId}/dashboards`),
  getDashboard: (id: number) => get<DashboardBoard>(`/dashboards/${id}`),
  putDashboard: (id: number, body: { panels: DashboardPanel[]; name?: string }) =>
    put<{ ok: boolean; panels: number; name: string }>(`/dashboards/${id}`, body),
  createDashboard: (projectId: number, name: string, workspaceId = 0) =>
    post<DashboardBoard>(`/projects/${projectId}/dashboards`, { name, workspaceId }),
  deleteDashboard: (id: number) => del<{ ok: boolean }>(`/dashboards/${id}`),
  ackAlert: (id: number) => post<{ ok: boolean }>(`/alerts/${id}/ack`),
  resolveAlert: (id: number, reason?: string) =>
    post<{ ok: boolean }>(`/alerts/${id}/resolve${reason ? `?reason=${encodeURIComponent(reason)}` : ""}`),

};

// --- Administration (isolated audience) ----------------------------------
const ADMIN_TOKEN_KEY = "aiiot_admin_token";

export function getAdminToken(): string | null {
  return localStorage.getItem(ADMIN_TOKEN_KEY);
}

export function setAdminToken(token: string | null) {
  if (token) localStorage.setItem(ADMIN_TOKEN_KEY, token);
  else localStorage.removeItem(ADMIN_TOKEN_KEY);
}

async function adminRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const token = getAdminToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(`${BASE}/admin${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 401 && path !== "/auth/login") {
    setAdminToken(null);
    if (!window.location.pathname.startsWith("/admin/login")) window.location.href = "/admin/login";
    throw new Error("unauthorized");
  }
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error(data?.error || `request failed (${res.status})`);
  return data as T;
}

export interface AdminUser {
  id: number;
  username: string;
  displayName?: string;
  status: string;
}

export const adminApi = {
  login: (username: string, password: string, totpCode?: string) =>
    adminRequest<{ token: string; admin: AdminUser }>("POST", "/auth/login", {
      username, password, ...(totpCode ? { totpCode } : {}),
    }),
  me: () => adminRequest<AdminUser>("GET", "/auth/me"),
  changePassword: (oldPassword: string, newPassword: string) =>
    adminRequest<{ ok: boolean }>("POST", "/auth/password", { oldPassword, newPassword }),
  totpStatus: () => adminRequest<{ enabled: boolean }>("GET", "/auth/totp/status"),
  totpSetup: () =>
    adminRequest<{ secret: string; otpauthUrl: string; period: number; enabled: boolean }>(
      "POST", "/auth/totp/setup", {}),
  totpEnable: (code: string) => adminRequest<{ enabled: boolean }>("POST", "/auth/totp/enable", { code }),
  totpDisable: (code: string) => adminRequest<{ enabled: boolean }>("POST", "/auth/totp/disable", { code }),
  listGateways: () => adminRequest<GatewayInstance[]>("GET", "/gateways"),
  deregisterGateway: (instanceId: string) =>
    adminRequest<{ ok: boolean }>("DELETE", `/gateways/${encodeURIComponent(instanceId)}`),
  getStorage: () => adminRequest<StorageInfo>("GET", "/storage"),
  updateRetention: (days: number) => adminRequest<{ retentionDays: number }>("PUT", "/storage/retention", { days }),
  natsStats: () => adminRequest<NATSStats>("GET", "/nats-stats"),
  refreshRollup: () => adminRequest<{ ok: boolean; rollup: RollupInfo }>("POST", "/storage/rollup/refresh"),
  dropPartition: (name: string) => adminRequest<{ ok: boolean }>("DELETE", `/storage/partitions/${name}`),
  listAuditLogs: (filters?: { path?: string; projectId?: number; limit?: number }) => {
    const qs = new URLSearchParams();
    if (filters?.path) qs.set("path", filters.path);
    if (filters?.projectId) qs.set("projectId", String(filters.projectId));
    if (filters?.limit) qs.set("limit", String(filters.limit));
    const q = qs.toString();
    return adminRequest<AuditLog[]>("GET", `/audit-logs${q ? `?${q}` : ""}`);
  },
};

export async function uploadFirmware(
  productId: number,
  file: File,
  meta: { version: string; name?: string; description?: string },
): Promise<Firmware> {
  const fd = new FormData();
  fd.append("file", file);
  fd.append("version", meta.version);
  if (meta.name) fd.append("name", meta.name);
  if (meta.description) fd.append("description", meta.description);
  const token = getToken();
  const res = await fetch(`${BASE}/products/${productId}/firmwares`, {
    method: "POST",
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body: fd,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error(data?.error || `upload failed (${res.status})`);
  return data as Firmware;
}
