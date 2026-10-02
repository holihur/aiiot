import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Database, HardDrive, Layers, RefreshCw, Save, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { adminApi } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

function formatBytes(n: number) {
  if (!n) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(n) / Math.log(1024));
  return `${(n / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export default function StoragePage() {
  const qc = useQueryClient();
  const { t } = useI18n();
  const { data, isLoading } = useQuery({ queryKey: ["admin", "storage"], queryFn: adminApi.getStorage });
  const [days, setDays] = useState<string>("");

  const retention = data?.retentionDays ?? 30;

  const saveRetention = useMutation({
    mutationFn: () => adminApi.updateRetention(Number(days || retention)),
    onSuccess: () => {
      toast.success("Retention policy updated");
      void qc.invalidateQueries({ queryKey: ["admin", "storage"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const drop = useMutation({
    mutationFn: (name: string) => adminApi.dropPartition(name),
    onSuccess: () => {
      toast.success("Partition dropped");
      void qc.invalidateQueries({ queryKey: ["admin", "storage"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const refreshRollup = useMutation({
    mutationFn: () => adminApi.refreshRollup(),
    onSuccess: () => {
      toast.success("Hourly rollup refreshed");
      void qc.invalidateQueries({ queryKey: ["admin", "storage"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight whitespace-nowrap">{t("storage.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("storage.subtitle")}</p>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm text-muted-foreground">
              <Database className="h-4 w-4" /> {t("storage.partitions")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold">{data?.partitions.length ?? 0}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm text-muted-foreground">
              <HardDrive className="h-4 w-4" /> {t("storage.approxRows")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold">{(data?.totalRows ?? 0).toLocaleString()}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm text-muted-foreground">{t("storage.diskSize")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold">{formatBytes(data?.totalBytes ?? 0)}</div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2 text-base">
              <Layers className="h-4 w-4" /> {t("storage.rollup")}
            </CardTitle>
            <CardDescription>{t("storage.rollupDesc")}</CardDescription>
          </div>
          <Button variant="outline" onClick={() => refreshRollup.mutate()} disabled={refreshRollup.isPending}>
            <RefreshCw className="h-4 w-4" /> {t("storage.refreshNow")}
          </Button>
        </CardHeader>
        <CardContent className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <div className="text-xs text-muted-foreground">{t("storage.rows")}</div>
            <div className="text-xl font-semibold">{(data?.rollup?.rows ?? 0).toLocaleString()}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("storage.size")}</div>
            <div className="text-xl font-semibold">{formatBytes(data?.rollup?.sizeBytes ?? 0)}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("storage.lastRefresh")}</div>
            <div className="text-xl font-semibold">
              {data?.rollup?.lastRefresh
                ? new Date(data.rollup.lastRefresh).toLocaleTimeString()
                : t("storage.never")}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("storage.retention")}</CardTitle>
          <CardDescription>{t("storage.retentionDesc", { days: data?.defaultDays ?? 30 })}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap items-end gap-3">
          <div className="space-y-2">
            <Label>{t("storage.retentionDays")}</Label>
            <Input
              className="w-40"
              type="number"
              value={days || retention}
              onChange={(e) => setDays(e.target.value)}
            />
          </div>
          <Button onClick={() => saveRetention.mutate()} disabled={saveRetention.isPending}>
            <Save className="h-4 w-4" /> {t("common.save")}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("storage.partitions")}</CardTitle>
          <CardDescription>Monthly partitions of the telemetry_data table.</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("storage.name")}</TableHead>
                <TableHead>{t("storage.range")}</TableHead>
                <TableHead>{t("storage.rows")}</TableHead>
                <TableHead>{t("storage.size")}</TableHead>
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
              {(data?.partitions ?? []).map((p) => (
                <TableRow key={p.name}>
                  <TableCell className="font-mono text-xs">{p.name}</TableCell>
                  <TableCell className="text-xs">
                    {p.from ? new Date(p.from).toLocaleDateString() : "-"} →{" "}
                    {p.to ? new Date(p.to).toLocaleDateString() : "-"}
                  </TableCell>
                  <TableCell>{p.rows.toLocaleString()}</TableCell>
                  <TableCell>{formatBytes(p.sizeBytes)}</TableCell>
                  <TableCell className="text-right">
                    <Button variant="ghost" size="icon" onClick={() => drop.mutate(p.name)} title="Drop partition" aria-label="Drop partition">
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
