import { useState } from "react";
import { useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2, Pencil, History, Code2, CheckCircle2, XCircle, Braces, ListPlus } from "lucide-react";
import { toast } from "sonner";
import { api, type Rule } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { TableSkeletonRows } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ProjectNav } from "@/components/ProjectNav";
import { ConfirmButton } from "@/components/ConfirmButton";
import { useProjectRole } from "@/lib/useProjectRole";
import { useI18n } from "@/lib/i18n";
import { formatTime } from "@/lib/utils";

const triggers = [
  { value: "schedule", label: "Schedule (cron)" },
  { value: "telemetry", label: "Telemetry (property)" },
  { value: "event", label: "Event" },
  { value: "device_online", label: "Device online" },
  { value: "device_offline", label: "Device offline" },
  { value: "peer_message", label: "Peer message" },
  { value: "shadow_delta", label: "Shadow delta" },
];

const actionTypes = [
  { value: "log", label: "Log" },
  { value: "notify", label: "Notify channel" },
  { value: "webhook", label: "Webhook" },
  { value: "set_desired", label: "Set shadow desired" },
  { value: "downlink", label: "Downlink / publish" },
];

const downlinkKinds = [
  { value: "property", label: "Property set" },
  { value: "service_call", label: "Service call" },
  { value: "peer", label: "Peer message" },
];

interface ActionDraft {
  type: string;
  channelId?: number;
  titleTemplate?: string;
  bodyTemplate?: string;
  message?: string;
  url?: string;
  method?: string;
  desiredJson?: string;
  kind?: string;
  identifier?: string;
  payloadJson?: string;
  deviceKey?: string;
}

interface Draft {
  name: string;
  description: string;
  triggerType: string;
  triggerSource: string;
  cron: string;
  condition: string;
  priority: number;
  actions: ActionDraft[];
}

const emptyDraft: Draft = {
  name: "",
  description: "",
  triggerType: "telemetry",
  triggerSource: "*",
  cron: "",
  condition: 'identifier == "temperature" && double(value) > 40.0',
  priority: 0,
  actions: [{ type: "log" }],
};

function safeParse(raw: string): unknown {
  if (!raw.trim()) return undefined;
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

function toAction(a: ActionDraft): Record<string, unknown> {
  const out: Record<string, unknown> = { type: a.type };
  switch (a.type) {
    case "log":
      if (a.message) out.message = a.message;
      break;
    case "notify":
      if (a.channelId) out.channelId = a.channelId;
      if (a.titleTemplate) out.titleTemplate = a.titleTemplate;
      if (a.bodyTemplate) out.bodyTemplate = a.bodyTemplate;
      if (a.message) out.message = a.message;
      break;
    case "webhook":
      out.url = a.url ?? "";
      out.method = a.method || "POST";
      if (a.bodyTemplate) out.bodyTemplate = a.bodyTemplate;
      if (a.payloadJson) out.body = safeParse(a.payloadJson);
      break;
    case "set_desired":
      out.desired = safeParse(a.desiredJson || "{}") ?? {};
      break;
    case "downlink":
      out.kind = a.kind || "property";
      if (a.identifier) out.identifier = a.identifier;
      if (a.payloadJson) out.payload = safeParse(a.payloadJson);
      if (a.deviceKey) out.deviceKey = a.deviceKey;
      break;
  }
  return out;
}

function fromAction(m: Record<string, unknown>): ActionDraft {
  const d: ActionDraft = { type: String(m.type ?? "log") };
  if (m.channelId != null) d.channelId = Number(m.channelId);
  if (m.titleTemplate) d.titleTemplate = String(m.titleTemplate);
  if (m.bodyTemplate) d.bodyTemplate = String(m.bodyTemplate);
  if (m.message) d.message = String(m.message);
  if (m.url) d.url = String(m.url);
  if (m.method) d.method = String(m.method);
  if (m.desired) d.desiredJson = JSON.stringify(m.desired, null, 2);
  if (m.kind) d.kind = String(m.kind);
  if (m.identifier) d.identifier = String(m.identifier);
  if (m.payload) d.payloadJson = JSON.stringify(m.payload, null, 2);
  if (m.deviceKey) d.deviceKey = String(m.deviceKey);
  return d;
}

export default function RulesPage() {
  const projectId = Number(useParams().projectId);
  const qc = useQueryClient();
  const { t } = useI18n();
  const { canWrite, canAdmin } = useProjectRole(projectId);
  const [page, setPage] = useState(1);
  const { data: rulesData, isLoading } = useQuery({
    queryKey: ["rules", projectId, page],
    queryFn: () => api.listRules(projectId, { page, pageSize: 50 }),
  });
  const rules = rulesData?.items ?? [];
  const total = rulesData?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / 50));
  const reference = useQuery({ queryKey: ["ruleReference"], queryFn: api.ruleReference });
  const channels = useQuery({ queryKey: ["channels", projectId], queryFn: () => api.listChannels(projectId) });

  const [open, setOpen] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [jsonMode, setJsonMode] = useState(false);
  const [actionsJson, setActionsJson] = useState("[]");
  const [validity, setValidity] = useState<{ valid: boolean; error?: string } | null>(null);
  const [logRule, setLogRule] = useState<Rule | null>(null);
  const [channelForm, setChannelForm] = useState<{ index: number; name: string; type: string; target: string; to: string } | null>(null);
  const [bField, setBField] = useState("identifier");
  const [bOp, setBOp] = useState("==");
  const [bValue, setBValue] = useState("");

  const builderFields = ["identifier", "value", "device_key", "state", "kind"];
  const builderOps = ["==", "!=", ">", ">=", "<", "<=", "contains"];

  function buildExpr(): string {
    const v = bValue;
    if (bField === "value") {
      const n = Number(v);
      const op = bOp === "contains" ? "==" : bOp;
      return `double(value) ${op} ${Number.isNaN(n) ? 0 : n}`;
    }
    if (bOp === "contains") return `${bField}.contains("${v}")`;
    return `${bField} ${bOp} "${v}"`;
  }

  function applyCondition(mode: "replace" | "and") {
    const expr = buildExpr();
    setDraft((d) => ({ ...d, condition: mode === "replace" ? expr : `${d.condition} && ${expr}` }));
    setValidity(null);
  }

  const logs = useQuery({
    queryKey: ["ruleLogs", logRule?.id],
    queryFn: () => api.ruleLogs(logRule!.id),
    enabled: !!logRule,
  });

  function openCreate() {
    setEditingId(null);
    setDraft(emptyDraft);
    setActionsJson(JSON.stringify([{ type: "log" }], null, 2));
    setJsonMode(false);
    setValidity(null);
    setOpen(true);
  }

  function openEdit(r: Rule) {
    setEditingId(r.id);
    setDraft({
      name: r.name,
      description: r.description ?? "",
      triggerType: r.triggerType,
      triggerSource: r.triggerSource,
      cron: r.cron ?? "",
      condition: r.condition,
      priority: r.priority,
      actions: (r.actions ?? []).map(fromAction),
    });
    setActionsJson(JSON.stringify(r.actions ?? [], null, 2));
    setJsonMode(false);
    setValidity(null);
    setOpen(true);
  }

  const save = useMutation({
    mutationFn: () => {
      let actions: Record<string, unknown>[];
      if (jsonMode) {
        try {
          actions = JSON.parse(actionsJson || "[]");
        } catch {
          throw new Error("Actions must be valid JSON");
        }
      } else {
        actions = draft.actions.map(toAction);
      }
      const payload = {
        name: draft.name,
        description: draft.description,
        triggerType: draft.triggerType,
        triggerSource: draft.triggerSource || "*",
        cron: draft.cron,
        condition: draft.condition,
        actions,
        priority: draft.priority,
      };
      return editingId ? api.updateRule(editingId, payload) : api.createRule(projectId, payload);
    },
    onSuccess: () => {
      toast.success(editingId ? "Rule updated" : "Rule created");
      setOpen(false);
      void qc.invalidateQueries({ queryKey: ["rules", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const validate = useMutation({
    mutationFn: () => api.validateRule(draft.condition),
    onSuccess: (r) => {
      setValidity(r);
      r.valid ? toast.success("CEL condition is valid") : toast.error(r.error || "Invalid condition");
    },
  });

  const toggle = useMutation({
    mutationFn: (id: number) => api.toggleRule(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["rules", projectId] }),
    onError: (e: Error) => toast.error(e.message),
  });

  const remove = useMutation({
    mutationFn: (id: number) => api.deleteRule(id),
    onSuccess: () => {
      toast.success("Rule deleted");
      void qc.invalidateQueries({ queryKey: ["rules", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const createChannel = useMutation({
    mutationFn: () => {
      const cfg: Record<string, unknown> = {};
      if (channelForm!.type === "webhook") cfg.url = channelForm!.target;
      if (channelForm!.type === "dingtalk") cfg.webhook = channelForm!.target;
      if (channelForm!.type === "email") cfg.to = channelForm!.to;
      return api.createChannel(projectId, {
        name: channelForm!.name,
        type: channelForm!.type as "webhook" | "dingtalk" | "email",
        config: cfg,
      });
    },
    onSuccess: (ch) => {
      toast.success("Channel created");
      void qc.invalidateQueries({ queryKey: ["channels", projectId] });
      updateAction(channelForm!.index, { channelId: ch.id });
      setChannelForm(null);
    },
    onError: (e: Error) => toast.error(e.message),
  });

  function updateAction(index: number, patch: Partial<ActionDraft>) {
    setDraft((d) => ({ ...d, actions: d.actions.map((a, i) => (i === index ? { ...a, ...patch } : a)) }));
  }

  function addAction() {
    setDraft((d) => ({ ...d, actions: [...d.actions, { type: "log" }] }));
  }

  function removeAction(index: number) {
    setDraft((d) => ({ ...d, actions: d.actions.filter((_, i) => i !== index) }));
  }

  function switchToJson() {
    setActionsJson(JSON.stringify(draft.actions.map(toAction), null, 2));
    setJsonMode(true);
  }

  function switchToVisual() {
    try {
      const parsed = JSON.parse(actionsJson || "[]") as Record<string, unknown>[];
      setDraft((d) => ({ ...d, actions: parsed.map(fromAction) }));
      setJsonMode(false);
    } catch {
      toast.error("Fix the JSON before switching to the visual editor");
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight whitespace-nowrap">{t("rules.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("rules.subtitle")}</p>
        </div>
        <Button onClick={openCreate} className="w-full sm:w-auto" disabled={!canWrite}>
          <Plus className="h-4 w-4" /> {t("rules.new")}
        </Button>
      </div>

      <ProjectNav projectId={projectId} />

      <Card>
        <CardContent className="pt-5">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("common.status")}</TableHead>
                <TableHead>{t("common.name")}</TableHead>
                <TableHead>{t("rules.trigger")}</TableHead>
                <TableHead>{t("rules.condition")}</TableHead>
                <TableHead>{t("rules.actions")}</TableHead>
                <TableHead>{t("rules.triggered")}</TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading && <TableSkeletonRows cols={7} />}
              {rules.length === 0 && !isLoading && (
                <TableRow>
                  <TableCell colSpan={7} className="py-8 text-center text-muted-foreground">
                    {t("rules.empty")}
                  </TableCell>
                </TableRow>
              )}
              {rules.map((r) => (
                <TableRow key={r.id}>
                  <TableCell>
                    <Switch checked={r.enabled} onCheckedChange={() => toggle.mutate(r.id)} disabled={!canWrite} />
                  </TableCell>
                  <TableCell>
                    <div className="font-medium">{r.name}</div>
                    <div className="text-xs text-muted-foreground">{r.description}</div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary">{r.triggerType}</Badge>
                    <div className="mt-1 text-xs text-muted-foreground">{r.triggerSource}</div>
                  </TableCell>
                  <TableCell>
                    <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{r.condition}</code>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {(r.actions ?? []).map((a, i) => (
                        <Badge key={i} variant="outline">
                          {String((a as Record<string, unknown>).type)}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>{r.triggerCount}</TableCell>
                  <TableCell className="text-right">
                    <Button variant="ghost" size="icon" onClick={() => setLogRule(r)} title="Execution logs">
                      <History className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="icon" onClick={() => openEdit(r)} disabled={!canWrite}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    {canAdmin && (
                      <ConfirmButton
                        title="Delete rule?"
                        description="The rule and its execution history will be removed."
                        onConfirm={() => remove.mutateAsync(r.id)}
                      >
                        <Button variant="ghost" size="icon" aria-label="Delete rule">
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </ConfirmButton>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {totalPages > 1 && (
            <div className="mt-3 flex items-center justify-between text-sm text-muted-foreground">
              <span>{t("common.totalCount", { count: total })}</span>
              <div className="flex items-center gap-2">
                <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
                  {t("common.prev")}
                </Button>
                <span>{t("common.pageOf", { page, pages: totalPages })}</span>
                <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
                  {t("common.next")}
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-4xl">
          <DialogHeader>
            <DialogTitle>{editingId ? t("rules.editTitle") : t("rules.createTitle")}</DialogTitle>
            <DialogDescription>{t("rules.createDesc")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 md:grid-cols-[1fr_240px]">
            <div className="space-y-4">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label>{t("common.name")}</Label>
                  <Input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
                </div>
                <div className="space-y-2">
                  <Label>{t("rules.priority")}</Label>
                  <Input
                    type="number"
                    value={draft.priority}
                    onChange={(e) => setDraft({ ...draft, priority: Number(e.target.value) })}
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t("rules.trigger")}</Label>
                  <Select value={draft.triggerType} onValueChange={(v) => setDraft({ ...draft, triggerType: v })}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {triggers.map((t) => (
                        <SelectItem key={t.value} value={t.value}>
                          {t.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                {draft.triggerType === "schedule" ? (
                  <div className="space-y-2">
                    <Label>
                      Cron <span className="text-xs font-normal text-muted-foreground">(5 fields: min hour dom month dow)</span>
                    </Label>
                    <Input
                      placeholder="*/5 * * * *"
                      value={draft.cron}
                      onChange={(e) => setDraft({ ...draft, cron: e.target.value })}
                    />
                  </div>
                ) : (
                  <div className="space-y-2">
                    <Label>{t("rules.triggerSource")}</Label>
                    <Input
                      value={draft.triggerSource}
                      onChange={(e) => setDraft({ ...draft, triggerSource: e.target.value })}
                    />
                  </div>
                )}
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="flex items-center gap-2">
                    <Code2 className="h-4 w-4" /> {t("rules.condition")}
                  </Label>
                  <Button size="sm" variant="outline" onClick={() => validate.mutate()} disabled={validate.isPending}>
                    {t("rules.validate")}
                  </Button>
                </div>
                <Textarea
                  className="font-mono"
                  rows={3}
                  value={draft.condition}
                  onChange={(e) => {
                    setDraft({ ...draft, condition: e.target.value });
                    setValidity(null);
                  }}
                />
                <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/30 p-2">
                  <span className="text-xs text-muted-foreground">{t("rules.builder")}:</span>
                  <Select value={bField} onValueChange={setBField}>
                    <SelectTrigger className="h-8 w-32">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {builderFields.map((f) => (
                        <SelectItem key={f} value={f}>
                          {f}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Select value={bOp} onValueChange={setBOp}>
                    <SelectTrigger className="h-8 w-24">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {builderOps.map((o) => (
                        <SelectItem key={o} value={o}>
                          {o}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Input
                    className="h-8 w-36"
                    placeholder="value"
                    value={bValue}
                    onChange={(e) => setBValue(e.target.value)}
                  />
                  <Button size="sm" variant="outline" onClick={() => applyCondition("and")}>
                    {t("rules.and")}
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => applyCondition("replace")}>
                    {t("rules.replace")}
                  </Button>
                </div>
                {validity && (
                  <div
                    className={
                      validity.valid
                        ? "flex items-center gap-1 text-sm text-emerald-600"
                        : "flex items-center gap-1 text-sm text-destructive"
                    }
                  >
                    {validity.valid ? <CheckCircle2 className="h-4 w-4" /> : <XCircle className="h-4 w-4" />}
                    {validity.valid ? "Valid" : validity.error}
                  </div>
                )}
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <Label className="flex items-center gap-2">
                    <ListPlus className="h-4 w-4" /> {t("rules.actions")}
                  </Label>
                  <div className="flex gap-2">
                    <Button size="sm" variant="outline" onClick={addAction} disabled={jsonMode}>
                      <Plus className="h-4 w-4" /> {t("common.add")}
                    </Button>
                    <Button size="sm" variant="outline" onClick={jsonMode ? switchToVisual : switchToJson}>
                      <Braces className="h-4 w-4" /> {jsonMode ? t("rules.visualMode") : t("rules.jsonMode")}
                    </Button>
                  </div>
                </div>

                {jsonMode ? (
                  <Textarea
                    className="font-mono"
                    rows={10}
                    value={actionsJson}
                    onChange={(e) => setActionsJson(e.target.value)}
                  />
                ) : (
                  <div className="space-y-3">
                    {draft.actions.length === 0 && (
                      <p className="text-sm text-muted-foreground">{t("rules.noActions")}</p>
                    )}
                    {draft.actions.map((a, i) => (
                      <div key={i} className="rounded-lg border p-3">
                        <div className="mb-3 flex items-center gap-2">
                          <Select value={a.type} onValueChange={(v) => updateAction(i, { type: v })}>
                            <SelectTrigger className="w-52">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {actionTypes.map((t) => (
                                <SelectItem key={t.value} value={t.value}>
                                  {t.label}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                          <Button variant="ghost" size="icon" className="ml-auto" onClick={() => removeAction(i)}>
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        </div>

                        {a.type === "log" && (
                          <div className="space-y-2">
                            <Label>Message (optional)</Label>
                            <Input value={a.message ?? ""} onChange={(e) => updateAction(i, { message: e.target.value })} />
                          </div>
                        )}

                        {a.type === "notify" && (
                          <div className="space-y-3">
                            <div className="space-y-2">
                              <div className="flex items-center justify-between">
                                <Label>Channel</Label>
                                <Button
                                  size="sm"
                                  variant="ghost"
                                  onClick={() =>
                                    setChannelForm({ index: i, name: "", type: "webhook", target: "", to: "" })
                                  }
                                >
                                  <Plus className="h-3 w-3" /> New
                                </Button>
                              </div>
                              <Select
                                value={a.channelId ? String(a.channelId) : ""}
                                onValueChange={(v) => updateAction(i, { channelId: Number(v) })}
                              >
                                <SelectTrigger>
                                  <SelectValue placeholder="Select channel" />
                                </SelectTrigger>
                                <SelectContent>
                                  {(channels.data ?? []).map((c) => (
                                    <SelectItem key={c.id} value={String(c.id)}>
                                      {c.name} ({c.type})
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                              {channelForm?.index === i && (
                                <div className="space-y-2 rounded-md border bg-muted/30 p-2">
                                  <Input
                                    placeholder="Channel name"
                                    value={channelForm.name}
                                    onChange={(e) => setChannelForm({ ...channelForm, name: e.target.value })}
                                  />
                                  <Select
                                    value={channelForm.type}
                                    onValueChange={(v) => setChannelForm({ ...channelForm, type: v })}
                                  >
                                    <SelectTrigger>
                                      <SelectValue />
                                    </SelectTrigger>
                                    <SelectContent>
                                      <SelectItem value="webhook">Webhook</SelectItem>
                                      <SelectItem value="dingtalk">DingTalk</SelectItem>
                                      <SelectItem value="email">Email</SelectItem>
                                    </SelectContent>
                                  </Select>
                                  {channelForm.type === "email" ? (
                                    <Input
                                      placeholder="recipients@example.com"
                                      value={channelForm.to}
                                      onChange={(e) => setChannelForm({ ...channelForm, to: e.target.value })}
                                    />
                                  ) : (
                                    <Input
                                      placeholder="https://..."
                                      value={channelForm.target}
                                      onChange={(e) => setChannelForm({ ...channelForm, target: e.target.value })}
                                    />
                                  )}
                                  <div className="flex gap-2">
                                    <Button
                                      size="sm"
                                      onClick={() => createChannel.mutate()}
                                      disabled={createChannel.isPending || !channelForm.name}
                                    >
                                      Create & select
                                    </Button>
                                    <Button size="sm" variant="outline" onClick={() => setChannelForm(null)}>
                                      Cancel
                                    </Button>
                                  </div>
                                </div>
                              )}
                            </div>
                            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                              <div className="space-y-2">
                                <Label>Title template</Label>
                                <Input
                                  placeholder="Alert: {{.deviceKey}}"
                                  value={a.titleTemplate ?? ""}
                                  onChange={(e) => updateAction(i, { titleTemplate: e.target.value })}
                                />
                              </div>
                              <div className="space-y-2">
                                <Label>Body template</Label>
                                <Input
                                  placeholder="{{.identifier}} = {{.value}}"
                                  value={a.bodyTemplate ?? ""}
                                  onChange={(e) => updateAction(i, { bodyTemplate: e.target.value })}
                                />
                              </div>
                            </div>
                          </div>
                        )}

                        {a.type === "webhook" && (
                          <div className="space-y-3">
                            <div className="grid grid-cols-1 gap-3 sm:grid-cols-[2fr_1fr]">
                              <div className="space-y-2">
                                <Label>URL</Label>
                                <Input value={a.url ?? ""} onChange={(e) => updateAction(i, { url: e.target.value })} />
                              </div>
                              <div className="space-y-2">
                                <Label>Method</Label>
                                <Select value={a.method ?? "POST"} onValueChange={(v) => updateAction(i, { method: v })}>
                                  <SelectTrigger>
                                    <SelectValue />
                                  </SelectTrigger>
                                  <SelectContent>
                                    <SelectItem value="POST">POST</SelectItem>
                                    <SelectItem value="PUT">PUT</SelectItem>
                                    <SelectItem value="GET">GET</SelectItem>
                                  </SelectContent>
                                </Select>
                              </div>
                            </div>
                            <div className="space-y-2">
                              <Label>Body template (optional)</Label>
                              <Input
                                placeholder='{"device":"{{.deviceKey}}","value":"{{.value}}"}'
                                value={a.bodyTemplate ?? ""}
                                onChange={(e) => updateAction(i, { bodyTemplate: e.target.value })}
                              />
                            </div>
                          </div>
                        )}

                        {a.type === "set_desired" && (
                          <div className="space-y-2">
                            <Label>Desired state (JSON)</Label>
                            <Textarea
                              className="font-mono"
                              rows={3}
                              placeholder='{"temperature": 80}'
                              value={a.desiredJson ?? ""}
                              onChange={(e) => updateAction(i, { desiredJson: e.target.value })}
                            />
                          </div>
                        )}

                        {a.type === "downlink" && (
                          <div className="space-y-3">
                            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                              <div className="space-y-2">
                                <Label>Kind</Label>
                                <Select value={a.kind ?? "property"} onValueChange={(v) => updateAction(i, { kind: v })}>
                                  <SelectTrigger>
                                    <SelectValue />
                                  </SelectTrigger>
                                  <SelectContent>
                                    {downlinkKinds.map((k) => (
                                      <SelectItem key={k.value} value={k.value}>
                                        {k.label}
                                      </SelectItem>
                                    ))}
                                  </SelectContent>
                                </Select>
                              </div>
                              <div className="space-y-2">
                                <Label>Identifier</Label>
                                <Input
                                  value={a.identifier ?? ""}
                                  onChange={(e) => updateAction(i, { identifier: e.target.value })}
                                />
                              </div>
                              <div className="space-y-2">
                                <Label>Target device key (optional)</Label>
                                <Input
                                  value={a.deviceKey ?? ""}
                                  onChange={(e) => updateAction(i, { deviceKey: e.target.value })}
                                />
                              </div>
                            </div>
                            <div className="space-y-2">
                              <Label>Payload (JSON)</Label>
                              <Textarea
                                className="font-mono"
                                rows={3}
                                value={a.payloadJson ?? ""}
                                onChange={(e) => updateAction(i, { payloadJson: e.target.value })}
                              />
                            </div>
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>

            <div className="space-y-3 rounded-lg border bg-muted/30 p-3 text-xs">
              <div className="font-semibold">CEL variables</div>
              <div className="space-y-1">
                {(reference.data?.variables ?? []).map((v) => (
                  <div key={v.name}>
                    <code className="font-mono text-primary">{v.name}</code>
                    <span className="text-muted-foreground"> : {v.type}</span>
                  </div>
                ))}
              </div>
              <div className="font-semibold pt-2">Examples</div>
              <div className="space-y-1">
                {(reference.data?.examples ?? []).map((ex) => (
                  <button
                    key={ex.name}
                    className="block w-full rounded bg-background p-2 text-left hover:bg-accent"
                    onClick={() => setDraft({ ...draft, condition: ex.condition })}
                  >
                    <div className="font-medium">{ex.name}</div>
                    <code className="text-[11px] text-muted-foreground">{ex.condition}</code>
                  </button>
                ))}
              </div>
              <p className="pt-2 text-muted-foreground">
                Notification templates support <code>{"{{.deviceKey}}"}</code>, <code>{"{{.value}}"}</code>,{" "}
                <code>{"{{.identifier}}"}</code>, <code>{"{{.now}}"}</code>.
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => save.mutate()} disabled={save.isPending || !draft.name || !draft.condition}>
              {editingId ? "Save" : "Create"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!logRule} onOpenChange={(o) => !o && setLogRule(null)}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{t("rules.logs")} — {logRule?.name}</DialogTitle>
          </DialogHeader>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Device</TableHead>
                <TableHead>Identifier</TableHead>
                <TableHead>Result</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(logs.data ?? []).length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="py-6 text-center text-muted-foreground">
                    {t("rules.noExecutions")}
                  </TableCell>
                </TableRow>
              )}
              {(logs.data ?? []).map((l) => (
                <TableRow key={l.id}>
                  <TableCell className="text-xs">{formatTime(l.occurredAt)}</TableCell>
                  <TableCell className="text-xs">#{l.deviceId}</TableCell>
                  <TableCell className="font-mono text-xs">{l.identifier}</TableCell>
                  <TableCell>
                    <Badge variant={l.success ? "success" : "destructive"}>{l.success ? "ok" : "error"}</Badge>
                    {l.error && <div className="mt-1 text-xs text-destructive">{l.error}</div>}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DialogContent>
      </Dialog>
    </div>
  );
}
