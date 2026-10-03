import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Cpu, Send } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ListSkeleton, TableSkeletonRows } from "@/components/ui/skeleton";
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
import { SecretDialog } from "@/components/SecretDialog";
import { useProjectRole } from "@/lib/useProjectRole";
import { formatTime } from "@/lib/utils";

export default function DevicesPage() {
  const projectId = Number(useParams().projectId);
  const qc = useQueryClient();
  const { t } = useI18n();
  const { canWrite } = useProjectRole(projectId);
  const [productFilter, setProductFilter] = useState<string>("all");
  const [workspaceFilter, setWorkspaceFilter] = useState<string>("all");
  const [keyword, setKeyword] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize] = useState(20);
  const [sort, setSort] = useState("id");
  const [order, setOrder] = useState<"asc" | "desc">("desc");

  const products = useQuery({ queryKey: ["products"], queryFn: () => api.listProducts() });
  const workspaces = useQuery({ queryKey: ["workspaces", projectId], queryFn: () => api.listWorkspaces(projectId) });

  const devices = useQuery({
    queryKey: ["devices", projectId, productFilter, workspaceFilter, keyword, page, pageSize, sort, order],
    queryFn: () =>
      api.listDevicesPaged(projectId, {
        productId: productFilter === "all" ? undefined : Number(productFilter),
        workspaceId: workspaceFilter === "all" ? undefined : Number(workspaceFilter),
        keyword: keyword || undefined,
        page,
        pageSize,
        sort,
        order,
      }),
  });
  const items = devices.data?.items ?? [];
  const total = devices.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  function toggleSort(col: string) {
    if (sort === col) setOrder((o) => (o === "asc" ? "desc" : "asc"));
    else {
      setSort(col);
      setOrder("asc");
    }
    setPage(1);
  }

  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: "", productId: "", workspaceId: "" });

  const [selected, setSelected] = useState<number[]>([]);
  const [revealed, setRevealed] = useState<{ secret: string; name: string } | null>(null);
  const [batchOpen, setBatchOpen] = useState(false);
  const [batchForm, setBatchForm] = useState({
    action: "command",
    kind: "property",
    identifier: "",
    json: '{\n  "reportInterval": 60\n}',
  });

  function toggleSelect(id: number) {
    setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));
  }

  function toggleAll(ids: number[]) {
    setSelected((s) => (s.length === ids.length ? [] : ids));
  }

  const createDevice = useMutation({
    mutationFn: () =>
      api.createDevice(Number(form.productId), {
        name: form.name,
        workspaceId: Number(form.workspaceId),
      }),
    onSuccess: (res) => {
      setRevealed({ secret: res.secret, name: res.device.name || res.device.key });
      toast.success(t("device.created"));
      setOpen(false);
      setForm({ name: "", productId: "", workspaceId: "" });
      void qc.invalidateQueries({ queryKey: ["devices"] });
    },
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
        deviceIds: selected,
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
      setSelected([]);
      void qc.invalidateQueries({ queryKey: ["devices"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight whitespace-nowrap">{t("devices.title")}</h1>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button disabled={!canWrite}>
              <Plus className="h-4 w-4" /> {t("devices.add")}
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("devices.createTitle")}</DialogTitle>
              <DialogDescription>{t("devices.createDesc")}</DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>{t("common.name")}</Label>
                <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
                <p className="text-xs text-muted-foreground">{t("devices.keyHint")}</p>
              </div>
              <div className="space-y-2">
                <Label>{t("ota.product")}</Label>
                <Select value={form.productId} onValueChange={(v) => setForm({ ...form, productId: v })}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select product" />
                  </SelectTrigger>
                  <SelectContent>
                    {(products.data ?? []).map((p) => (
                      <SelectItem key={p.id} value={String(p.id)}>
                        {p.name} ({p.protocol})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("project.workspaces")}</Label>
                <Select value={form.workspaceId} onValueChange={(v) => setForm({ ...form, workspaceId: v })}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select workspace" />
                  </SelectTrigger>
                  <SelectContent>
                    {(workspaces.data ?? []).map((w) => (
                      <SelectItem key={w.id} value={String(w.id)}>
                        {w.name} ({w.key})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <DialogFooter>
              <Button
                onClick={() => createDevice.mutate()}
                disabled={createDevice.isPending || !form.name || !form.productId || !form.workspaceId}
              >
                {t("common.create")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      <ProjectNav projectId={projectId} />

      <div className="flex flex-wrap gap-3">
        <Select value={productFilter} onValueChange={setProductFilter}>
          <SelectTrigger className="w-full sm:w-48">
            <SelectValue placeholder="Product" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("devices.allProducts")}</SelectItem>
            {(products.data ?? []).map((p) => (
              <SelectItem key={p.id} value={String(p.id)}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={workspaceFilter} onValueChange={setWorkspaceFilter}>
          <SelectTrigger className="w-full sm:w-48">
            <SelectValue placeholder="Workspace" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("devices.allWorkspaces")}</SelectItem>
            {(workspaces.data ?? []).map((w) => (
              <SelectItem key={w.id} value={String(w.id)}>
                {w.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          className="w-full sm:w-56"
          placeholder={t("devices.searchPlaceholder")}
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
        />
        <Button variant="outline" disabled={!canWrite || selected.length === 0} onClick={() => setBatchOpen(true)}>
          <Send className="h-4 w-4" /> {t("devices.batch")} ({selected.length})
        </Button>
      </div>

      <Card>
        <CardContent className="pt-5">
          <div className="hidden md:block">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">
                  <input
                    type="checkbox"
                    className="h-4 w-4"
                    checked={items.length > 0 && selected.length === items.length}
                    onChange={() => toggleAll(items.map((d) => d.id))}
                  />
                </TableHead>
                <TableHead className="cursor-pointer select-none" onClick={() => toggleSort("name")}>
                  {t("common.name")} {sort === "name" ? (order === "asc" ? "↑" : "↓") : ""}
                </TableHead>
                <TableHead>{t("common.key")}</TableHead>
                <TableHead>{t("ota.product")}</TableHead>
                <TableHead>{t("project.workspaces")}</TableHead>
                <TableHead className="cursor-pointer select-none" onClick={() => toggleSort("online")}>
                  {t("common.status")} {sort === "online" ? (order === "asc" ? "↑" : "↓") : ""}
                </TableHead>
                <TableHead className="cursor-pointer select-none" onClick={() => toggleSort("last_seen_at")}>
                  {t("devices.lastSeen")} {sort === "last_seen_at" ? (order === "asc" ? "↑" : "↓") : ""}
                </TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {devices.isLoading && <TableSkeletonRows cols={8} />}
              {!devices.isLoading && items.length === 0 && (
                <TableRow>
                  <TableCell colSpan={8} className="py-10 text-center text-muted-foreground">
                    <Cpu className="mx-auto mb-2 h-6 w-6" />
                    {t("devices.empty")}
                  </TableCell>
                </TableRow>
              )}
              {items.map((d) => (
                <TableRow key={d.id}>
                  <TableCell>
                    <input
                      type="checkbox"
                      className="h-4 w-4"
                      checked={selected.includes(d.id)}
                      onChange={() => toggleSelect(d.id)}
                    />
                  </TableCell>
                  <TableCell className="font-medium">{d.name}</TableCell>
                  <TableCell className="font-mono text-xs">{d.key}</TableCell>
                  <TableCell className="max-w-[10rem] truncate">{d.product?.name}</TableCell>
                  <TableCell className="max-w-[10rem] truncate">{d.workspace?.name}</TableCell>
                  <TableCell>
                    <Badge variant={d.online ? "success" : "secondary"}>{d.online ? "online" : "offline"}</Badge>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{formatTime(d.lastSeenAt)}</TableCell>
                  <TableCell className="text-right">
                    <Link to={`/devices/${d.id}`} className="text-sm text-primary hover:underline">
                      {t("devices.open")}
                    </Link>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          </div>
          <div className="space-y-2 md:hidden">
            {devices.isLoading && <ListSkeleton rows={5} />}
            {!devices.isLoading && items.length === 0 && (
              <p className="py-8 text-center text-sm text-muted-foreground">{t("devices.empty")}</p>
            )}
            {items.map((d) => (
              <div key={d.id} className="cv-auto rounded-lg border p-3 text-sm">
                <div className="flex items-center justify-between">
                  <span className="font-medium">{d.name}</span>
                  <Badge variant={d.online ? "success" : "secondary"}>{d.online ? "online" : "offline"}</Badge>
                </div>
                <div className="mt-1 font-mono text-xs text-muted-foreground">{d.key}</div>
                <div className="mt-1 text-xs text-muted-foreground">
                  {d.product?.name} · {formatTime(d.lastSeenAt)}
                </div>
                <div className="mt-2 flex items-center justify-between">
                  <label className="flex items-center gap-2 text-xs">
                    <input
                      type="checkbox"
                      className="h-4 w-4"
                      checked={selected.includes(d.id)}
                      onChange={() => toggleSelect(d.id)}
                      aria-label={`Select ${d.name}`}
                    />
                    {t("devices.select")}
                  </label>
                  <Link to={`/devices/${d.id}`} className="text-sm text-primary hover:underline">
                    {t("devices.open")}
                  </Link>
                </div>
              </div>
            ))}
          </div>
          <div className="mt-3 flex items-center justify-between text-sm text-muted-foreground">
            <span>
              {total === 0 ? "0" : `${(page - 1) * pageSize + 1}–${Math.min(page * pageSize, total)}`} of {total}
            </span>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
                Prev
              </Button>
              <span>
                {page} / {totalPages}
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={page >= totalPages}
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              >
                Next
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <Dialog open={batchOpen} onOpenChange={setBatchOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("devices.batchTitle", { count: selected.length })}</DialogTitle>
            <DialogDescription>{t("devices.batchDesc")}</DialogDescription>
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
              <Label>{batchForm.action === "set_desired" ? t("devices.desiredState") : t("devices.payload")}</Label>
              <Textarea
                className="font-mono"
                rows={5}
                value={batchForm.json}
                onChange={(e) => setBatchForm({ ...batchForm, json: e.target.value })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setBatchOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => batch.mutate()} disabled={batch.isPending}>
              <Send className="h-4 w-4" /> {t("devices.applyTo", { count: selected.length })}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <SecretDialog
        open={!!revealed}
        onOpenChange={(o) => !o && setRevealed(null)}
        secret={revealed?.secret ?? ""}
        subject={revealed?.name}
        filename={revealed?.name}
      />
    </div>
  );
}
