import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Copy, Download, Plug } from "lucide-react";
import { toast } from "sonner";
import { api, type DeviceConnection } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";

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

// --- quick-start code templates -------------------------------------------

type Translator = (key: string, vars?: Record<string, string | number>) => string;

function httpCurl(c: DeviceConnection, t: Translator): string {
  return `# ${t("connect.code.lifecycle")}
curl -X POST http://${c.host}/api/v1/ingest/${c.productKey}/${c.deviceKey} \\
  -H "X-Device-Secret: ${c.password}" -H 'Content-Type: application/json' \\
  -d '{"kind":"lifecycle","state":"online"}'

# ${t("connect.code.reportProps")}
curl -X POST http://${c.host}/api/v1/ingest/${c.productKey}/${c.deviceKey} \\
  -H "X-Device-Secret: ${c.password}" -H 'Content-Type: application/json' \\
  -d '{"kind":"property","identifier":"temperature","params":{"value":25.4}}'`;
}

function httpPython(c: DeviceConnection): string {
  return `import json, time, urllib.request

BASE = "http://${c.host}"
URL = f"{BASE}/api/v1/ingest/${c.productKey}/${c.deviceKey}"
HEADERS = {"X-Device-Secret": "${c.password}", "Content-Type": "application/json"}

def post(obj):
    req = urllib.request.Request(URL, data=json.dumps(obj).encode(), headers=HEADERS)
    with urllib.request.urlopen(req) as r:
        print("->", r.status, obj)

post({"kind": "lifecycle", "state": "online"})
for i in range(60):
    post({"kind": "property", "identifier": "temperature",
          "params": {"value": round(20 + (i % 30) / 3, 2)}})
    time.sleep(10)`;
}

function mqttPython(c: DeviceConnection, t: Translator): string {
  const mqtt = c.mqtt!;
  return `import json, time
import paho.mqtt.client as mqtt

HOST, PORT = "${c.host}", ${mqtt.tcpPort}
USER, PASS = "${c.username}", "${c.password}"   # ${t("connect.code.credentialsComment")}

def on_connect(client, userdata, flags, rc, props=None):
    print("connected:", rc)
    client.subscribe("${mqtt.downlinkTopic}")

client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2)
client.username_pw_set(USER, PASS)
client.on_connect = on_connect
client.connect(HOST, PORT, 60)
client.loop_start()

for i in range(60):
    payload = {"temperature": round(20 + (i % 10) * 1.2, 1)}
    client.publish("${mqtt.uplinkTopic}", json.dumps(payload))
    print("sent:", payload)
    time.sleep(10)`;
}

function customPython(c: DeviceConnection): string {
  const tcp = c.custom!;
  return `import json, socket, time

HOST, PORT = "${c.host}", ${tcp.tcpPort}
PRODUCT, KEY, SECRET = "${c.productKey}", "${c.deviceKey}", "${c.password}"

s = socket.create_connection((HOST, PORT))

def send(obj):
    s.sendall((json.dumps(obj) + "\\n").encode())

send({"type": "auth", "productKey": PRODUCT, "deviceKey": KEY, "secret": SECRET})
print("auth sent")
time.sleep(0.5)

for i in range(60):
    send({"type": "property", "identifier": "temperature",
          "value": round(20 + (i % 10) / 3, 2), "id": f"m{i}"})
    print("sent", i)
    time.sleep(10)`;
}

function codeFor(c: DeviceConnection, lang: string, t: Translator): string {
  switch (lang) {
    case "curl":
      return httpCurl(c, t);
    case "mqtt":
      return mqttPython(c, t);
    case "custom":
      return customPython(c);
    default:
      return httpPython(c);
  }
}

// Available quick-start templates derived from the device protocol.
function langsFor(c: DeviceConnection): { value: string; label: string }[] {
  const base = [
    { value: "python", label: "Python · HTTP" },
    { value: "curl", label: "cURL · HTTP" },
  ];
  if (c.protocol === "mqtt" && c.mqtt) base.push({ value: "mqtt", label: "Python · MQTT" });
  if (c.protocol === "custom" && c.custom) base.push({ value: "custom", label: "Python · Custom TCP" });
  return base;
}

// --- device simulator -----------------------------------------------------

function Simulator({ c }: { c: DeviceConnection }) {
  const { t } = useI18n();
  const [running, setRunning] = useState(false);
  const [identifier, setIdentifier] = useState("temperature");
  const [intervalSec, setIntervalSec] = useState(5);
  const [log, setLog] = useState<{ t: string; ok: boolean; msg: string }[]>([]);
  const [sent, setSent] = useState(0);
  const [errors, setErrors] = useState(0);
  const timer = useRef<number | null>(null);

  const push = (ok: boolean, msg: string) => {
    setLog((l) => [{ t: new Date().toLocaleTimeString(), ok, msg }, ...l].slice(0, 8));
    if (ok) setSent((s) => s + 1);
    else setErrors((e) => e + 1);
  };

  const send = async (body: Record<string, unknown>) => {
    try {
      const r = await fetch(`/api/v1/ingest/${c.productKey}/${c.deviceKey}`, {
        method: "POST",
        headers: { "X-Device-Secret": c.password, "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      push(true, JSON.stringify(body));
    } catch (e) {
      push(false, (e as Error).message);
    }
  };

  const start = () => {
    void send({ kind: "lifecycle", state: "online" });
    timer.current = window.setInterval(() => {
      void send({
        kind: "property",
        identifier,
        params: { value: Math.round((20 + Math.random() * 15) * 10) / 10 },
      });
    }, Math.max(2, intervalSec) * 1000);
    setRunning(true);
  };

  const stop = () => {
    if (timer.current) window.clearInterval(timer.current);
    timer.current = null;
    setRunning(false);
  };

  useEffect(() => () => stop(), []);

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t("connect.simulatorDesc")}</p>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label>{t("connect.simulatorIdentifier")}</Label>
          <Input value={identifier} onChange={(e) => setIdentifier(e.target.value)} disabled={running} />
        </div>
        <div className="space-y-1.5">
          <Label>{t("connect.simulatorInterval")} (s)</Label>
          <Input
            type="number"
            min={2}
            value={intervalSec}
            onChange={(e) => setIntervalSec(Number(e.target.value))}
            disabled={running}
          />
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {!running ? (
          <Button onClick={start}>
            <Plug className="h-4 w-4" /> {t("connect.simulatorStart")}
          </Button>
        ) : (
          <Button variant="destructive" onClick={stop}>
            <span className="h-2 w-2 rounded-full bg-white" /> {t("connect.simulatorStop")}
          </Button>
        )}
        <Badge variant="secondary">{t("connect.simulatorSent")}: {sent}</Badge>
        {errors > 0 && <Badge variant="destructive">{t("connect.simulatorErrors")}: {errors}</Badge>}
      </div>
      <div className="rounded-md bg-muted/40 p-2">
        <div className="mb-1 text-xs text-muted-foreground">{t("connect.simulatorLog")}</div>
        {log.length === 0 && <div className="py-2 text-center text-xs text-muted-foreground">{t("connect.simulatorNoLog")}</div>}
        <div className="max-h-40 space-y-1 overflow-auto font-mono text-[11px]">
          {log.map((l, i) => (
            <div key={i} className={cn("flex gap-2", l.ok ? "text-foreground" : "text-destructive")}>
              <span className="shrink-0 text-muted-foreground">{l.t}</span>
              <span className="min-w-0 truncate">{l.msg}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// --- dialog ---------------------------------------------------------------

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
  const [lang, setLang] = useState("python");

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
          <Tabs defaultValue="credentials">
            <TabsList>
              <TabsTrigger value="credentials">{t("connect.credentials")}</TabsTrigger>
              <TabsTrigger value="quickstart">{t("connect.quickStart")}</TabsTrigger>
              <TabsTrigger value="simulator">{t("connect.simulator")}</TabsTrigger>
            </TabsList>

            <TabsContent value="credentials" className="space-y-4">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">{c.protocol}</Badge>
                <Badge variant="secondary">host {c.host}</Badge>
                <Button size="sm" variant="outline" className="ml-auto" onClick={downloadCredentials}>
                  <Download className="h-4 w-4" /> {t("connect.download")}
                </Button>
              </div>

              <CopyRow label={t("connect.username")} value={c.username} />
              <CopyRow label={t("connect.password")} value={c.password} />

              {c.protocol === "mqtt" && c.mqtt && (
                <>
                  <CopyRow label={t("connect.broker")} value={`${c.host}:${c.mqtt.tcpPort}`} />
                  <CopyRow label={t("connect.uplink")} value={c.mqtt.uplinkTopic} />
                  <CopyRow label={t("connect.downlink")} value={c.mqtt.downlinkTopic} />
                  <CopyRow label={t("connect.event")} value={c.mqtt.eventTopic} />
                  <CopyRow label={t("connect.peer")} value={c.mqtt.peerTopic} />
                  <CopyRow label={t("connect.sample")} value={c.mqtt.samplePayload} />
                </>
              )}

              {c.protocol === "coap" && c.coap && (
                <>
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
                </>
              )}

              {c.protocol === "custom" && c.custom && (
                <>
                  <CopyRow label={t("connect.server")} value={`${c.host}:${c.custom.tcpPort}`} />
                  <CopyRow label={t("connect.authFrame")} value={JSON.stringify(c.custom.authFrame)} />
                  <CopyRow label={t("connect.propertyFrame")} value={JSON.stringify(c.custom.propertyFrame)} />
                </>
              )}

              <div className="rounded-md bg-muted/40 p-3 text-xs text-muted-foreground">
                {t("connect.workspaceHint", { ws: c.workspace })}
              </div>
            </TabsContent>

            <TabsContent value="quickstart" className="space-y-3">
              <p className="text-xs text-muted-foreground">{t("connect.quickStartDesc")}</p>
              <div className="flex flex-wrap items-center gap-3">
                <Label>{t("connect.lang")}</Label>
                <Select value={lang} onValueChange={setLang}>
                  <SelectTrigger className="w-52">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {langsFor(c).map((l) => (
                      <SelectItem key={l.value} value={l.value}>
                        {l.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <CodeBlock code={codeFor(c, lang, t)} />
            </TabsContent>

            <TabsContent value="simulator">
              <Simulator c={c} />
            </TabsContent>
          </Tabs>
        )}
      </DialogContent>
    </Dialog>
  );
}

function CodeBlock({ code }: { code: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="relative">
      <Button
        variant="ghost"
        size="icon"
        className="absolute right-1 top-1 z-10 text-slate-300 hover:bg-slate-800 hover:text-white"
        title="Copy code"
        onClick={() => {
          void navigator.clipboard.writeText(code);
          setCopied(true);
          toast.success("Code copied");
          setTimeout(() => setCopied(false), 1500);
        }}
      >
        {copied ? <Check className="h-4 w-4 text-emerald-400" /> : <Copy className="h-4 w-4" />}
      </Button>
      <pre className="max-h-80 overflow-auto whitespace-pre-wrap rounded-md bg-slate-950 p-3 pr-12 font-mono text-xs leading-relaxed text-slate-100">
        {code}
      </pre>
    </div>
  );
}