import { useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, KeyRound, Plug, RefreshCw, Send, Trash2, Wifi } from "lucide-react";
import { toast } from "sonner";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { api, type LatestValue, type TelemetryPoint } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { useEventStream } from "@/lib/useEventStream";
import { ConnectDialog } from "@/components/ConnectDialog";
import { ConfirmButton } from "@/components/ConfirmButton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { formatNumber, formatTime } from "@/lib/utils";

function valueOf(v: LatestValue): string {
  if (v.numValue != null) return formatNumber(v.numValue, 3);
  if (v.boolValue != null) return v.boolValue ? "true" : "false";
  if (v.strValue != null) return v.strValue;
  if (v.jsonValue != null) return JSON.stringify(v.jsonValue);
  return "-";
}

export default function DeviceDetailPage() {
  const deviceId = Number(useParams().deviceId);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { t } = useI18n();

  const device = useQuery({ queryKey: ["device", deviceId], queryFn: () => api.getDevice(deviceId) });
  const latest = useQuery({
    queryKey: ["latest", deviceId],
    queryFn: () => api.latest(deviceId),
    refetchInterval: 5000,
  });
  const events = useQuery({ queryKey: ["events", deviceId], queryFn: () => api.events(deviceId) });
  const shadow = useQuery({
    queryKey: ["shadow", deviceId],
    queryFn: () => api.getShadow(deviceId),
    refetchInterval: 5000,
  });
  const shadowHistory = useQuery({
    queryKey: ["shadowHistory", deviceId],
    queryFn: () => api.getShadowHistory(deviceId),
    refetchInterval: 5000,
  });

  // Live updates: refresh latest values / device state as events stream in.
  useEventStream(device.data?.projectId, (e) => {
    if (e.deviceId !== deviceId) return;
    void qc.invalidateQueries({ queryKey: ["latest", deviceId] });
    if (e.type === "lifecycle") {
      void qc.invalidateQueries({ queryKey: ["device", deviceId] });
    }
  });

  const [identifier, setIdentifier] = useState("");
  const [interval, setInterval] = useState("1m");
  const [range, setRange] = useState("24h");

  const telemetry = useQuery({
    queryKey: ["telemetry", deviceId, identifier, interval, range],
    queryFn: () =>
      api.telemetry(deviceId, {
        identifier: identifier || undefined,
        interval: interval === "raw" ? undefined : interval,
        from: new Date(Date.now() - Number(range.replace("h", "").replace("d", "")) * (range.endsWith("d") ? 86400000 : 3600000)).toISOString(),
        to: new Date().toISOString(),
        limit: 5000,
      }),
    enabled: !!identifier,
  });

  const chartData = useMemo(() => {
    const pts = telemetry.data?.points ?? [];
    return pts
      .map((p: TelemetryPoint) => ({
        t: new Date(p.bucket ?? p.time ?? 0).toLocaleTimeString(),
        value: p.avg ?? p.numValue ?? null,
        min: p.min,
        max: p.max,
      }))
      .reverse();
  }, [telemetry.data]);

  const [commandKind, setCommandKind] = useState("property");
  const [commandIdentifier, setCommandIdentifier] = useState("");
  const [commandPayload, setCommandPayload] = useState('{"temperature": 30}');

  const sendCommand = useMutation({
    mutationFn: () =>
      api.command(deviceId, {
        kind: commandKind,
        identifier: commandIdentifier || undefined,
        payload: JSON.parse(commandPayload || "{}"),
      }),
    onSuccess: () => toast.success("Command sent to gateway"),
    onError: (e: Error) => toast.error(e.message),
  });

  const [peerTarget, setPeerTarget] = useState("");
  const [peerBroadcast, setPeerBroadcast] = useState(false);
  const [peerPayload, setPeerPayload] = useState('{"msg": "hello"}');
  const sendPeer = useMutation({
    mutationFn: () =>
      api.peer(deviceId, {
        target: peerTarget || undefined,
        broadcast: peerBroadcast,
        payload: JSON.parse(peerPayload || "{}"),
      }),
    onSuccess: (r) => toast.success(`Delivered to ${r.sent} device(s)`),
    onError: (e: Error) => toast.error(e.message),
  });

  const [desiredPatch, setDesiredPatch] = useState('{\n  "reportInterval": 60\n}');
  const [connectOpen, setConnectOpen] = useState(false);
  const applyDesired = useMutation({
    mutationFn: () => api.patchShadowDesired(deviceId, JSON.parse(desiredPatch || "{}")),
    onSuccess: () => {
      toast.success("Desired state updated");
      void qc.invalidateQueries({ queryKey: ["shadow", deviceId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const clearDesired = useMutation({
    mutationFn: () => api.clearShadowDesired(deviceId),
    onSuccess: () => {
      toast.success("Desired state cleared");
      void qc.invalidateQueries({ queryKey: ["shadow", deviceId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const rotate = useMutation({
    mutationFn: () => api.rotateSecret(deviceId),
    onSuccess: (r) => toast.success(`New secret: ${r.secret}`, { duration: 20000 }),
    onError: (e: Error) => toast.error(e.message),
  });

  const toggleStatus = useMutation({
    mutationFn: () => api.setDeviceStatus(deviceId, device.data?.status === "enabled" ? "disabled" : "enabled"),
    onSuccess: () => {
      toast.success("Device updated");
      void qc.invalidateQueries({ queryKey: ["device", deviceId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const remove = useMutation({
    mutationFn: () => api.deleteDevice(deviceId),
    onSuccess: () => {
      toast.success("Device deleted");
      navigate(-1);
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const d = device.data;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate(-1)}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-semibold tracking-tight">{d?.name ?? "Device"}</h1>
              <Badge variant={d?.online ? "success" : "secondary"}>
                <Wifi className="mr-1 h-3 w-3" />
                {d?.online ? "online" : "offline"}
              </Badge>
              {d && <Badge variant="outline">{d.status}</Badge>}
            </div>
            <p className="text-sm text-muted-foreground">
              <span className="font-mono">{d?.key}</span>
              {d?.product && <> · {d.product.name} ({d.product.protocol})</>}
              {d?.workspace && <> · {d.workspace.name}</>}
            </p>
          </div>
        </div>
        <Button variant="outline" onClick={() => setConnectOpen(true)}>
          <Plug className="h-4 w-4" /> {t("device.connect")}
        </Button>
      </div>

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">{t("device.overview")}</TabsTrigger>
          <TabsTrigger value="telemetry">{t("device.telemetry")}</TabsTrigger>
          <TabsTrigger value="events">{t("device.events")}</TabsTrigger>
          <TabsTrigger value="command">{t("device.command")}</TabsTrigger>
          <TabsTrigger value="peer">{t("device.peer")}</TabsTrigger>
          <TabsTrigger value="shadow">{t("device.shadow")}</TabsTrigger>
          <TabsTrigger value="settings">{t("device.settings")}</TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {(latest.data ?? []).map((v) => (
              <Card key={v.identifier}>
                <CardHeader className="pb-2">
                  <CardDescription>{v.identifier}</CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-semibold">{valueOf(v)}</div>
                  <div className="mt-1 text-xs text-muted-foreground">{formatTime(v.updatedAt)}</div>
                </CardContent>
              </Card>
            ))}
            {(latest.data ?? []).length === 0 && (
              <p className="text-sm text-muted-foreground">{t("device.latestEmpty")}</p>
            )}
          </div>
        </TabsContent>

        <TabsContent value="telemetry">
          <Card>
            <CardHeader className="flex flex-row flex-wrap items-end gap-3">
              <div className="space-y-2">
                <Label>{t("common.identifier")}</Label>
                <Select value={identifier} onValueChange={setIdentifier}>
                  <SelectTrigger className="w-48">
                    <SelectValue placeholder={t("device.selectIdentifier")} />
                  </SelectTrigger>
                  <SelectContent>
                    {(latest.data ?? []).map((v) => (
                      <SelectItem key={v.identifier} value={v.identifier}>
                        {v.identifier}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("device.interval")}</Label>
                <Select value={interval} onValueChange={setInterval}>
                  <SelectTrigger className="w-32">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="raw">raw</SelectItem>
                    <SelectItem value="1m">1m</SelectItem>
                    <SelectItem value="5m">5m</SelectItem>
                    <SelectItem value="1h">1h</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("device.range")}</Label>
                <Select value={range} onValueChange={setRange}>
                  <SelectTrigger className="w-32">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="1h">1h</SelectItem>
                    <SelectItem value="6h">6h</SelectItem>
                    <SelectItem value="24h">24h</SelectItem>
                    <SelectItem value="7d">7d</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Button variant="outline" onClick={() => telemetry.refetch()}>
                <RefreshCw className="h-4 w-4" /> {t("common.refresh")}
              </Button>
              {telemetry.data?.source && <Badge variant="outline">source: {telemetry.data.source}</Badge>}
            </CardHeader>
            <CardContent>
              {!identifier ? (
                <p className="py-10 text-center text-sm text-muted-foreground">{t("device.selectIdentifier")}</p>
              ) : chartData.length === 0 ? (
                <p className="py-10 text-center text-sm text-muted-foreground">{t("device.noData")}</p>
              ) : (
                <ResponsiveContainer width="100%" height={340}>
                  <LineChart data={chartData}>
                    <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                    <XAxis dataKey="t" fontSize={12} />
                    <YAxis fontSize={12} />
                    <Tooltip />
                    <Line type="monotone" dataKey="value" stroke="hsl(var(--primary))" dot={false} strokeWidth={2} />
                  </LineChart>
                </ResponsiveContainer>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="events">
          <Card>
            <CardContent className="pt-5">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("audit.time")}</TableHead>
                    <TableHead>{t("common.identifier")}</TableHead>
                    <TableHead>{t("common.type")}</TableHead>
                    <TableHead>{t("devices.payload")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(events.data ?? []).length === 0 && (
                    <TableRow>
                      <TableCell colSpan={4} className="py-8 text-center text-muted-foreground">
                        {t("device.noEvents")}
                      </TableCell>
                    </TableRow>
                  )}
                  {(events.data ?? []).map((e) => (
                    <TableRow key={e.id}>
                      <TableCell className="text-xs">{formatTime(e.occurredAt)}</TableCell>
                      <TableCell className="font-mono text-xs">{e.identifier}</TableCell>
                      <TableCell>{e.type}</TableCell>
                      <TableCell className="max-w-md truncate font-mono text-xs">{JSON.stringify(e.payload)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="command">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("device.commandTitle")}</CardTitle>
              <CardDescription>{t("device.commandDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label>{t("devices.kind")}</Label>
                  <Select value={commandKind} onValueChange={setCommandKind}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="property">{t("devices.propertySet")}</SelectItem>
                      <SelectItem value="service_call">{t("devices.serviceCall")}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t("common.identifier")}</Label>
                  <Input value={commandIdentifier} onChange={(e) => setCommandIdentifier(e.target.value)} />
                </div>
              </div>
              <div className="space-y-2">
                <Label>{t("devices.payload")}</Label>
                <Textarea className="font-mono" rows={6} value={commandPayload} onChange={(e) => setCommandPayload(e.target.value)} />
              </div>
              <Button onClick={() => sendCommand.mutate()} disabled={sendCommand.isPending}>
                <Send className="h-4 w-4" /> {t("device.sendCommand")}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="peer">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("device.peerTitle")}</CardTitle>
              <CardDescription>{t("device.peerDesc")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center gap-3">
                <Switch checked={peerBroadcast} onCheckedChange={setPeerBroadcast} />
                <Label>{t("device.broadcast")}</Label>
              </div>
              {!peerBroadcast && (
                <div className="space-y-2">
                  <Label>{t("device.targetKey")}</Label>
                  <Input value={peerTarget} onChange={(e) => setPeerTarget(e.target.value)} placeholder="sensor-b" />
                </div>
              )}
              <div className="space-y-2">
                <Label>{t("devices.payload")}</Label>
                <Textarea className="font-mono" rows={5} value={peerPayload} onChange={(e) => setPeerPayload(e.target.value)} />
              </div>
              <Button onClick={() => sendPeer.mutate()} disabled={sendPeer.isPending}>
                <Send className="h-4 w-4" /> {t("device.sendPeer")}
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="shadow">
          <div className="space-y-4">
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm">{t("device.desired")}</CardTitle>
                  <CardDescription>{t("device.desiredDesc")}</CardDescription>
                </CardHeader>
                <CardContent>
                  <pre className="max-h-56 overflow-auto rounded bg-muted p-3 text-xs">
                    {JSON.stringify(shadow.data?.desired ?? {}, null, 2)}
                  </pre>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm">{t("device.reported")}</CardTitle>
                  <CardDescription>{t("device.reportedDesc")}</CardDescription>
                </CardHeader>
                <CardContent>
                  <pre className="max-h-56 overflow-auto rounded bg-muted p-3 text-xs">
                    {JSON.stringify(shadow.data?.reported ?? {}, null, 2)}
                  </pre>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm">{t("device.delta")}</CardTitle>
                  <CardDescription>{t("device.deltaDesc")}</CardDescription>
                </CardHeader>
                <CardContent>
                  <pre className="max-h-56 overflow-auto rounded bg-muted p-3 text-xs">
                    {JSON.stringify(shadow.data?.delta ?? {}, null, 2)}
                  </pre>
                </CardContent>
              </Card>
            </div>
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm">{t("device.updateDesired")}</CardTitle>
                <CardDescription>{t("device.updateDesiredDesc")}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                <Textarea
                  className="font-mono"
                  rows={5}
                  value={desiredPatch}
                  onChange={(e) => setDesiredPatch(e.target.value)}
                />
                <div className="flex flex-wrap gap-2">
                  <Button onClick={() => applyDesired.mutate()} disabled={applyDesired.isPending}>
                    <Send className="h-4 w-4" /> {t("device.applyDesired")}
                  </Button>
                  <Button variant="outline" onClick={() => clearDesired.mutate()} disabled={clearDesired.isPending}>
                    {t("device.clearDesired")}
                  </Button>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm">{t("device.shadowHistory")}</CardTitle>
                <CardDescription>{t("device.shadowHistoryDesc")}</CardDescription>
              </CardHeader>
              <CardContent>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("audit.time")}</TableHead>
                      <TableHead>{t("device.source")}</TableHead>
                      <TableHead>{t("device.desired")}</TableHead>
                      <TableHead>{t("device.reported")}</TableHead>
                      <TableHead>{t("device.delta")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(shadowHistory.data ?? []).length === 0 && (
                      <TableRow>
                        <TableCell colSpan={5} className="py-6 text-center text-muted-foreground">
                          {t("device.noShadowChanges")}
                        </TableCell>
                      </TableRow>
                    )}
                    {(shadowHistory.data ?? []).map((h) => (
                      <TableRow key={h.id}>
                        <TableCell className="whitespace-nowrap text-xs">{formatTime(h.occurredAt)}</TableCell>
                        <TableCell>
                          <Badge variant={h.source === "rule" ? "warning" : h.source === "api" ? "default" : "secondary"}>
                            {h.source}
                            {h.ruleId ? ` #${h.ruleId}` : ""}
                          </Badge>
                        </TableCell>
                        <TableCell className="max-w-[16rem] truncate font-mono text-xs">
                          {JSON.stringify(h.desired)}
                        </TableCell>
                        <TableCell className="max-w-[16rem] truncate font-mono text-xs">
                          {JSON.stringify(h.reported)}
                        </TableCell>
                        <TableCell className="max-w-[12rem] truncate font-mono text-xs">
                          {JSON.stringify(h.delta)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="settings">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{t("device.settingsTitle")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-wrap gap-3">
                <Button variant="outline" onClick={() => rotate.mutate()}>
                  <KeyRound className="h-4 w-4" /> {t("device.rotateSecret")}
                </Button>
                <Button variant="outline" onClick={() => toggleStatus.mutate()}>
                  {d?.status === "enabled" ? t("device.disable") : t("device.enable")}
                </Button>
                <ConfirmButton
                  title={t("device.deleteConfirm")}
                  description={t("device.deleteDesc")}
                  onConfirm={() => remove.mutateAsync()}
                >
                  <Button variant="destructive">
                    <Trash2 className="h-4 w-4" /> {t("device.deleteDevice")}
                  </Button>
                </ConfirmButton>
              </div>
              <p className="text-sm text-muted-foreground">
                Connection credentials: username <code className="rounded bg-muted px-1">{"{productKey}/{deviceKey}"}</code>, password is the
                device secret.
              </p>
              <Link to={d ? `/projects/${d.projectId}/devices` : "#"} className="text-sm text-primary hover:underline">
                {t("device.backToDevices")}
              </Link>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <ConnectDialog deviceId={deviceId} deviceName={d?.name} open={connectOpen} onOpenChange={setConnectOpen} />
    </div>
  );
}
