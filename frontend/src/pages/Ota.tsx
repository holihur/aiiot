import { useState } from "react";
import { useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Rocket } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
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
import { useProjectRole } from "@/lib/useProjectRole";
import { formatTime } from "@/lib/utils";

const statusVariant: Record<string, "default" | "success" | "destructive" | "warning" | "secondary"> = {
  succeeded: "success",
  failed: "destructive",
  running: "default",
  pending: "secondary",
  canceled: "secondary",
};

export default function OtaPage() {
  const projectId = Number(useParams().projectId);
  const qc = useQueryClient();
  const { t } = useI18n();
  const { canWrite } = useProjectRole(projectId);
  const { data: tasks, isLoading } = useQuery({
    queryKey: ["otaTasks", projectId],
    queryFn: () => api.listOtaTasks(projectId),
    refetchInterval: 5000,
  });
  const products = useQuery({ queryKey: ["products"], queryFn: () => api.listProducts() });
  const workspaces = useQuery({ queryKey: ["workspaces", projectId], queryFn: () => api.listWorkspaces(projectId) });

  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ productId: "", firmwareId: "", workspaceId: "all", name: "", batchSize: "" });
  const firmwares = useQuery({
    queryKey: ["firmwares", form.productId],
    queryFn: () => api.listFirmwares(Number(form.productId)),
    enabled: !!form.productId,
  });

  const [detailId, setDetailId] = useState<number | null>(null);
  const detail = useQuery({
    queryKey: ["otaTask", detailId],
    queryFn: () => api.getOtaTask(detailId!),
    enabled: !!detailId,
    refetchInterval: 3000,
  });

  const create = useMutation({
    mutationFn: () =>
      api.createOtaTask(projectId, {
        firmwareId: Number(form.firmwareId),
        name: form.name || undefined,
        productId: Number(form.productId),
        workspaceId: form.workspaceId === "all" ? undefined : Number(form.workspaceId),
        batchSize: form.batchSize ? Number(form.batchSize) : undefined,
      }),
    onSuccess: () => {
      toast.success("OTA task created and dispatched");
      setOpen(false);
      setForm({ productId: "", firmwareId: "", workspaceId: "all", name: "", batchSize: "" });
      void qc.invalidateQueries({ queryKey: ["otaTasks", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const rollback = useMutation({
    mutationFn: (id: number) => api.rollbackOtaTask(id),
    onSuccess: () => {
      toast.success("Rollback rollout created");
      void qc.invalidateQueries({ queryKey: ["otaTasks", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("ota.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("ota.subtitle")}</p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="w-full sm:w-auto" disabled={!canWrite}>
              <Plus className="h-4 w-4" /> {t("ota.new")}
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("ota.createTitle")}</DialogTitle>
              <DialogDescription>{t("ota.createDesc")}</DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>{t("ota.product")}</Label>
                <Select value={form.productId} onValueChange={(v) => setForm({ ...form, productId: v, firmwareId: "" })}>
                  <SelectTrigger>
                    <SelectValue placeholder={t("ota.selectProduct")} />
                  </SelectTrigger>
                  <SelectContent>
                    {(products.data ?? []).map((p) => (
                      <SelectItem key={p.id} value={String(p.id)}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("ota.firmware")}</Label>
                <Select value={form.firmwareId} onValueChange={(v) => setForm({ ...form, firmwareId: v })}>
                  <SelectTrigger>
                    <SelectValue placeholder={t("ota.selectFirmware")} />
                  </SelectTrigger>
                  <SelectContent>
                    {(firmwares.data ?? []).map((f) => (
                      <SelectItem key={f.id} value={String(f.id)}>
                        {f.version} ({f.fileName})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("ota.targetWorkspace")}</Label>
                <Select value={form.workspaceId} onValueChange={(v) => setForm({ ...form, workspaceId: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t("ota.allWorkspaces")}</SelectItem>
                    {(workspaces.data ?? []).map((w) => (
                      <SelectItem key={w.id} value={String(w.id)}>
                        {w.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("ota.nameOptional")}</Label>
                <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
              </div>
              <div className="space-y-2">
                <Label>{t("ota.batchSize")}</Label>
                <Input
                  type="number"
                  value={form.batchSize}
                  onChange={(e) => setForm({ ...form, batchSize: e.target.value })}
                  placeholder="0"
                />
                <p className="text-xs text-muted-foreground">{t("ota.batchHint")}</p>
              </div>
            </div>
            <DialogFooter>
              <Button
                onClick={() => create.mutate()}
                disabled={create.isPending || !form.productId || !form.firmwareId}
              >
                {t("ota.dispatch")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      <ProjectNav projectId={projectId} />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Rocket className="h-4 w-4" /> {t("ota.rollouts")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("common.name")}</TableHead>
                <TableHead>{t("ota.firmware")}</TableHead>
                <TableHead>{t("common.status")}</TableHead>
                <TableHead>{t("ota.progress")}</TableHead>
                <TableHead>{t("ota.wave")}</TableHead>
                <TableHead>{t("ota.created")}</TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading && (
                <TableRow>
                  <TableCell colSpan={7} className="py-8 text-center text-muted-foreground">
                    {t("common.loading")}
                  </TableCell>
                </TableRow>
              )}
              {(tasks ?? []).length === 0 && !isLoading && (
                <TableRow>
                  <TableCell colSpan={7} className="py-8 text-center text-muted-foreground">
                    {t("ota.empty")}
                  </TableCell>
                </TableRow>
              )}
              {(tasks ?? []).map((task) => (
                <TableRow key={task.id} className="cursor-pointer" onClick={() => setDetailId(task.id)}>
                  <TableCell className="font-medium">{task.name}</TableCell>
                  <TableCell>{task.firmware?.version ?? task.firmwareId}</TableCell>
                  <TableCell>
                    <Badge variant={statusVariant[task.status] ?? "secondary"}>{task.status}</Badge>
                  </TableCell>
                  <TableCell>
                    {task.succeeded}/{task.total}
                    {task.failed > 0 && <span className="text-destructive"> ({task.failed} failed)</span>}
                  </TableCell>
                  <TableCell className="text-xs">
                    {task.batchSize > 0 ? `${task.currentWave + 1}/${Math.ceil(task.total / task.batchSize)}` : "-"}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{formatTime(task.createdAt)}</TableCell>
                  <TableCell className="text-right">
                    {(task.status === "succeeded" || task.status === "failed") && (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={!canWrite}
                        onClick={(e) => {
                          e.stopPropagation();
                          rollback.mutate(task.id);
                        }}
                      >
                        {t("ota.rollback")}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Dialog open={!!detailId} onOpenChange={(o) => !o && setDetailId(null)}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{detail.data?.name ?? "Task"}</DialogTitle>
            <DialogDescription>
              Firmware {detail.data?.firmware?.version} · status {detail.data?.status}
            </DialogDescription>
          </DialogHeader>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("ota.device")}</TableHead>
                <TableHead>{t("ota.wave")}</TableHead>
                <TableHead>{t("common.status")}</TableHead>
                <TableHead>{t("ota.progress")}</TableHead>
                <TableHead>{t("ota.message")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(detail.data?.devices ?? []).map((d) => (
                <TableRow key={d.id}>
                  <TableCell className="font-medium">{d.device?.name ?? `#${d.deviceId}`}</TableCell>
                  <TableCell>{d.wave + 1}</TableCell>
                  <TableCell>
                    <Badge variant={statusVariant[d.status] ?? "secondary"}>{d.status}</Badge>
                  </TableCell>
                  <TableCell>{d.progress}%</TableCell>
                  <TableCell className="text-xs text-muted-foreground">{d.message || "-"}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DialogContent>
      </Dialog>
    </div>
  );
}
