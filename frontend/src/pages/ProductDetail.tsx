import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Plus, Rocket, Save, Trash2, Pencil, Upload, History } from "lucide-react";
import { toast } from "sonner";
import { api, uploadFirmware, type ModelParam, type ThingModelElement } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { useAuth } from "@/lib/auth";
import { ConfirmButton } from "@/components/ConfirmButton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
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
import { Switch } from "@/components/ui/switch";
import { formatTime } from "@/lib/utils";

const dataTypes = ["int32", "int64", "float", "double", "bool", "text", "date", "enum", "json", "struct", "array"];
const protocolVariant: Record<string, "success" | "warning" | "secondary"> = {
  mqtt: "success",
  coap: "warning",
  custom: "secondary",
};

// specParams / setSpecParams manage service inputs / event fields stored in
// the element's specs JSON (the backend passes them through).
function specParams(el: ThingModelElement, key: string): ModelParam[] {
  const v = el.specs?.[key];
  return Array.isArray(v) ? (v as ModelParam[]) : [];
}
function setSpecParams(
  el: ThingModelElement,
  key: string,
  items: ModelParam[],
  setDraft: React.Dispatch<React.SetStateAction<ThingModelElement>>,
) {
  setDraft({ ...el, specs: { ...(el.specs ?? {}), [key]: items } });
}

function ParamEditor({ items, onItems }: { items: ModelParam[]; onItems: (a: ModelParam[]) => void }) {
  const { t } = useI18n();
  const update = (i: number, patch: Partial<ModelParam>) =>
    onItems(items.map((it, idx) => (idx === i ? { ...it, ...patch } : it)));
  return (
    <div className="space-y-2">
      {items.map((p, i) => (
        <div key={i} className="space-y-2 rounded-md border p-2">
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-[1fr_9rem_auto_auto] sm:items-center">
            <Input placeholder={t("product.paramName")} value={p.name} onChange={(e) => update(i, { name: e.target.value })} />
            <Select value={p.dataType} onValueChange={(v) => update(i, { dataType: v })}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {dataTypes.map((dt) => (
                  <SelectItem key={dt} value={dt}>
                    {dt}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              <Switch checked={!!p.required} onCheckedChange={(v) => update(i, { required: v })} />
              {t("product.paramRequired")}
            </label>
            <Button variant="ghost" size="icon" onClick={() => onItems(items.filter((_, idx) => idx !== i))}>
              <Trash2 className="h-4 w-4 text-destructive" />
            </Button>
          </div>
          {p.dataType === "enum" && (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-[11px] text-muted-foreground">{t("product.enumValues")}</span>
              <Input
                className="h-8 flex-1 min-w-[10rem] font-mono text-xs"
                placeholder="a,b,c"
                value={(p.enumValues ?? []).join(", ")}
                onChange={(e) =>
                  update(i, {
                    enumValues: e.target.value
                      .split(",")
                      .map((x) => x.trim())
                      .filter(Boolean),
                  })
                }
              />
            </div>
          )}
          <div className="flex flex-wrap items-end gap-2">
            <div className="space-y-0.5">
              <span className="text-[11px] text-muted-foreground">{t("product.unit")}</span>
              <Input
                className="h-8 w-20"
                value={p.unit ?? ""}
                onChange={(e) => update(i, { unit: e.target.value })}
              />
            </div>
            <div className="space-y-0.5">
              <span className="text-[11px] text-muted-foreground">{t("product.min")}</span>
              <Input
                className="h-8 w-24"
                type="number"
                value={p.min ?? ""}
                onChange={(e) => update(i, { min: e.target.value === "" ? undefined : Number(e.target.value) })}
              />
            </div>
            <div className="space-y-0.5">
              <span className="text-[11px] text-muted-foreground">{t("product.max")}</span>
              <Input
                className="h-8 w-24"
                type="number"
                value={p.max ?? ""}
                onChange={(e) => update(i, { max: e.target.value === "" ? undefined : Number(e.target.value) })}
              />
            </div>
          </div>
        </div>
      ))}
      <Button size="sm" variant="outline" onClick={() => onItems([...items, { name: "", dataType: "double", required: true }])}>
        <Plus className="h-4 w-4" /> {t("product.addParam")}
      </Button>
    </div>
  );
}

function EnumEditor({ values, onChange }: { values: string[]; onChange: (v: string[]) => void }) {
  const { t } = useI18n();
  return (
    <div className="space-y-2">
      {values.map((v, i) => (
        <div key={i} className="flex items-center gap-2">
          <Input value={v} onChange={(e) => onChange(values.map((x, idx) => (idx === i ? e.target.value : x)))} />
          <Button variant="ghost" size="icon" onClick={() => onChange(values.filter((_, idx) => idx !== i))}>
            <Trash2 className="h-4 w-4 text-destructive" />
          </Button>
        </div>
      ))}
      <Button size="sm" variant="outline" onClick={() => onChange([...values, ""])}>
        <Plus className="h-4 w-4" /> {t("product.addEnum")}
      </Button>
    </div>
  );
}

const emptyElement: ThingModelElement = {
  type: "property",
  identifier: "",
  name: "",
  dataType: "double",
  accessMode: "rw",
  unit: "",
  description: "",
};

export default function ProductDetailPage() {
  const productId = Number(useParams().productId);
  const qc = useQueryClient();
  const { t } = useI18n();
  const { user } = useAuth();
  const product = useQuery({ queryKey: ["product", productId], queryFn: () => api.getProduct(productId) });
  const canManage = !!user && (user.systemRole === "admin" || product.data?.createdBy === user.id);
  const thingModel = useQuery({ queryKey: ["thingModel", productId], queryFn: () => api.getThingModel(productId) });

  const [elements, setElements] = useState<ThingModelElement[]>([]);
  const [version, setVersion] = useState("1.0.0");
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (thingModel.data) {
      setElements(thingModel.data.elements ?? []);
      setVersion(thingModel.data.version ?? "1.0.0");
      setDirty(false);
    }
  }, [thingModel.data]);

  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editForm, setEditForm] = useState({ name: "", description: "" });
  const saveInfo = useMutation({
    mutationFn: () =>
      api.updateProduct(productId, {
        name: editForm.name,
        description: editForm.description,
        protocol: product.data?.protocol,
      }),
    onSuccess: () => {
      toast.success("Product updated");
      setEditOpen(false);
      void qc.invalidateQueries({ queryKey: ["product", productId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const removeProduct = useMutation({
    mutationFn: () => api.deleteProduct(productId),
    onSuccess: () => {
      toast.success("Product deleted");
      navigate("/products");
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [draft, setDraft] = useState<ThingModelElement>(emptyElement);

  const save = useMutation({
    mutationFn: () => api.putThingModel(productId, { version, elements }),
    onSuccess: () => {
      toast.success("Thing model saved");
      setDirty(false);
      void qc.invalidateQueries({ queryKey: ["thingModel", productId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  // Firmware management
  const firmwares = useQuery({
    queryKey: ["firmwares", productId],
    queryFn: () => api.listFirmwares(productId),
  });
  const fileInput = useRef<HTMLInputElement>(null);
  const [fwOpen, setFwOpen] = useState(false);
  const [fwForm, setFwForm] = useState({ version: "", name: "" });
  const upload = useMutation({
    mutationFn: async () => {
      const file = fileInput.current?.files?.[0];
      if (!file) throw new Error("Select a firmware file");
      if (!fwForm.version) throw new Error("Version is required");
      return uploadFirmware(productId, file, { version: fwForm.version, name: fwForm.name });
    },
    onSuccess: () => {
      toast.success("Firmware uploaded");
      setFwOpen(false);
      setFwForm({ version: "", name: "" });
      if (fileInput.current) fileInput.current.value = "";
      void qc.invalidateQueries({ queryKey: ["firmwares", productId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const deleteFw = useMutation({
    mutationFn: (id: number) => api.deleteFirmware(id),
    onSuccess: () => {
      toast.success("Firmware deleted");
      void qc.invalidateQueries({ queryKey: ["firmwares", productId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const [versionOpen, setVersionOpen] = useState(false);
  const versions = useQuery({
    queryKey: ["thingModelVersions", productId],
    queryFn: () => api.listThingModelVersions(productId),
    enabled: versionOpen,
  });
  const publishModel = useMutation({
    mutationFn: () => api.publishThingModel(productId),
    onSuccess: (r) => {
      toast.success(`Published v${r.version}`);
      void qc.invalidateQueries({ queryKey: ["thingModel", productId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const rollbackModel = useMutation({
    mutationFn: (vid: number) => api.rollbackThingModelVersion(productId, vid),
    onSuccess: () => {
      toast.success("Rolled back to snapshot");
      setVersionOpen(false);
      void qc.invalidateQueries({ queryKey: ["thingModel", productId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const importInput = useRef<HTMLInputElement>(null);
  function exportThingModel() {
    const blob = new Blob([JSON.stringify({ version, elements }, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `thing-model-${product.data?.key ?? productId}.json`;
    a.click();
    URL.revokeObjectURL(url);
  }
  function importThingModel(file: File) {
    void file.text().then((text) => {
      try {
        const parsed = JSON.parse(text);
        if (!Array.isArray(parsed.elements)) throw new Error("elements[] missing");
        setElements(parsed.elements as ThingModelElement[]);
        if (parsed.version) setVersion(String(parsed.version));
        setDirty(true);
        toast.success("Thing model imported from JSON — save to persist");
      } catch (e) {
        toast.error((e as Error).message);
      }
    });
  }

  const grouped = useMemo(() => {
    const order = ["property", "service", "event"];
    return order.map((t) => ({ type: t, items: elements.filter((e) => e.type === t) }));
  }, [elements]);

  function openAdd(type: string) {
    setEditingIndex(null);
    const base: ThingModelElement = { ...emptyElement, type: type as ThingModelElement["type"], specs: {} };
    if (type === "service") base.specs = { inputs: [] };
    else if (type === "event") base.specs = { fields: [] };
    setDraft(base);
    setOpen(true);
  }

  function openEdit(index: number) {
    setEditingIndex(index);
    const el = elements[index];
    const specs: Record<string, unknown> = { ...(el.specs ?? {}) };
    if (el.type === "service" && !specs.inputs) specs.inputs = [];
    if (el.type === "event" && !specs.fields) specs.fields = [];
    setDraft({ ...el, specs });
    setOpen(true);
  }

  function applyDraft() {
    if (!draft.identifier) {
      toast.error("Identifier is required");
      return;
    }
    setElements((prev) => {
      const next = [...prev];
      if (editingIndex === null) next.push(draft);
      else next[editingIndex] = draft;
      return next;
    });
    setDirty(true);
    setOpen(false);
  }

  function removeElement(index: number) {
    setElements((prev) => prev.filter((_, i) => i !== index));
    setDirty(true);
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <h1 className="min-w-0 truncate text-2xl font-semibold tracking-tight whitespace-nowrap">
              {product.data?.name ?? t("product.title")}
            </h1>
            {product.data && (
              <Badge variant={protocolVariant[product.data.protocol] ?? "secondary"} className="shrink-0">
                {product.data.protocol}
              </Badge>
            )}
            <code className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{product.data?.key}</code>
          </div>
          <p className="text-sm text-muted-foreground">{product.data?.description || t("projects.noDescription")}</p>
        </div>
        {canManage && (
        <div className="flex flex-wrap items-center gap-2">
          <Button onClick={() => save.mutate()} disabled={!dirty || save.isPending} className="w-full sm:w-auto">
            <Save className="h-4 w-4" /> {t("product.saveThingModel")}
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              setEditForm({ name: product.data?.name ?? "", description: product.data?.description ?? "" });
              setEditOpen(true);
            }}
          >
            <Pencil className="h-4 w-4" /> {t("common.edit")}
          </Button>
          <ConfirmButton
            title={t("product.deleteTitle")}
            description={t("product.deleteDesc")}
            confirmLabel={t("common.delete")}
            onConfirm={() => removeProduct.mutateAsync()}
          >
            <Button variant="outline" className="text-destructive hover:text-destructive">
              <Trash2 className="h-4 w-4" /> {t("common.delete")}
            </Button>
          </ConfirmButton>
        </div>
        )}

      </div>

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("product.editTitle")}</DialogTitle>
            <DialogDescription>{t("product.editDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("common.name")}</Label>
              <Input value={editForm.name} onChange={(e) => setEditForm({ ...editForm, name: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t("common.description")}</Label>
              <Textarea
                rows={3}
                value={editForm.description}
                onChange={(e) => setEditForm({ ...editForm, description: e.target.value })}
              />
            </div>
            <div className="rounded-md bg-muted/40 p-3 text-xs text-muted-foreground">
              {t("product.editHint")}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={() => saveInfo.mutate()} disabled={saveInfo.isPending || !editForm.name}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={versionOpen} onOpenChange={setVersionOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("product.versions")}</DialogTitle>
            <DialogDescription>{t("product.versionsDesc")}</DialogDescription>
          </DialogHeader>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("product.version")}</TableHead>
                <TableHead>{t("audit.time")}</TableHead>
                <TableHead className="w-24 text-right">{t("common.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(versions.data ?? []).length === 0 && (
                <TableRow>
                  <TableCell colSpan={3} className="py-8 text-center text-muted-foreground">
                    {t("product.noVersions")}
                  </TableCell>
                </TableRow>
              )}
              {(versions.data ?? []).map((v) => (
                <TableRow key={v.id}>
                  <TableCell className="font-mono">{v.version}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">{formatTime(v.publishedAt)}</TableCell>
                  <TableCell className="text-right">
                    {canManage && (
                      <ConfirmButton
                        title={t("product.rollbackTitle")}
                        description={t("product.rollbackDesc", { version: v.version })}
                        confirmLabel={t("common.delete")}
                        onConfirm={() => rollbackModel.mutateAsync(v.id)}
                      >
                        <Button size="sm" variant="outline">
                          {t("product.rollback")}
                        </Button>
                      </ConfirmButton>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DialogContent>
      </Dialog>

      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <div>
            <CardTitle className="flex items-center gap-2 text-base">
              {t("product.thingModel")}
              <Badge variant={thingModel.data?.status === "published" ? "success" : "secondary"}>
                {thingModel.data?.status ?? "draft"}
              </Badge>
            </CardTitle>
            <CardDescription>{t("product.thingModelDesc")}</CardDescription>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" variant="outline" onClick={exportThingModel}>
              <Download className="h-4 w-4" /> {t("product.export")}
            </Button>
            {canManage && (
              <>
                <Button size="sm" variant="outline" onClick={() => importInput.current?.click()}>
                  <Upload className="h-4 w-4" /> {t("product.import")}
                </Button>
                <input
                  ref={importInput}
                  type="file"
                  accept="application/json"
                  className="hidden"
                  onChange={(e) => {
                    const f = e.target.files?.[0];
                    if (f) importThingModel(f);
                    e.target.value = "";
                  }}
                />
                <Button size="sm" variant="outline" onClick={() => setVersionOpen(true)}>
                  <History className="h-4 w-4" /> {t("product.versions")}
                </Button>
                <Button size="sm" onClick={() => publishModel.mutate()} disabled={publishModel.isPending}>
                  <Rocket className="h-4 w-4" /> {t("product.publish")}
                </Button>
              </>
            )}
          </div>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
            <div className="space-y-2">
              <Label>{t("product.version")}</Label>
              <Input
                className="w-full sm:w-40"
                value={version}
                onChange={(e) => {
                  setVersion(e.target.value);
                  setDirty(true);
                }}
              />
            </div>
            {canManage && (
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="outline" onClick={() => openAdd("property")}>
                <Plus className="h-4 w-4" /> {t("product.property")}
              </Button>
              <Button size="sm" variant="outline" onClick={() => openAdd("service")}>
                <Plus className="h-4 w-4" /> {t("product.service")}
              </Button>
              <Button size="sm" variant="outline" onClick={() => openAdd("event")}>
                <Plus className="h-4 w-4" /> {t("product.event")}
              </Button>
            </div>
            )}
          </div>

          {grouped.map((group) => (
            <div key={group.type}>
              <h3 className="mb-2 text-sm font-semibold">{t(`product.group_${group.type}`)}</h3>
              <div className="rounded-lg border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="whitespace-nowrap">{t("common.identifier")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("common.name")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("common.type")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("product.access")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("product.unit")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("product.paramsColumn")}</TableHead>
                      <TableHead className="whitespace-nowrap">{t("product.range")}</TableHead>
                      <TableHead className="w-24 whitespace-nowrap text-right">{t("common.actions")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {group.items.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={8} className="py-6 text-center text-muted-foreground">
                          {t("product.emptyHint", { type: t(`product.group_${group.type}`) })}
                        </TableCell>
                      </TableRow>
                    )}
                    {group.items.map((el) => {
                      const index = elements.indexOf(el);
                      return (
                        <TableRow key={index}>
                          <TableCell className="font-mono text-xs">{el.identifier}</TableCell>
                          <TableCell className="max-w-[12rem] truncate whitespace-nowrap">{el.name || "-"}</TableCell>
                          <TableCell className="whitespace-nowrap">{el.dataType}</TableCell>
                          <TableCell className="whitespace-nowrap">{el.type === "property" ? el.accessMode : "-"}</TableCell>
                          <TableCell className="whitespace-nowrap">{el.unit || "-"}</TableCell>
                          <TableCell className="whitespace-nowrap">
                            {el.type === "service"
                              ? String(specParams(el, "inputs").length)
                              : el.type === "event"
                                ? String(specParams(el, "fields").length)
                                : "-"}
                          </TableCell>
                          <TableCell className="whitespace-nowrap">
                            {el.min != null || el.max != null ? `${el.min ?? "-"} ~ ${el.max ?? "-"}` : "-"}
                          </TableCell>
                          <TableCell className="text-right">
                            {canManage && (
                            <div className="flex items-center justify-end gap-1 whitespace-nowrap">
                              <Button variant="ghost" size="icon" onClick={() => openEdit(index)}>
                                <Pencil className="h-4 w-4" />
                              </Button>
                              <Button variant="ghost" size="icon" onClick={() => removeElement(index)}>
                                <Trash2 className="h-4 w-4 text-destructive" />
                              </Button>
                            </div>
                            )}
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">{t("product.firmware")}</CardTitle>
            <CardDescription>{t("product.firmwareDesc")}</CardDescription>
          </div>
          <Dialog open={fwOpen} onOpenChange={setFwOpen}>
            {canManage && (
            <DialogTrigger asChild>
              <Button size="sm">
                <Upload className="h-4 w-4" /> {t("product.upload")}
              </Button>
            </DialogTrigger>
            )}
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{t("product.uploadTitle")}</DialogTitle>
                <DialogDescription>{t("product.uploadDesc")}</DialogDescription>
              </DialogHeader>
              <div className="space-y-4">
                <div className="space-y-2">
                  <Label>{t("product.version")}</Label>
                  <Input value={fwForm.version} onChange={(e) => setFwForm({ ...fwForm, version: e.target.value })} placeholder="1.0.1" />
                </div>
                <div className="space-y-2">
                  <Label>{t("common.name")}</Label>
                  <Input value={fwForm.name} onChange={(e) => setFwForm({ ...fwForm, name: e.target.value })} />
                </div>
                <div className="space-y-2">
                  <Label>{t("product.binary")}</Label>
                  <input ref={fileInput} type="file" className="block w-full text-sm" />
                </div>
              </div>
              <DialogFooter>
                <Button onClick={() => upload.mutate()} disabled={upload.isPending}>
                  {t("product.upload")}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("product.version")}</TableHead>
                <TableHead>{t("product.file")}</TableHead>
                <TableHead>{t("product.size")}</TableHead>
                <TableHead>{t("product.checksum")}</TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(firmwares.data ?? []).length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground">
                    {t("product.noFirmware")}
                  </TableCell>
                </TableRow>
              )}
              {(firmwares.data ?? []).map((f) => (
                <TableRow key={f.id}>
                  <TableCell className="font-medium">{f.version}</TableCell>
                  <TableCell className="font-mono text-xs">{f.fileName}</TableCell>
                  <TableCell>{f.fileSize} B</TableCell>
                  <TableCell className="font-mono text-xs">{f.checksum.slice(0, 16)}...</TableCell>
                  <TableCell className="text-right">
                    {canManage && (
                    <Button variant="ghost" size="icon" onClick={() => deleteFw.mutate(f.id)}>
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editingIndex === null ? t("product.addElement") : t("product.edit")} {draft.type}
            </DialogTitle>
          </DialogHeader>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label>{t("common.type")}</Label>
              <Select
                value={draft.type}
                onValueChange={(v) => {
                  const t = v as ThingModelElement["type"];
                  const specs: Record<string, unknown> = { ...(draft.specs ?? {}) };
                  if (t === "service" && !specs.inputs) specs.inputs = [];
                  if (t === "event" && !specs.fields) specs.fields = [];
                  setDraft({ ...draft, type: t, specs });
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="property">property</SelectItem>
                  <SelectItem value="service">service</SelectItem>
                  <SelectItem value="event">event</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t("common.identifier")}</Label>
              <Input value={draft.identifier} onChange={(e) => setDraft({ ...draft, identifier: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t("common.name")}</Label>
              <Input value={draft.name ?? ""} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t("product.dataType")}</Label>
              <Select value={draft.dataType} onValueChange={(v) => setDraft({ ...draft, dataType: v })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {dataTypes.map((t) => (
                    <SelectItem key={t} value={t}>
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {draft.type === "event" && (
              <div className="space-y-2">
                <Label>{t("product.eventType")}</Label>
                <Select value={draft.eventType ?? "info"} onValueChange={(v) => setDraft({ ...draft, eventType: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="info">info</SelectItem>
                    <SelectItem value="alarm">alarm</SelectItem>
                    <SelectItem value="fault">fault</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}
            {draft.dataType === "enum" && (
              <div className="space-y-2 sm:col-span-2">
                <Label>{t("product.enumValues")}</Label>
                <EnumEditor
                  values={Array.isArray(draft.specs?.enumValues) ? (draft.specs!.enumValues as unknown as string[]) : []}
                  onChange={(vals) =>
                    setDraft({ ...draft, specs: { ...(draft.specs ?? {}), enumValues: vals } })
                  }
                />
              </div>
            )}
            {draft.type === "property" && (
              <div className="space-y-2">
                <Label>{t("product.accessMode")}</Label>
                <Select value={draft.accessMode ?? "rw"} onValueChange={(v) => setDraft({ ...draft, accessMode: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="r">read</SelectItem>
                    <SelectItem value="w">write</SelectItem>
                    <SelectItem value="rw">read/write</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="space-y-2">
              <Label>{t("product.unit")}</Label>
              <Input value={draft.unit ?? ""} onChange={(e) => setDraft({ ...draft, unit: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t("product.min")}</Label>
              <Input
                type="number"
                value={draft.min ?? ""}
                onChange={(e) => setDraft({ ...draft, min: e.target.value === "" ? null : Number(e.target.value) })}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("product.max")}</Label>
              <Input
                type="number"
                value={draft.max ?? ""}
                onChange={(e) => setDraft({ ...draft, max: e.target.value === "" ? null : Number(e.target.value) })}
              />
            </div>
          </div>
          {draft.type === "service" && (
            <div className="mt-4 space-y-2">
              <Label>{t("product.serviceInputs")}</Label>
              <ParamEditor items={specParams(draft, "inputs")} onItems={(items) => setSpecParams(draft, "inputs", items, setDraft)} />
            </div>
          )}
          {draft.type === "service" && (
            <div className="mt-4 space-y-2">
              <Label>{t("product.serviceOutputs")}</Label>
              <ParamEditor items={specParams(draft, "outputs")} onItems={(items) => setSpecParams(draft, "outputs", items, setDraft)} />
            </div>
          )}
          {draft.type === "event" && (
            <div className="mt-4 space-y-2">
              <Label>{t("product.eventFields")}</Label>
              <ParamEditor items={specParams(draft, "fields")} onItems={(items) => setSpecParams(draft, "fields", items, setDraft)} />
            </div>
          )}
          <div className="space-y-2">
            <Label>{t("common.description")}</Label>
            <Textarea value={draft.description ?? ""} onChange={(e) => setDraft({ ...draft, description: e.target.value })} />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={applyDraft}>{t("common.apply")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
