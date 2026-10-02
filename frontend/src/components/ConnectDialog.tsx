import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Copy, Download, Plug } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

function CopyRow({ label, value, mono = true }: { label: string; value: string; mono?: boolean }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="flex items-center justify-between gap-2 rounded-md border p-2">
      <div className="min-w-0">
        <div className="text-xs text-muted-foreground">{label}</div>
        <div className={mono ? "truncate font-mono text-sm" : "truncate text-sm"}>{value || "-"}</div>
      </div>
      <Button
        variant="ghost"
        size="icon"
        title="Copy"
        onClick={() => {
          void navigator.clipboard.writeText(value);
          setCopied(true);
          toast.success(`${label} copied`);
          setTimeout(() => setCopied(false), 1500);
        }}
      >
        {copied ? <Check className="h-4 w-4 text-emerald-500" /> : <Copy className="h-4 w-4" />}
      </Button>
    </div>
  );
}

export function ConnectDialog({
  deviceId,
  deviceName,
  open,
  onOpenChange,
}: {
  deviceId: number;
  deviceName?: string;
  open: boolean;
  onOpenChange: (o: boolean) => void;
}) {
  const conn = useQuery({
    queryKey: ["deviceConnection", deviceId],
    queryFn: () => api.getDeviceConnection(deviceId),
    enabled: open,
  });
  const { t } = useI18n();
  const c = conn.data;

  function downloadCredentials() {
    if (!c) return;
    const content = JSON.stringify(c, null, 2);
    const blob = new Blob([content], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${c.deviceKey}-credentials.json`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Plug className="h-4 w-4" /> {t("connect.title", { name: deviceName ?? "device" })}
          </DialogTitle>
          <DialogDescription>{t("connect.desc")}</DialogDescription>
        </DialogHeader>

        {!c ? (
          <p className="py-6 text-center text-sm text-muted-foreground">{t("connect.loading")}</p>
        ) : (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="outline">{c.protocol}</Badge>
              <Badge variant="secondary">host {c.host}</Badge>
              <Button size="sm" variant="outline" className="ml-auto" onClick={downloadCredentials}>
                <Download className="h-4 w-4" /> {t("connect.download")}
              </Button>
            </div>

            {c.protocol === "mqtt" && c.mqtt && (
              <Tabs defaultValue="mqtt">
                <TabsList>
                  <TabsTrigger value="mqtt">MQTT</TabsTrigger>
                </TabsList>
                <TabsContent value="mqtt" className="space-y-2">
                  <CopyRow label={t("connect.broker")} value={`${c.host}:${c.mqtt.tcpPort}`} />
                  <CopyRow label={t("connect.username")} value={c.username} />
                  <CopyRow label={t("connect.password")} value={c.password} />
                  <CopyRow label={t("connect.uplink")} value={c.mqtt.uplinkTopic} />
                  <CopyRow label={t("connect.downlink")} value={c.mqtt.downlinkTopic} />
                  <CopyRow label={t("connect.event")} value={c.mqtt.eventTopic} />
                  <CopyRow label={t("connect.peer")} value={c.mqtt.peerTopic} />
                  <CopyRow label={t("connect.sample")} value={c.mqtt.samplePayload} />
                </TabsContent>
              </Tabs>
            )}

            {c.protocol === "coap" && c.coap && (
              <Tabs defaultValue="coap">
                <TabsList>
                  <TabsTrigger value="coap">CoAP</TabsTrigger>
                </TabsList>
                <TabsContent value="coap" className="space-y-2">
                  <CopyRow label={t("connect.server")} value={`coap://${c.host}:${c.coap.udpPort}`} />
                  <CopyRow
                    label={t("connect.propertyResource")}
                    value={`${c.coap.propertyPath}?${c.coap.secretQueryKey}=${c.password}`}
                  />
                  <CopyRow
                    label={t("connect.eventResource")}
                    value={`${c.coap.eventPath}?${c.coap.secretQueryKey}=${c.password}`}
                  />
                  <CopyRow label={t("connect.sample")} value={c.coap.samplePayload} />
                </TabsContent>
              </Tabs>
            )}

            {c.protocol === "custom" && c.custom && (
              <Tabs defaultValue="tcp">
                <TabsList>
                  <TabsTrigger value="tcp">Custom TCP</TabsTrigger>
                </TabsList>
                <TabsContent value="tcp" className="space-y-2">
                  <CopyRow label={t("connect.server")} value={`${c.host}:${c.custom.tcpPort}`} />
                  <CopyRow label={t("connect.authFrame")} value={JSON.stringify(c.custom.authFrame)} />
                  <CopyRow label={t("connect.propertyFrame")} value={JSON.stringify(c.custom.propertyFrame)} />
                </TabsContent>
              </Tabs>
            )}

            <div className="rounded-md bg-muted/40 p-3 text-xs text-muted-foreground">
              {t("connect.workspaceHint", { ws: c.workspace })}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
