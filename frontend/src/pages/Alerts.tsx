import { useState } from "react";
import { useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing, History, Plus, Send, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type Alert as AlertItem, type NotifyChannel } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ProjectNav } from "@/components/ProjectNav";
import { ConfirmButton } from "@/components/ConfirmButton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useProjectRole } from "@/lib/useProjectRole";
import { formatTime, sinceText } from "@/lib/utils";

export default function AlertsPage() {
  const projectId = Number(useParams().projectId);
  const qc = useQueryClient();
  const { t } = useI18n();
  const { canWrite, canAdmin } = useProjectRole(projectId);
  const [alertStatus, setAlertStatus] = useState<string>("all");
  const alerts = useQuery({
    queryKey: ["alerts", projectId, alertStatus],
    queryFn: () => api.listAlerts(projectId, { status: alertStatus === "all" ? undefined : alertStatus }),
    refetchInterval: 5000,
  });
  const ackAlert = useMutation({
    mutationFn: (id: number) => api.ackAlert(id),
    onSuccess: () => {
      toast.success("Alert acknowledged");
      void qc.invalidateQueries({ queryKey: ["alerts", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const resolveAlert = useMutation({
    mutationFn: (id: number) => api.resolveAlert(id),
    onSuccess: () => {
      toast.success("Alert resolved");
      void qc.invalidateQueries({ queryKey: ["alerts", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const { data: channels, isLoading } = useQuery({
    queryKey: ["channels", projectId],
    queryFn: () => api.listChannels(projectId),
  });

  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: "", type: "webhook", target: "", to: "" });
  const [logChannel, setLogChannel] = useState<NotifyChannel | null>(null);
  const logs = useQuery({
    queryKey: ["channelLogs", logChannel?.id],
    queryFn: () => api.channelLogs(logChannel!.id),
    enabled: !!logChannel,
  });

  const create = useMutation({
    mutationFn: () => {
      const config: Record<string, unknown> = {};
      if (form.type === "webhook") config.url = form.target;
      if (form.type === "dingtalk") config.webhook = form.target;
      if (form.type === "email") config.to = form.to;
      return api.createChannel(projectId, { name: form.name, type: form.type as NotifyChannel["type"], config });
    },
    onSuccess: () => {
      toast.success("Channel created");
      setOpen(false);
      setForm({ name: "", type: "webhook", target: "", to: "" });
      void qc.invalidateQueries({ queryKey: ["channels", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const toggle = useMutation({
    mutationFn: (ch: NotifyChannel) =>
      api.updateChannel(ch.id, { name: ch.name, type: ch.type, enabled: !ch.enabled, config: ch.config }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["channels", projectId] }),
    onError: (e: Error) => toast.error(e.message),
  });

  const test = useMutation({
    mutationFn: (id: number) => api.testChannel(id),
    onSuccess: () => toast.success("Test notification sent"),
    onError: (e: Error) => toast.error(e.message),
  });

  const remove = useMutation({
    mutationFn: (id: number) => api.deleteChannel(id),
    onSuccess: () => {
      toast.success("Channel deleted");
      void qc.invalidateQueries({ queryKey: ["channels", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight whitespace-nowrap">{t("alerts.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("alerts.subtitle")}</p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="w-full sm:w-auto" disabled={!canWrite}>
              <Plus className="h-4 w-4" /> {t("alerts.new")}
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("alerts.createTitle")}</DialogTitle>
              <DialogDescription>{t("alerts.createDesc")}</DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>{t("common.name")}</Label>
                <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
              </div>
              <div className="space-y-2">
                <Label>{t("common.type")}</Label>
                <Select value={form.type} onValueChange={(v) => setForm({ ...form, type: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="webhook">Webhook</SelectItem>
                    <SelectItem value="dingtalk">DingTalk</SelectItem>
                    <SelectItem value="email">Email</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {form.type !== "email" ? (
                <div className="space-y-2">
                  <Label>{form.type === "dingtalk" ? t("alerts.dingtalkUrl") : t("alerts.webhookUrl")}</Label>
                  <Input value={form.target} onChange={(e) => setForm({ ...form, target: e.target.value })} placeholder="https://..." />
                </div>
              ) : (
                <div className="space-y-2">
                  <Label>{t("alerts.recipients")}</Label>
                  <Input value={form.to} onChange={(e) => setForm({ ...form, to: e.target.value })} placeholder="ops@example.com" />
                  <p className="text-xs text-muted-foreground">{t("alerts.smtpHint")}</p>
                </div>
              )}
            </div>
            <DialogFooter>
              <Button onClick={() => create.mutate()} disabled={create.isPending || !form.name}>
                {t("common.create")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      <ProjectNav projectId={projectId} />

      <Tabs defaultValue="alerts">
        <TabsList>
          <TabsTrigger value="alerts">{t("alerts.tabAlerts")}</TabsTrigger>
          <TabsTrigger value="channels">{t("alerts.tabChannels")}</TabsTrigger>
        </TabsList>

        <TabsContent value="alerts" className="space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            {alerts.data?.counts?.firing != null && (
              <Badge variant="destructive">{t("alerts.firing")}: {alerts.data.counts.firing}</Badge>
            )}
            {alerts.data?.counts?.acknowledged != null && (
              <Badge variant="warning">{t("alerts.acknowledged")}: {alerts.data.counts.acknowledged}</Badge>
            )}
            {alerts.data?.counts?.resolved != null && (
              <Badge variant="secondary">{t("alerts.resolved")}: {alerts.data.counts.resolved}</Badge>
            )}
            <Select value={alertStatus} onValueChange={setAlertStatus}>
              <SelectTrigger className="ml-auto w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("alerts.all")}</SelectItem>
                <SelectItem value="firing">{t("alerts.firing")}</SelectItem>
                <SelectItem value="acknowledged">{t("alerts.acknowledged")}</SelectItem>
                <SelectItem value="resolved">{t("alerts.resolved")}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <Card>
            <CardContent className="pt-5">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("audit.time")}</TableHead>
                    <TableHead>{t("alerts.alert")}</TableHead>
                    <TableHead>{t("devices.titleSingle")}</TableHead>
                    <TableHead>{t("common.identifier")}</TableHead>
                    <TableHead>{t("alerts.level")}</TableHead>
                    <TableHead>{t("alerts.status")}</TableHead>
                    <TableHead>{t("alerts.fireCount")}</TableHead>
                    <TableHead className="w-28 text-right">{t("common.actions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(alerts.data?.items ?? []).length === 0 && (
                    <TableRow>
                      <TableCell colSpan={8} className="py-8 text-center text-muted-foreground">
                        {t("alerts.noAlerts")}
                      </TableCell>
                    </TableRow>
                  )}
                  {(alerts.data?.items ?? []).map((a: AlertItem) => (
                    <TableRow key={a.id}>
                      <TableCell className="whitespace-nowrap text-xs">{formatTime(a.startsAt)}</TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <span className="font-medium">{a.title}</span>
                          {(a.escalations ?? 0) > 0 && (
                            <Badge variant="warning" title="escalated">
                              ↑{a.escalations}
                            </Badge>
                          )}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {a.status === "resolved" ? a.resolveReason || "resolved" : sinceText(a.startsAt)}
                        </div>
                      </TableCell>
                      <TableCell className="font-mono text-xs">{a.deviceKey}</TableCell>
                      <TableCell className="font-mono text-xs">{a.identifier || "-"}</TableCell>
                      <TableCell>
                        <Badge variant={a.level === "critical" ? "destructive" : a.level === "warning" ? "warning" : "secondary"}>
                          {a.level}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge variant={a.status === "firing" ? "destructive" : a.status === "acknowledged" ? "warning" : "secondary"}>
                          {a.status}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs">{a.fireCount}</TableCell>
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-1 whitespace-nowrap">
                          {a.status === "firing" && (
                            <Button size="sm" variant="outline" onClick={() => ackAlert.mutate(a.id)}>
                              {t("alerts.ack")}
                            </Button>
                          )}
                          {a.status !== "resolved" && (
                            <Button size="sm" variant="ghost" onClick={() => resolveAlert.mutate(a.id)}>
                              {t("alerts.resolve")}
                            </Button>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="channels" className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <BellRing className="h-4 w-4" /> {t("alerts.channels")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Enabled</TableHead>
                <TableHead>{t("common.name")}</TableHead>
                <TableHead>{t("common.type")}</TableHead>
                <TableHead className="hidden sm:table-cell">{t("alerts.target")}</TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading && (
                <TableRow>
                  <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                    {t("common.loading")}
                  </TableCell>
                </TableRow>
              )}
              {(channels ?? []).length === 0 && !isLoading && (
                <TableRow>
                  <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                    {t("alerts.empty")}
                  </TableCell>
                </TableRow>
              )}
              {(channels ?? []).map((ch) => (
                <TableRow key={ch.id}>
                  <TableCell>
                    <Switch checked={ch.enabled} onCheckedChange={() => toggle.mutate(ch)} />
                  </TableCell>
                  <TableCell className="font-medium">{ch.name}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">{ch.type}</Badge>
                  </TableCell>
                  <TableCell className="hidden max-w-xs truncate text-xs text-muted-foreground sm:table-cell">
                    {String(ch.config?.url ?? ch.config?.webhook ?? ch.config?.to ?? "-")}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => test.mutate(ch.id)}
                      title="Send test"
                      aria-label="Send test notification"
                      disabled={!canWrite}
                    >
                      <Send className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="icon" onClick={() => setLogChannel(ch)} title="Logs">
                      <History className="h-4 w-4" />
                    </Button>
                    {canAdmin && (
                      <ConfirmButton
                        title={t("alerts.deleteConfirm")}
                        description={t("alerts.deleteDesc")}
                        onConfirm={() => remove.mutateAsync(ch.id)}
                      >
                        <Button variant="ghost" size="icon" aria-label="Delete channel">
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </ConfirmButton>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
        </TabsContent>
      </Tabs>

      <Dialog open={!!logChannel} onOpenChange={(o) => !o && setLogChannel(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("alerts.logs")} — {logChannel?.name}</DialogTitle>
          </DialogHeader>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("audit.time")}</TableHead>
                <TableHead>{t("common.status")}</TableHead>
                <TableHead>{t("common.name")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(logs.data ?? []).length === 0 && (
                <TableRow>
                  <TableCell colSpan={3} className="py-6 text-center text-muted-foreground">
                    {t("alerts.noDeliveries")}
                  </TableCell>
                </TableRow>
              )}
              {(logs.data ?? []).map((l) => (
                <TableRow key={l.id}>
                  <TableCell className="text-xs">{formatTime(l.createdAt)}</TableCell>
                  <TableCell>
                    <Badge variant={l.success ? "success" : "destructive"}>{l.success ? "ok" : "error"}</Badge>
                    {l.error && <div className="mt-1 text-xs text-destructive">{l.error}</div>}
                  </TableCell>
                  <TableCell className="text-xs">{l.title}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DialogContent>
      </Dialog>
    </div>
  );
}
