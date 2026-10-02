import { useMemo } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Activity, Boxes, CheckCircle2, Circle, Cpu, FolderKanban, Radio } from "lucide-react";
import { api } from "@/lib/api";
import { useEventStream } from "@/lib/useEventStream";
import { useI18n } from "@/lib/i18n";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { formatTime } from "@/lib/utils";

export default function DashboardPage() {
  const { t } = useI18n();
  const projects = useQuery({ queryKey: ["projects"], queryFn: api.listProjects });
  const products = useQuery({ queryKey: ["products"], queryFn: () => api.listProducts() });

  const projectId = useMemo(() => {
    const stored = Number(localStorage.getItem("aiiot_project") || 0);
    if (stored && (projects.data ?? []).some((p) => p.id === stored)) return stored;
    return projects.data?.[0]?.id ?? 0;
  }, [projects.data]);

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

  const { events, connected } = useEventStream(projectId || undefined);

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

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("dash.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("dash.subtitle")}</p>
        </div>
        <Badge variant={connected ? "success" : "secondary"}>
          <Radio className="mr-1 h-3 w-3" /> {connected ? t("dash.live") : t("dash.offline")}
        </Badge>
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
            {events.length === 0 && (
              <p className="text-sm text-muted-foreground">
                {t("dash.waiting")}
              </p>
            )}
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
    </div>
  );
}
