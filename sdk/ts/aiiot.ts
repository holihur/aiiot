// aiiot platform SDK for TypeScript/JavaScript (browser or Node 18+, no deps).
// Auth: login() -> store token, pass as Authorization: Bearer on all calls.
//
//   const aiiot = new AiiotClient("http://localhost:8080");
//   await aiiot.login("admin", "secret");
//   const devices = await aiiot.devices(1);
//   const rows = await aiiot.telemetry(devices[0].id, "temperature", "5m");

export interface Project { id: number; key: string; name: string; description?: string }
export interface Workspace { id: number; projectId: number; key: string; name: string }
export interface Product { id: number; key: string; name: string; protocol: string; status: string }
export interface ThingModelElement {
  type: "property" | "service" | "event";
  identifier: string;
  name?: string;
  dataType?: string;
  unit?: string;
  specs?: Record<string, unknown>;
}
export interface Device {
  id: number; projectId: number; workspaceId: number; productId: number;
  key: string; name: string; status: string; online: boolean; tags?: Record<string, unknown>;
}
export interface LatestValue {
  identifier: string; dataType: string;
  numValue?: number | null; boolValue?: boolean | null; strValue?: string | null;
  updatedAt: string;
}
export interface Point {
  bucket?: string; identifier?: string;
  avg?: number | null; min?: number | null; max?: number | null; count?: number;
}
export interface CompareSeries {
  deviceId: number; deviceKey: string; identifier: string;
  points: { t: string; v: number | null }[];
}

export class AiiotClient {
  token = "";

  constructor(
    private baseUrl = "http://localhost:8080",
    private timeoutMs = 15000,
  ) {}

  async login(username: string, password: string): Promise<string> {
    const data = await this.request<{ token: string }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    this.token = data.token;
    return this.token;
  }

  // --- projects / workspaces / products ------------------------------------
  projects = () => this.get<Project[]>("/api/v1/projects");
  createProject = (name: string, key = "", description = "") =>
    this.post<Project>("/api/v1/projects", { name, key, description });
  workspaces = (projectId: number) => this.get<Workspace[]>(`/api/v1/projects/${projectId}/workspaces`);
  products = () => this.get<Product[]>("/api/v1/products");
  createProduct = (name: string, protocol = "mqtt", key = "") =>
    this.post<Product>("/api/v1/products", { name, protocol, key });

  // --- device management ------------------------------------------------------
  devices = (projectId: number, query?: Record<string, string>) =>
    this.get<Device[]>(`/api/v1/projects/${projectId}/devices`, query);
  createDevice = (productId: number, name: string, workspaceId: number, key = "") =>
    this.post<{ device: Device; secret: string }>(`/api/v1/products/${productId}/devices`, {
      name, workspaceId, key,
    });
  device = (id: number) => this.get<Device>(`/api/v1/devices/${id}`);
  exportDevicesCsv = (productId: number) => this.raw("GET", `/api/v1/products/${productId}/devices/export`);
  importDevicesCsv = (productId: number, file: Blob) => {
    const fd = new FormData();
    fd.append("file", file);
    return this.request<{ created: number; skipped: number; failed: number; results: unknown[] }>(
      `/api/v1/products/${productId}/devices/import`, { method: "POST", body: fd, json: false });
  };

  // --- thing models ------------------------------------------------------------
  thingModel = (productId: number) =>
    this.get<{ version: string; status: string; elements: ThingModelElement[] }>(
      `/api/v1/products/${productId}/thing-model`);
  saveThingModel = (productId: number, elements: ThingModelElement[], version = "") =>
    this.put(`/api/v1/products/${productId}/thing-model`, { elements, version });
  publishThingModel = (productId: number) =>
    this.post<{ ok: boolean; snapshotId: number; version: string }>(
      `/api/v1/products/${productId}/thing-model/publish`, {});

  // --- certificates --------------------------------------------------------------
  issueCertificate = (deviceId: number) =>
    this.post<{ certPem: string; keyPem: string; caPem: string; serial: string; expiresAt: string }>(
      `/api/v1/devices/${deviceId}/certificate`, {});
  revokeCertificate = (deviceId: number) =>
    this.request(`/api/v1/devices/${deviceId}/certificate`, { method: "DELETE" });

  // --- telemetry -----------------------------------------------------------------
  latest = (deviceId: number) => this.get<LatestValue[]>(`/api/v1/devices/${deviceId}/latest`);
  telemetry = (
    deviceId: number,
    identifier: string,
    interval = "",
    from = "",
    to = "",
  ) => {
    const q: Record<string, string> = { identifier };
    if (interval) q.interval = interval;
    if (from) q.from = from;
    if (to) q.to = to;
    return this.get<{ mode: string; source?: string; points: Point[] }>(
      `/api/v1/devices/${deviceId}/telemetry`, q).then((d) => d.points);
  };
  compare = (deviceIds: number[], identifiers: string[], interval = "5m") =>
    this.post<{ interval: string; series: CompareSeries[] }>("/api/v1/telemetry/compare", {
      deviceIds, identifiers, interval,
    }).then((d) => d.series);
  exportTelemetryCsv = (
    deviceId: number,
    identifiers: string[],
    interval = "",
    from = "",
    to = "",
  ) => {
    const q: Record<string, string> = { identifiers: identifiers.join(",") };
    if (interval) q.interval = interval;
    if (from) q.from = from;
    if (to) q.to = to;
    return this.raw("GET", `/api/v1/devices/${deviceId}/telemetry/export`, q);
  };

  // --- alerts / channels ---------------------------------------------------------
  alerts = (projectId: number, query?: Record<string, string>) =>
    this.get(`/api/v1/projects/${projectId}/alerts`, query);
  channels = (projectId: number) => this.get(`/api/v1/projects/${projectId}/channels`);
  testChannel = (channelId: number, title = "aiiot SDK test", body = "Hello") =>
    this.post(`/api/v1/channels/${channelId}/test`, { title, body });

  // --- transport --------------------------------------------------------------------
  private get<T>(path: string, query?: Record<string, string>): Promise<T> {
    return this.request(path, { json: true, ...(query ? { query } : {}) });
  }
  private post<T>(path: string, body: unknown): Promise<T> {
    return this.request(path, { method: "POST", json: true, body: JSON.stringify(body) });
  }
  private put<T>(path: string, body: unknown): Promise<T> {
    return this.request(path, { method: "PUT", json: true, body: JSON.stringify(body) });
  }
  private raw(method = "GET", path: string, query?: Record<string, string>) {
    return this.request<ArrayBuffer>(path, { method, ...(query ? { query } : {}) }, true);
  }

  private async request<T>(
    path: string,
    opts: { method?: string; body?: BodyInit; json?: boolean; query?: Record<string, string> } = {},
    asBuffer = false,
  ): Promise<T> {
    let url = this.baseUrl + path;
    if (opts.query) {
      const qs = new URLSearchParams();
      for (const [k, v] of Object.entries(opts.query)) if (v) qs.set(k, v);
      url += `?${qs.toString()}`;
    }
    const headers: Record<string, string> = { ...(opts.json ? { "Content-Type": "application/json" } : {}) };
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), this.timeoutMs);
    try {
      const resp = await fetch(url, {
        method: opts.method ?? "GET",
        headers,
        body: opts.body,
        signal: ctrl.signal,
      });
      if (!resp.ok) {
        const text = await resp.text().catch(() => "");
        throw new Error(`${resp.status} ${resp.statusText}: ${text.slice(0, 200)}`);
      }
      return (asBuffer ? await resp.arrayBuffer() : await resp.json()) as T;
    } finally {
      clearTimeout(timer);
    }
  }
}