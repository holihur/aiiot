import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Boxes, Plus, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { useAuth } from "@/lib/auth";
import { ConfirmButton } from "@/components/ConfirmButton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

const protocolVariant: Record<string, "default" | "secondary" | "success" | "warning"> = {
  mqtt: "success",
  coap: "warning",
  custom: "secondary",
};

export default function ProductsPage() {
  const qc = useQueryClient();
  const remove = useMutation({
    mutationFn: (id: number) => api.deleteProduct(id),
    onSuccess: () => {
      toast.success("Product deleted");
      void qc.invalidateQueries({ queryKey: ["products"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const { t } = useI18n();
  const { user } = useAuth();
  const canManage = (p: { createdBy?: number }) => !!user && (user.systemRole === "admin" || p.createdBy === user.id);
  const [keyword, setKeyword] = useState("");
  const [view, setView] = useState<"grid" | "table">("grid");
  const { data: products, isLoading } = useQuery({
    queryKey: ["products", keyword],
    queryFn: () => api.listProducts({ keyword: keyword || undefined }),
  });

  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({
    name: "",
    protocol: "mqtt" as "mqtt" | "coap" | "custom" | "modbus" | "opcua",
    category: "",
    description: "",
  });

  const create = useMutation({
    mutationFn: () => api.createProduct(form),
    onSuccess: () => {
      toast.success("Product created");
      setOpen(false);
      setForm({ name: "", protocol: "mqtt", category: "", description: "" });
      void qc.invalidateQueries({ queryKey: ["products"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight whitespace-nowrap">{t("products.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("products.subtitle")}</p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="w-full sm:w-auto">
              <Plus className="h-4 w-4" /> {t("products.new")}
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("products.createTitle")}</DialogTitle>
              <DialogDescription>{t("products.createDesc")}</DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>{t("common.name")}</Label>
                <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
                <p className="text-xs text-muted-foreground">{t("products.keyHint")}</p>
              </div>
              <div className="space-y-2">
                <Label>{t("common.protocol")}</Label>
                <Select
                  value={form.protocol}
                  onValueChange={(v) => setForm({ ...form, protocol: v as "mqtt" | "coap" | "custom" | "modbus" | "opcua" })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="mqtt">MQTT</SelectItem>
                    <SelectItem value="coap">CoAP</SelectItem>
                    <SelectItem value="custom">Custom</SelectItem>
                    <SelectItem value="modbus">Modbus</SelectItem>
                    <SelectItem value="opcua">OPC-UA</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t("common.name")} (category)</Label>
                <Input value={form.category} onChange={(e) => setForm({ ...form, category: e.target.value })} />
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

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="relative w-full sm:max-w-xs">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            className="pl-8"
            placeholder={t("products.searchPlaceholder")}
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
          />
        </div>
        <div className="flex gap-2 sm:ml-auto">
          <Button variant={view === "grid" ? "default" : "outline"} size="sm" onClick={() => setView("grid")}>
            Grid
          </Button>
          <Button variant={view === "table" ? "default" : "outline"} size="sm" onClick={() => setView("table")}>
            Table
          </Button>
        </div>
      </div>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : (products ?? []).length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
            <Boxes className="h-8 w-8 text-muted-foreground" />
            <p className="font-medium">{t("products.empty")}</p>
          </CardContent>
        </Card>
      ) : view === "table" ? (
        <Card>
          <CardContent className="pt-5">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("common.name")}</TableHead>
                  <TableHead>{t("common.key")}</TableHead>
                  <TableHead>{t("common.protocol")}</TableHead>
                  <TableHead>{t("common.status")}</TableHead>
                  <TableHead className="w-16 text-right">{t("common.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(products ?? []).map((p) => (
                  <TableRow key={p.id}>
                    <TableCell>
                      <Link to={`/products/${p.id}`} className="font-medium text-primary hover:underline">
                        {p.name}
                      </Link>
                    </TableCell>
                    <TableCell className="font-mono text-xs">{p.key}</TableCell>
                    <TableCell>
                      <Badge variant={protocolVariant[p.protocol] ?? "secondary"}>{p.protocol}</Badge>
                    </TableCell>
                    <TableCell>{p.status}</TableCell>
                    <TableCell className="text-right">
                      {canManage(p) && (
                      <ConfirmButton
                        title={t("product.deleteTitle")}
                        description={t("product.deleteDesc")}
                        confirmLabel={t("common.delete")}
                        onConfirm={() => remove.mutateAsync(p.id)}
                      >
                        <Button variant="ghost" size="icon" aria-label="Delete product">
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
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {(products ?? []).map((p) => (
            <Card key={p.id} className="h-full transition-colors hover:border-primary/50">
              <CardHeader>
                <CardTitle className="flex items-center justify-between gap-2 text-base">
                  <Link to={`/products/${p.id}`} className="min-w-0 truncate hover:underline">
                    {p.name}
                  </Link>
                  <span className="flex shrink-0 items-center gap-1">
                    <Badge variant={protocolVariant[p.protocol] ?? "secondary"}>{p.protocol}</Badge>
                    {canManage(p) && (
                    <ConfirmButton
                      title={t("product.deleteTitle")}
                      description={t("product.deleteDesc")}
                      confirmLabel={t("common.delete")}
                      onConfirm={() => remove.mutateAsync(p.id)}
                    >
                      <Button variant="ghost" size="icon" aria-label="Delete product">
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </ConfirmButton>
                    )}
                  </span>
                </CardTitle>
                <CardDescription className="line-clamp-2">{p.description || t("projects.noDescription")}</CardDescription>
              </CardHeader>
              <CardContent>
                <code className="text-xs text-muted-foreground">{p.key}</code>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
