import { useState } from "react";
import { useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderPlus, Plus, Send, Trash2, Users } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ListSkeleton } from "@/components/ui/skeleton";
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
import { useProjectRole } from "@/lib/useProjectRole";

export default function GroupsPage() {
  const projectId = Number(useParams().projectId);
  const qc = useQueryClient();
  const { t } = useI18n();
  const { canWrite, canAdmin } = useProjectRole(projectId);
  const groups = useQuery({ queryKey: ["groups", projectId], queryFn: () => api.listDeviceGroups(projectId) });
  const devices = useQuery({ queryKey: ["devices", projectId], queryFn: () => api.listDevices(projectId) });

  const [selected, setSelected] = useState<number | null>(null);
  const groupDevices = useQuery({
    queryKey: ["groupDevices", selected],
    queryFn: () => api.listGroupDevices(selected!),
    enabled: !!selected,
  });

  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState({ name: "", description: "" });
  const [addOpen, setAddOpen] = useState(false);
  const [toAdd, setToAdd] = useState<number[]>([]);
  const [batchOpen, setBatchOpen] = useState(false);
  const [batchForm, setBatchForm] = useState({ action: "command", kind: "property", identifier: "", json: "{}" });

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["groups", projectId] });
    void qc.invalidateQueries({ queryKey: ["groupDevices", selected] });
  };

  const create = useMutation({
    mutationFn: () => api.createDeviceGroup(projectId, form),
    onSuccess: () => {
      toast.success("Group created");
      setCreateOpen(false);
      setForm({ name: "", description: "" });
      invalidate();
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const removeGroup = useMutation({
    mutationFn: (id: number) => api.deleteDeviceGroup(id),
    onSuccess: () => {
      toast.success("Group deleted");
      setSelected(null);
      invalidate();
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const addDevices = useMutation({
    mutationFn: () => api.addGroupDevices(selected!, toAdd),
    onSuccess: (r) => {
      toast.success(`Added ${r.added} device(s)`);
      setAddOpen(false);
      setToAdd([]);
      invalidate();
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const removeDevice = useMutation({
    mutationFn: (deviceId: number) => api.removeGroupDevice(selected!, deviceId),
    onSuccess: () => invalidate(),
    onError: (e: Error) => toast.error(e.message),
  });

  const batch = useMutation({
    mutationFn: () => {
      let parsed: unknown = {};
      try {
        parsed = JSON.parse(batchForm.json || "{}");
      } catch {
        throw new Error("Payload must be valid JSON");
      }
      return api.batchCommand(projectId, {
        groupId: selected!,
        action: batchForm.action,
        kind: batchForm.kind,
        identifier: batchForm.identifier || undefined,
        payload: parsed,
        desired: batchForm.action === "set_desired" ? (parsed as Record<string, unknown>) : undefined,
      });
    },
    onSuccess: (r) => {
      toast.success(`Batch done: ${r.succeeded} ok, ${r.failed} failed`);
      setBatchOpen(false);
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight whitespace-nowrap">{t("groups.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("groups.subtitle")}</p>
        </div>
        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogTrigger asChild>
            <Button className="w-full sm:w-auto" disabled={!canWrite}>
              <FolderPlus className="h-4 w-4" /> {t("groups.new")}
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("groups.createTitle")}</DialogTitle>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>{t("common.name")}</Label>
                <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
              </div>
              <div className="space-y-2">
                <Label>{t("common.description")}</Label>
                <Textarea value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
              </div>
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

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[320px_1fr]">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Users className="h-4 w-4" /> {t("groups.members")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {groups.isLoading && <ListSkeleton rows={4} />}
            {!groups.isLoading && (groups.data ?? []).length === 0 && <p className="text-sm text-muted-foreground">{t("groups.empty")}</p>}
            {(groups.data ?? []).map((g) => (
              <button
                key={g.id}
                onClick={() => setSelected(g.id)}
                className={`flex w-full items-center justify-between rounded-lg border p-3 text-left text-sm ${
                  selected === g.id ? "border-primary bg-primary/5" : "hover:bg-accent"
                }`}
              >
                <div>
                  <div className="font-medium">{g.name}</div>
                  <div className="text-xs text-muted-foreground">{t("groups.count", { count: g.deviceCount })}</div>
                </div>
                {canAdmin && (
                  <Trash2
                    className="h-4 w-4 text-destructive"
                    onClick={(e) => {
                      e.stopPropagation();
                      removeGroup.mutate(g.id);
                    }}
                  />
                )}
              </button>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3">
            <div>
              <CardTitle className="text-base">{t("groups.members")}</CardTitle>
              <CardDescription>{selected ? t("groups.membersDesc") : t("groups.selectPrompt")}</CardDescription>
            </div>
            {selected && (
              <div className="flex gap-2">
                <Button size="sm" variant="outline" onClick={() => setAddOpen(true)} disabled={!canWrite}>
                  <Plus className="h-4 w-4" /> {t("groups.addDevices")}
                </Button>
                <Button size="sm" onClick={() => setBatchOpen(true)} disabled={!canWrite}>
                  <Send className="h-4 w-4" /> {t("groups.batch")}
                </Button>
              </div>
            )}
          </CardHeader>
          <CardContent>
            <div className="hidden md:block">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("common.name")}</TableHead>
                  <TableHead>{t("common.key")}</TableHead>
                  <TableHead>{t("common.status")}</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(groupDevices.data ?? []).length === 0 && (
                  <TableRow>
                    <TableCell colSpan={4} className="py-8 text-center text-muted-foreground">
                      {selected ? t("groups.noDevices") : t("groups.selectHint")}
                    </TableCell>
                  </TableRow>
                )}
                {(groupDevices.data ?? []).map((d) => (
                  <TableRow key={d.id}>
                    <TableCell className="font-medium">{d.name}</TableCell>
                    <TableCell className="font-mono text-xs">{d.key}</TableCell>
                    <TableCell>
                      <Badge variant={d.online ? "success" : "secondary"}>{d.online ? "online" : "offline"}</Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <ConfirmButton
                        title={t("groups.remove")}
                        description={t("groups.removeDesc")}
                        confirmLabel={t("groups.removeLabel")}
                        onConfirm={() => removeDevice.mutateAsync(d.id)}
                      >
                        <Button variant="ghost" size="icon" aria-label="Remove from group">
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </ConfirmButton>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            </div>

            {/* Mobile cards */}
            <div className="space-y-2 md:hidden">
              {(groupDevices.data ?? []).length === 0 && (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  {selected ? t("groups.noDevices") : t("groups.selectHint")}
                </p>
              )}
              {(groupDevices.data ?? []).map((d) => (
                <div key={d.id} className="rounded-lg border p-3">
                  <div className="flex min-w-0 items-center justify-between gap-2">
                    <span className="min-w-0 truncate font-medium">{d.name}</span>
                    <Badge variant={d.online ? "success" : "secondary"} className="shrink-0">
                      {d.online ? "online" : "offline"}
                    </Badge>
                  </div>
                  <div className="mt-1 font-mono text-xs text-muted-foreground">{d.key}</div>
                  <div className="mt-2 flex justify-end">
                    <ConfirmButton
                      title={t("groups.remove")}
                      description={t("groups.removeDesc")}
                      confirmLabel={t("groups.removeLabel")}
                      onConfirm={() => removeDevice.mutateAsync(d.id)}
                    >
                      <Button variant="ghost" size="icon" aria-label="Remove from group">
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </ConfirmButton>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("groups.addTitle")}</DialogTitle>
            <DialogDescription>{t("groups.addDesc")}</DialogDescription>
          </DialogHeader>
          <div className="max-h-72 space-y-1 overflow-y-auto rounded border p-2">
            {(devices.data ?? []).map((d) => (
              <label key={d.id} className="flex items-center gap-2 rounded px-2 py-1 text-sm hover:bg-accent">
                <input
                  type="checkbox"
                  className="h-4 w-4"
                  checked={toAdd.includes(d.id)}
                  onChange={() =>
                    setToAdd((s) => (s.includes(d.id) ? s.filter((x) => x !== d.id) : [...s, d.id]))
                  }
                />
                <span className="font-medium">{d.name}</span>
                <span className="font-mono text-xs text-muted-foreground">{d.key}</span>
              </label>
            ))}
          </div>
          <DialogFooter>
            <Button onClick={() => addDevices.mutate()} disabled={addDevices.isPending || toAdd.length === 0}>
              {t("groups.addCount", { count: toAdd.length })}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={batchOpen} onOpenChange={setBatchOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("groups.batchTitle")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("devices.operation")}</Label>
              <Select value={batchForm.action} onValueChange={(v) => setBatchForm({ ...batchForm, action: v })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="command">{t("devices.downlinkCommand")}</SelectItem>
                  <SelectItem value="set_desired">{t("devices.setDesired")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {batchForm.action === "command" && (
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label>{t("devices.kind")}</Label>
                  <Select value={batchForm.kind} onValueChange={(v) => setBatchForm({ ...batchForm, kind: v })}>
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
                  <Input
                    value={batchForm.identifier}
                    onChange={(e) => setBatchForm({ ...batchForm, identifier: e.target.value })}
                  />
                </div>
              </div>
            )}
            <div className="space-y-2">
              <Label>{t("devices.payload")}</Label>
              <Textarea
                className="font-mono"
                rows={5}
                value={batchForm.json}
                onChange={(e) => setBatchForm({ ...batchForm, json: e.target.value })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button onClick={() => batch.mutate()} disabled={batch.isPending}>
              <Send className="h-4 w-4" /> {t("groups.applyToGroup")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
