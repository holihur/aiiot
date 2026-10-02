import { useQuery } from "@tanstack/react-query";
import { Activity, Boxes, RefreshCw, Send } from "lucide-react";
import { adminApi } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatBytes, formatTime } from "@/lib/utils";

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="text-xl font-semibold tabular-nums">{value}</span>
    </div>
  );
}

export default function NatsPage() {
  const { t } = useI18n();
  const stats = useQuery({
    queryKey: ["admin", "nats-stats"],
    queryFn: adminApi.natsStats,
    refetchInterval: 5000,
  });

  const js = stats.data?.jetstream;
  const counters = stats.data?.counters;
  const errorRate =
    counters && counters.natsConsumedTotal + counters.natsConsumeErrors > 0
      ? ((counters.natsConsumeErrors / (counters.natsConsumedTotal + counters.natsConsumeErrors)) * 100).toFixed(2)
      : "0";

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("nats.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("nats.subtitle")}</p>
        </div>
        <Button variant="outline" onClick={() => stats.refetch()}>
          <RefreshCw className="h-4 w-4" /> {t("common.refresh")}
        </Button>
      </div>

      {stats.isError && (
        <Card className="border-destructive/50">
          <CardContent className="pt-6 text-sm text-destructive">{(stats.error as Error).message}</CardContent>
        </Card>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-base">
              <Boxes className="h-4 w-4" /> {t("nats.stream")}
            </CardTitle>
            <CardDescription>{js?.stream ?? "AIOT_UPLINK"}</CardDescription>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-4">
            <Stat label={t("nats.messages")} value={js?.messages ?? 0} />
            <Stat label={t("nats.bytes")} value={formatBytes(js?.bytes ?? 0)} />
            <Stat label={t("nats.firstSeq")} value={js?.firstSeq ?? 0} />
            <Stat label={t("nats.lastSeq")} value={js?.lastSeq ?? 0} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-base">
              <Send className="h-4 w-4" /> {t("nats.consumer")}
            </CardTitle>
            <CardDescription className="flex items-center gap-2">
              {js?.consumer.name}
              {js?.consumer.pushBound ? <Badge variant="success">{t("nats.pushBound")}</Badge> : <Badge variant="warning">{t("nats.notBound")}</Badge>}
            </CardDescription>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-4">
            <Stat label={t("nats.delivered")} value={js?.consumer.deliveredStreamSeq ?? 0} />
            <Stat label={t("nats.ackFloor")} value={js?.consumer.ackFloorStreamSeq ?? 0} />
            <Stat label={t("nats.pending")} value={js?.consumer.numPending ?? 0} />
            <Stat label={t("nats.redelivered")} value={js?.consumer.numRedelivered ?? 0} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-base">
              <Activity className="h-4 w-4" /> {t("nats.throughput")}
            </CardTitle>
            <CardDescription>{t("nats.cumulative")}</CardDescription>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-4">
            <Stat label={t("nats.consumed")} value={counters?.natsConsumedTotal ?? 0} />
            <Stat label={t("nats.ingested")} value={counters?.ingestTotal ?? 0} />
            <Stat label={`${t("nats.consumeErrors")} (${errorRate}%)`} value={counters?.natsConsumeErrors ?? 0} />
            <Stat label={t("nats.ingestErrors")} value={counters?.ingestErrors ?? 0} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-base">
              <Activity className="h-4 w-4" /> {t("nats.platform")}
            </CardTitle>
            <CardDescription>{t("nats.platformDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-4">
            <Stat label={t("nats.devicesOnline")} value={counters?.devicesOnline ?? 0} />
            <Stat label={t("nats.gatewaysHealthy")} value={counters?.gatewaysHealthy ?? 0} />
            <Stat label={t("nats.ruleEvaluations")} value={counters?.ruleEvaluations ?? 0} />
            <Stat label={t("nats.ruleTriggers")} value={counters?.ruleTriggers ?? 0} />
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("nats.detail")}</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("nats.field")}</TableHead>
                <TableHead>{t("nats.value")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell>{t("nats.stream")}</TableCell>
                <TableCell>{js?.stream ?? "—"}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>{t("nats.consumer")}</TableCell>
                <TableCell>{js?.consumer.name ?? "—"}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>{t("nats.numAckPending")}</TableCell>
                <TableCell>{js?.consumer.numAckPending ?? "—"}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>{t("nats.numWaiting")}</TableCell>
                <TableCell>{js?.consumer.numWaiting ?? "—"}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>{t("nats.consumerCreated")}</TableCell>
                <TableCell>{formatTime(js?.consumer.created)}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}