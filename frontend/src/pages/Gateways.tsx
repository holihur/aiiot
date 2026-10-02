import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Network, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { adminApi } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatTime } from "@/lib/utils";

const protocolVariant: Record<string, "success" | "warning" | "secondary"> = {
  mqtt: "success",
  coap: "warning",
  custom: "secondary",
};

export default function GatewaysPage() {
  const qc = useQueryClient();
  const { t } = useI18n();
  const gateways = useQuery({ queryKey: ["admin", "gateways"], queryFn: adminApi.listGateways, refetchInterval: 5000 });

  const deregister = useMutation({
    mutationFn: (instanceId: string) => adminApi.deregisterGateway(instanceId),
    onSuccess: () => {
      toast.success("Gateway removed from registry");
      void qc.invalidateQueries({ queryKey: ["admin", "gateways"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("gateways.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("gateways.subtitle")}</p>
        </div>
        <Button variant="outline" onClick={() => gateways.refetch()}>
          <RefreshCw className="h-4 w-4" /> {t("common.refresh")}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Network className="h-4 w-4" /> {t("gateways.instances")}
          </CardTitle>
          <CardDescription>{t("gateways.instancesDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("gateways.instance")}</TableHead>
                <TableHead>{t("common.protocol")}</TableHead>
                <TableHead>{t("gateways.version")}</TableHead>
                <TableHead>{t("gateways.activeDevices")}</TableHead>
                <TableHead>{t("gateways.downlinkUrl")}</TableHead>
                <TableHead>{t("gateways.lastHeartbeat")}</TableHead>
                <TableHead>{t("gateways.state")}</TableHead>
                <TableHead className="w-16"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(gateways.data ?? []).length === 0 && (
                <TableRow>
                  <TableCell colSpan={8} className="py-10 text-center text-muted-foreground">
                    {t("gateways.empty")}
                    <div className="mt-2 text-xs">
                      Start one, e.g.{" "}
                      <code className="rounded bg-muted px-1">GATEWAY_TOKEN=... ./mqtt-gateway</code>
                    </div>
                  </TableCell>
                </TableRow>
              )}
              {(gateways.data ?? []).map((g) => (
                <TableRow key={g.instanceId}>
                  <TableCell className="font-medium">{g.instanceId}</TableCell>
                  <TableCell>
                    <Badge variant={protocolVariant[g.protocol] ?? "secondary"}>{g.protocol}</Badge>
                  </TableCell>
                  <TableCell>{g.version}</TableCell>
                  <TableCell>{g.activeDevices}</TableCell>
                  <TableCell className="font-mono text-xs">{g.downlinkUrl}</TableCell>
                  <TableCell className="text-xs">{formatTime(g.lastHeartbeat)}</TableCell>
                  <TableCell>
                    <Badge variant={g.healthy ? "success" : "destructive"}>{g.healthy ? t("gateways.healthy") : t("gateways.stale")}</Badge>
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      title={t("gateways.remove")}
                      aria-label={t("gateways.remove")}
                      onClick={() => deregister.mutate(g.instanceId)}
                    >
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
