import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  ArrowDown,
  ArrowUp,
  Boxes,
  CheckCircle2,
  Circle,
  Cpu,
  FolderKanban,
  LayoutDashboard,
  Pencil,
  Plus,
  Radio,
  Trash2,
} from "lucide-react";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { api, type DashboardPanel, type Device, type LatestValue } from "@/lib/api";
import { toast } from "sonner";
import { useEventStream } from "@/lib/useEventStream";
import { useProjectRole } from "@/lib/useProjectRole";
import { useI18n } from "@/lib/i18n";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { formatNumber, formatTime, sinceText } from "@/lib/utils";

const SERIES_COLORS = [
  "hsl(var(--primary))",
  "#f59e0b",
  "#10b981",
  "#8b5cf6",
  "#ef4444",
  "#06b6d4",
  "#ec4899",
  "#84cc16",
];

function statValue(v: LatestValue | undefined): string {
  if (!v) return "-";
  if (v.numValue != null) return formatNumber(v.numValue, 2);
  if (v.boolValue != null) return v.boolValue ? "true" : "false";
  if (v.strValue != null) return v.strValue;
  if (v.jsonValue != null) return JSON.stringify(v.jsonValue);
  return "-";
}

function PanelShell({
  panel,
  editing,
  onRemove,
  onUp,
  onDown,
  children,
}: {
  panel: DashboardPanel;
  editing: boolean;
  onRemove: () => void;
  onUp: () => void;
  onDown: () => void;
  children: React.ReactNode;
}) {
  return (
    <Card className="h-full">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{panel.title || panel.type}</CardTitle>
        {editing && (
          <div className="flex items-center gap-1">
            <Button variant="ghost" size="icon" onClick={onUp} title="up">
              <ArrowUp className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" onClick={onDown} title="down">
              <ArrowDown className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" onClick={onRemove} title="remove">
              <Trash2 className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        )}
      </CardHeader>
      <CardContent className="pb-4">{children}</CardContent>
    </Card>
  );
}

function valueOfLatest(v: LatestValue | undefined): string {
  return statValue(v);
}

function StatPanel({ panel, devices }: { panel: DashboardPanel; devices: Device[] }) {
  const deviceId = Number(panel.config?.deviceId ?? 0);
  const identifier = String(panel.config?.identifier ?? "");
  const dev = devices.find((d) => d.id === deviceId);
  const latest = useQuery({
    queryKey: ["latest", deviceId],
    queryFn: () => api.latest(deviceId),
    enabled: !!deviceId,
    refetchInterval: 10000,
  });
  const val = (latest.data ?? []).find((v) => v.identifier === identifier);
  return (
    <div>
      <div className="text-3xl font-bold tabular-nums">
        {valueOfLatest(val)}
        {panel.config?.unit ? (
          <span className="ml-1 text-base font-normal text-muted-foreground">{String(panel.config.unit)}</span>
        ) : null}
      </div>
      <div className="mt-1 text-xs text-muted-foreground">
        {dev?.name ?? `#${deviceId}`} · {identifier}
        {val ? ` · ${sinceText(val.updatedAt)}` : ""}
      </div>
    </div>
  );
}

function TrendPanel({ panel, devices }: { panel: DashboardPanel; devices: Device[] }) {
  const singleId = Number(panel.config?.deviceId ?? 0);
  const deviceIds =
    (panel.config?.deviceIds as number[] | undefined)?.filter((d) => d > 0) ?? [singleId];
  const identifiers =
    (panel.config?.identifiers as string[] | undefined)?.filter((x) => x) ??
    [String(panel.config?.identifier ?? "")];
  const range = String(panel.config?.range ?? "24h");
  const dev = devices.find((d) => d.id === deviceIds[0]);
  const key = JSON.stringify([deviceIds, identifiers, range]);
  const compare = useQuery({
    queryKey: ["compare", key],
    queryFn: () => {
      const hours = range === "7d" ? 168 : range === "1h" ? 1 : 24;
      return api.compare({
        deviceIds,
        identifiers,
        interval: range === "7d" ? "1h" : range === "1h" ? "1m" : "5m",
        from: new Date(Date.now() - hours * 3600000).toISOString(),
        to: new Date().toISOString(),
      });
    },
    enabled: deviceIds.length > 0 && identifiers.length > 0,
  });
  const label = (s: { deviceKey: string; identifier: string }) => `${s.deviceKey}/${s.identifier}`;
  const chart = useMemo(() => {
    const rows = new Map<string, Record<string, unknown>>();
    for (const s of compare.data?.series ?? []) {
      const k = label(s);
      for (const p of s.points) {
        if (p.v == null) continue;
        const t = new Date(p.t).toLocaleString();
        if (!rows.has(t)) rows.set(t, { t });
        rows.get(t)![k] = p.v;
      }
    }
    const order = [...rows.keys()].sort();
    return { rows: order.map((t) => rows.get(t)!), series: (compare.data?.series ?? []).map(label) };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [compare.data]);
  const header =
    deviceIds.length > 1 || identifiers.length > 1
      ? `${deviceIds.length} devices · ${identifiers.join(", ")} · ${range}`
      : `${dev?.name ?? `#${deviceIds[0]}`} · ${identifiers[0]} · ${range}`;
  return (
    <div>
      <div className="mb-2 text-xs text-muted-foreground">{header}</div>
      {chart.rows.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">no data</p>
      ) : (
        <ResponsiveContainer width="100%" height={200}>
          <LineChart data={chart.rows}>
            <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
            <XAxis dataKey="t" fontSize={10} tickCount={5} />
            <YAxis fontSize={10} width={44} />
            <Tooltip />
            {chart.series.map((k, i) => (
              <Line
                key={k}
                type="monotone"
                dataKey={k}
                stroke={SERIES_COLORS[i % SERIES_COLORS.length]}
                dot={false}
                strokeWidth={2}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      )}
    </div>
  );
}

function AlertsPanel({ projectId }: { projectId: number }) {
  const alerts = useQuery({
    queryKey: ["alerts", projectId, "dash"],
    queryFn: () => api.listAlerts(projectId, { limit: 200 }),
    refetchInterval: 15000,
  });
  const counts = alerts.data?.counts ?? {};
  return (
    <div className="grid grid-cols-3 gap-2">
      {(["firing", "acknowledged", "resolved"] as const).map((k) => (
        <div key={k} className="rounded-md border p-2 text-center">
          <div className="text-xl font-bold tabular-nums">{counts[k] ?? 0}</div>
          <div className="text-[11px] text-muted-foreground">{k}</div>
        </div>
      ))}
    </div>
  );
}

// MapPanel renders device coordinates (tags.lat / tags.lng) as a plain SVG
// scatter plot — no map-tile dependency. Devices without coordinates are
// listed beside the plot.
function MapPanel({ devices }: { devices: Device[] }) {
  const geo = devices.filter((d) => {
    const t = d.tags as Record<string, unknown> | undefined;
    return t && typeof t.lat === "number" && typeof t.lng === "number";
  });
  const W = 420, H = 240, PAD = 18;
  const lats = geo.map((d) => (d.tags as Record<string, number>).lat);
  const lngs = geo.map((d) => (d.tags as Record<string, number>).lng);
  const minLat = Math.min(...lats, 0), maxLat = Math.max(...lats, 1);
  const minLng = Math.min(...lngs, 0), maxLng = Math.max(...lngs, 1);
  const x = (lng: number) => PAD + ((lng - minLng) / (maxLng - minLng || 1)) * (W - 2 * PAD);
  const y = (lat: number) => PAD + ((maxLat - lat) / (maxLat - minLat || 1)) * (H - 2 * PAD);
  const other = devices.filter((d) => !geo.includes(d));
  return (
    <div>
      <div className="mb-2 flex items-center justify-between text-xs text-muted-foreground">
        <span>{geo.length} located · {devices.length} total</span>
      </div>
      {geo.length > 0 ? (
        <svg viewBox={`0 0 ${W} ${H}`} className="w-full rounded border bg-background">
          {geo.map((d, i) => (
            <g key={d.id}>
              <circle
                cx={x((d.tags as Record<string, number>).lng)}
                cy={y((d.tags as Record<string, number>).lat)}
                r={6}
                className="fill-primary/80"
              >
                <title>{`${d.name} (${(d.tags as Record<string, number>).lat}, ${(d.tags as Record<string, number>).lng})`}</title>
              </circle>
            </g>
          ))}
          <text x={PAD} y={H - 4} fontSize={9} fill="currentColor" opacity={0.6}>
            lat {minLat.toFixed(1)}–{maxLat.toFixed(1)} · lng {minLng.toFixed(1)}–{maxLng.toFixed(1)}
          </text>
        </svg>
      ) : (
        <p className="py-6 text-center text-sm text-muted-foreground">
          Set device tags lat/lng to plot positions
        </p>
      )}
      {other.length > 0 && (
        <div className="mt-2 space-y-1">
          {other.slice(0, 5).map((d) => (
            <div key={d.id} className="flex items-center justify-between text-xs">
              <span className="min-w-0 truncate">{d.name}</span>
              <Badge variant={d.online ? "success" : "secondary"}>{d.online ? "online" : "offline"}</Badge>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function DevicesPanel({ devices }: { devices: Device[] }) {
  const online = devices.filter((d) => d.online).length;
  return (
    <div>
      <div className="mb-2 flex items-center gap-4 text-xs text-muted-foreground">
        <span>
          {t_dash_online} <span className="font-semibold text-emerald-600">{online}</span>
        </span>
        <span>
          total <span className="font-semibold">{devices.length}</span>
        </span>
      </div>
      <div className="space-y-1">
        {devices.slice(0, 6).map((d) => (
          <div key={d.id} className="flex items-center justify-between text-xs">
            <span className="min-w-0 truncate">{d.name}</span>
            <Badge variant={d.online ? "success" : "secondary"}>{d.online ? "online" : "offline"}</Badge>
          </div>
        ))}
      </div>
    </div>
  );
}

const t_dash_online = "online";

export default function DashboardPage() {
  const { t } = useI18n();
  const qc = useQueryClient();
  const projects = useQuery({ queryKey: ["projects"], queryFn: api.listProjects });
  const products = useQuery({ queryKey: ["products"], queryFn: () => api.listProducts() });

  const projectId = useMemo(() => {
    const stored = Number(localStorage.getItem("aiiot_project") || 0);
    if (stored && (projects.data ?? []).some((p) => p.id === stored)) return stored;
    return projects.data?.[0]?.id ?? 0;
  }, [projects.data]);
  const { canWrite } = useProjectRole(projectId);

  const workspaces = useQuery({
    queryKey: ["workspaces", projectId],
    queryFn: () => api.listWorkspaces(projectId),
    enabled: !!projectId,
  });
  const devices = useQuery({
    queryKey: ["devices", projectId, "dashboard"],
    queryFn: () => api.listDevices(projectId),
    enabled: !!projectId,
  });
  const boards = useQuery({
    queryKey: ["dashboards", projectId],
    queryFn: () => api.listDashboards(projectId),
    enabled: !!projectId,
  });
  const [boardId, setBoardId] = useState<number>(() =>
    Number(localStorage.getItem(`aiiot_dashboard_${projectId}`) || 0),
  );
  const effectiveBoardId = boards.data?.length ? boardId || boards.data[0].id : 0;
  useEffect(() => {
    if (effectiveBoardId) localStorage.setItem(`aiiot_dashboard_${projectId}`, String(effectiveBoardId));
  }, [projectId, effectiveBoardId]);
  const board = useQuery({
    queryKey: ["dashboard", effectiveBoardId],
    queryFn: () => api.getDashboard(effectiveBoardId),
    enabled: !!effectiveBoardId,
  });
  const { events, connected } = useEventStream(projectId || undefined);

  const [editing, setEditing] = useState(false);
  const [panels, setPanels] = useState<DashboardPanel[]>([]);
  const [addOpen, setAddOpen] = useState(false);
  const [draft, setDraft] = useState<DashboardPanel>({ type: "stat", title: "", config: {} });

  const save = useMutation({
    mutationFn: () => api.putDashboard(effectiveBoardId, { panels }),
    onSuccess: () => {
      toast.success("Dashboard saved");
      setEditing(false);
      void qc.invalidateQueries({ queryKey: ["dashboard", effectiveBoardId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const [boardOpen, setBoardOpen] = useState(false);
  const [boardName, setBoardName] = useState("");
  const [boardWs, setBoardWs] = useState(0);
  const createBoard = useMutation({
    mutationFn: () => api.createDashboard(projectId || 0, boardName.trim() || "看板", boardWs),
    onSuccess: (b) => {
      toast.success("Dashboard created");
      setBoardOpen(false);
      setBoardName("");
      setBoardWs(0);
      setBoardId(b.id);
      void qc.invalidateQueries({ queryKey: ["dashboards", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const removeBoard = useMutation({
    mutationFn: () => api.deleteDashboard(effectiveBoardId),
    onSuccess: () => {
      toast.success("Dashboard deleted");
      setBoardId(0);
      void qc.invalidateQueries({ queryKey: ["dashboards", projectId] });
      void qc.invalidateQueries({ queryKey: ["dashboard", effectiveBoardId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  function startEdit() {
    setPanels(board.data?.panels ?? []);
    setEditing(true);
  }
  function move(i: number, dir: -1 | 1) {
    setPanels((prev) => {
      const next = [...prev];
      const j = i + dir;
      if (j < 0 || j >= next.length) return prev;
      [next[i], next[j]] = [next[j], next[i]];
      return next;
    });
  }
  function addPanel() {
    setPanels((prev) => [...prev, draft]);
    setAddOpen(false);
    setDraft({ type: "stat", title: "", config: {} });
  }

  const steps = [
    { label: t("check.project"), done: (projects.data?.length ?? 0) > 0, to: "/projects", cta: t("nav.projects") },
    { label: t("check.workspace"), done: (workspaces.data?.length ?? 0) > 0, to: `/projects/${projectId}`, cta: t("tab.overview") },
    { label: t("check.product"), done: (products.data?.length ?? 0) > 0, to: "/products", cta: t("nav.products") },
    { label: t("check.device"), done: (devices.data?.length ?? 0) > 0, to: `/projects/${projectId}/devices`, cta: t("tab.devices") },
  ];
  const doneCount = steps.filter((s) => s.done).length;

  const stats = [
    { label: t("dash.projects"), value: projects.data?.length ?? 0, icon: FolderKanban, to: "/projects" },
    { label: t("nav.products"), value: products.data?.length ?? 0, icon: Boxes, to: "/products" },
    { label: t("tab.devices"), value: devices.data?.length ?? 0, icon: Cpu, to: `/projects/${projectId}/devices` },
  ];
  const shown = editing ? panels : board.data?.panels ?? [];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold tracking-tight whitespace-nowrap">{t("dash.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("dash.subtitle")}</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {(boards.data?.length ?? 0) > 0 && !editing && (
            <div className="flex items-center gap-1">
              <Select value={String(effectiveBoardId)} onValueChange={(v) => setBoardId(Number(v))}>
                <SelectTrigger className="w-52">
                  <SelectValue placeholder="select dashboard" />
                </SelectTrigger>
                <SelectContent>
                  {(boards.data ?? []).map((b) => {
                    const ws = b.workspaceId ? (workspaces.data ?? []).find((w) => w.id === b.workspaceId) : undefined;
                    return (
                      <SelectItem key={b.id} value={String(b.id)}>
                        {b.name || `#${b.id}`}
                        <span className="ml-1 text-xs text-muted-foreground">
                          · {ws ? ws.name : "项目级"}
                        </span>
                      </SelectItem>
                    );
                  })}
                </SelectContent>
              </Select>
              {canWrite && (
                <>
                  <Button size="sm" variant="ghost" onClick={() => setBoardOpen(true)} title="New dashboard">
                    <Plus className="h-4 w-4" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    title="Delete dashboard"
                    onClick={() => {
                      if (editing || window.confirm("Delete this dashboard?")) removeBoard.mutate();
                    }}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </>
              )}
            </div>
          )}
          <Badge variant={connected ? "success" : "secondary"}>
            <Radio className="mr-1 h-3 w-3" /> {connected ? t("dash.live") : t("dash.offline")}
          </Badge>
          {canWrite &&
            (editing ? (
              <>
                <Button size="sm" onClick={() => setAddOpen(true)}>
                  <Plus className="h-4 w-4" /> {t("dash.addPanel")}
                </Button>
                <Button size="sm" variant="outline" onClick={() => save.mutate()} disabled={save.isPending}>
                  {t("common.save")}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>
                  {t("common.cancel")}
                </Button>
              </>
            ) : (
              <Button size="sm" variant="outline" onClick={startEdit}>
                <Pencil className="h-4 w-4" /> {t("dash.edit")}
              </Button>
            ))}
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        {stats.map((s) => (
          <Link key={s.label} to={s.to}>
            <Card className="transition-colors hover:border-primary/50">
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium text-muted-foreground">{s.label}</CardTitle>
                <s.icon className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-3xl font-bold">{s.value}</div>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>

      {shown.length > 0 ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {shown.map((p, i) => (
            <div key={i} className={p.type === "trend" ? "xl:col-span-2" : ""}>
              <PanelShell
                panel={p}
                editing={editing}
                onRemove={() => setPanels((prev) => prev.filter((_, idx) => idx !== i))}
                onUp={() => move(i, -1)}
                onDown={() => move(i, 1)}
              >
                {p.type === "stat" && <StatPanel panel={p} devices={devices.data ?? []} />}
                {p.type === "trend" && <TrendPanel panel={p} devices={devices.data ?? []} />}
                {p.type === "map" && <MapPanel devices={devices.data ?? []} />}
                {p.type === "alerts" && <AlertsPanel projectId={projectId} />}
                {p.type === "devices" && <DevicesPanel devices={devices.data ?? []} />}
                {p.type === "text" && (
                  <p className="whitespace-pre-wrap text-sm text-muted-foreground">
                    {String(p.config?.text ?? "")}
                  </p>
                )}
              </PanelShell>
            </div>
          ))}
        </div>
      ) : (
        canWrite && (
          <Card>
            <CardContent className="flex flex-col items-center gap-3 py-10 text-center">
              <LayoutDashboard className="h-8 w-8 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">{t("dash.blank")}</p>
              <Button size="sm" onClick={startEdit}>
                <Plus className="h-4 w-4" /> {t("dash.addPanel")}
              </Button>
            </CardContent>
          </Card>
        )
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("dash.gettingStarted")}</CardTitle>
            <div className="text-sm text-muted-foreground">
              {t("dash.stepsComplete", { done: doneCount, total: steps.length })}
            </div>
          </CardHeader>
          <CardContent className="space-y-1">
            {steps.map((s) => (
              <Link
                key={s.label}
                to={s.to}
                className="flex items-center gap-3 rounded-md px-2 py-2 text-sm hover:bg-accent"
              >
                {s.done ? (
                  <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                ) : (
                  <Circle className="h-4 w-4 text-muted-foreground" />
                )}
                <span className={s.done ? "text-muted-foreground line-through" : "font-medium"}>{s.label}</span>
                {!s.done && <span className="ml-auto text-xs text-primary">{s.cta} →</span>}
              </Link>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="flex items-center gap-2 text-base">
              <Activity className="h-4 w-4" /> {t("dash.liveActivity")}
            </CardTitle>
            <Badge variant={connected ? "success" : "secondary"}>{connected ? t("dash.live") : t("dash.offline")}</Badge>
          </CardHeader>
          <CardContent className="space-y-2">
            {events.length === 0 && <p className="text-sm text-muted-foreground">{t("dash.waiting")}</p>}
            {events.slice(0, 10).map((e, i) => (
              <div key={i} className="flex items-center justify-between rounded-md border p-2 text-xs">
                <div className="min-w-0">
                  <span className="font-mono">{e.deviceKey}</span>
                  {e.type === "telemetry" ? (
                    <span className="ml-2 text-muted-foreground">
                      {e.identifier} = {JSON.stringify(e.value)}
                    </span>
                  ) : (
                    <span className="ml-2 text-muted-foreground">{e.online ? "online" : "offline"}</span>
                  )}
                </div>
                <span className="ml-2 whitespace-nowrap text-muted-foreground">{formatTime(e.time)}</span>
              </div>
            ))}
          </CardContent>
        </Card>
      </div>

      <Dialog open={boardOpen} onOpenChange={setBoardOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New dashboard</DialogTitle>
            <DialogDescription>Create another dashboard for this project (project-scoped).</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-2">
              <Label>Name</Label>
              <Input
                value={boardName}
                onChange={(e) => setBoardName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && boardName.trim()) createBoard.mutate();
                }}
                placeholder="e.g. 产线大屏"
              />
            </div>
            <div className="space-y-2">
              <Label>Scope</Label>
              <Select value={String(boardWs)} onValueChange={(v) => setBoardWs(Number(v))}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="0">项目级</SelectItem>
                  {(workspaces.data ?? []).map((w) => (
                    <SelectItem key={w.id} value={String(w.id)}>
                      工作区：{w.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setBoardOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button onClick={() => createBoard.mutate()} disabled={createBoard.isPending || !boardName.trim()}>
                {t("common.save")}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("dash.addPanel")}</DialogTitle>
            <DialogDescription>{t("dash.addPanelDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("dash.panelTitle")}</Label>
              <Input value={draft.title ?? ""} onChange={(e) => setDraft({ ...draft, title: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t("dash.panelType")}</Label>
              <Select value={draft.type} onValueChange={(v) => setDraft({ ...draft, type: v as DashboardPanel["type"], config: {} })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="stat">stat</SelectItem>
                  <SelectItem value="trend">trend</SelectItem>
                  <SelectItem value="alerts">alerts</SelectItem>
                  <SelectItem value="devices">devices</SelectItem>
                  <SelectItem value="map">map</SelectItem>
                  <SelectItem value="text">text</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {(draft.type === "stat" || draft.type === "trend") && (
              <>
                <div className="space-y-2">
                  <Label>{t("dash.device")}</Label>
                  <Select
                    value={String(draft.config?.deviceId ?? "")}
                    onValueChange={(v) => setDraft({ ...draft, config: { ...draft.config, deviceId: Number(v) } })}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="select device" />
                    </SelectTrigger>
                    <SelectContent>
                      {(devices.data ?? []).map((d) => (
                        <SelectItem key={d.id} value={String(d.id)}>
                          {d.name} ({d.key})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t("dash.identifier")}</Label>
                  <Input
                    value={String(draft.config?.identifier ?? "")}
                    onChange={(e) => setDraft({ ...draft, config: { ...draft.config, identifier: e.target.value } })}
                  />
                </div>
                {draft.type === "stat" && (
                  <div className="space-y-2">
                    <Label>{t("product.unit")}</Label>
                    <Input
                      value={String(draft.config?.unit ?? "")}
                      onChange={(e) => setDraft({ ...draft, config: { ...draft.config, unit: e.target.value } })}
                    />
                  </div>
                )}
                {draft.type === "trend" && (
                  <>
                    <div className="space-y-2">
                      <Label>
                        {t("dash.range")}
                        <span className="ml-1 text-xs font-normal text-muted-foreground">(compare: pick more)</span>
                      </Label>
                    </div>
                    <div className="space-y-2">
                      <Label className="flex items-center gap-2 text-xs text-muted-foreground">
                        compare devices
                      </Label>
                      <div className="flex max-h-28 flex-wrap gap-x-4 gap-y-1 overflow-auto rounded-md border p-2">
                        {(devices.data ?? []).map((d) => (
                          <label key={d.id} className="flex cursor-pointer items-center gap-1.5 text-xs">
                            <input
                              type="checkbox"
                              className="accent-primary"
                              checked={((draft.config?.deviceIds as number[] | undefined) ?? [draft.config?.deviceId])
                                .includes(d.id)}
                              onChange={(e) => {
                                const cur = (draft.config?.deviceIds as number[] | undefined) ??
                                  [Number(draft.config?.deviceId ?? 0)].filter((x) => x > 0);
                                const next = e.target.checked
                                  ? [...cur.filter((x) => x !== d.id), d.id]
                                  : cur.filter((x) => x !== d.id);
                                setDraft({ ...draft, config: { ...draft.config, deviceIds: next } });
                              }}
                            />
                            {d.name}
                          </label>
                        ))}
                      </div>
                    </div>
                    <div className="space-y-2">
                      <Label className="text-xs text-muted-foreground">compare identifiers (comma)</Label>
                      <Input
                        defaultValue={((draft.config?.identifiers as string[] | undefined) ?? [String(draft.config?.identifier ?? "")])
                          .join(",")}
                        onChange={(e) =>
                          setDraft({ ...draft, config: { ...draft.config, identifiers: e.target.value.split(",").map((x) => x.trim()).filter(Boolean) } })
                        }
                      />
                    </div>
                  </>
                )}
                {draft.type === "trend" && (
                  <div className="space-y-2">
                    <Label>{t("dash.range")}</Label>
                    <Select
                      value={String(draft.config?.range ?? "24h")}
                      onValueChange={(v) => setDraft({ ...draft, config: { ...draft.config, range: v } })}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="1h">1h</SelectItem>
                        <SelectItem value="24h">24h</SelectItem>
                        <SelectItem value="7d">7d</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                )}
              </>
            )}
            {draft.type === "text" && (
              <div className="space-y-2">
                <Label>{t("dash.text")}</Label>
                <Textarea
                  value={String(draft.config?.text ?? "")}
                  onChange={(e) => setDraft({ ...draft, config: { ...draft.config, text: e.target.value } })}
                />
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={addPanel}>{t("dash.add")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
