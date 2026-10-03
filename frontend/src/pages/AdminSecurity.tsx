import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check, Copy, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { adminApi } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export default function AdminSecurityPage() {
  const { t } = useI18n();
  const qc = useQueryClient();
  const [code, setCode] = useState("");
  const [setupSecret, setSetupSecret] = useState<{ secret: string; url: string } | null>(null);

  const status = useQuery({
    queryKey: ["admin", "totp"],
    queryFn: adminApi.totpStatus,
  });

  const setup = useMutation({
    mutationFn: adminApi.totpSetup,
    onSuccess: (r) => setSetupSecret({ secret: r.secret, url: r.otpauthUrl }),
    onError: (e: Error) => toast.error(e.message),
  });

  const enable = useMutation({
    mutationFn: (c: string) => adminApi.totpEnable(c),
    onSuccess: () => {
      toast.success("Two-factor authentication enabled");
      setSetupSecret(null);
      setCode("");
      void qc.invalidateQueries({ queryKey: ["admin", "totp"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const disable = useMutation({
    mutationFn: (c: string) => adminApi.totpDisable(c),
    onSuccess: () => {
      toast.success("Two-factor authentication disabled");
      setCode("");
      void qc.invalidateQueries({ queryKey: ["admin", "totp"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="mx-auto max-w-2xl space-y-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t("admin.security")}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Two-factor authentication (TOTP, RFC 6238). After enabling, sign-in
          requires a 6-digit code from your authenticator app.
        </p>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <ShieldCheck className="h-5 w-5 text-primary" />
            <CardTitle className="text-base">Status</CardTitle>
          </div>
          <CardDescription>
            {status.data?.enabled
              ? "Enabled — a one-time code is required at sign-in."
              : "Disabled — username + password only."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {!status.data?.enabled && !setupSecret && (
            <Button onClick={() => setup.mutate()} disabled={setup.isPending}>
              Enable two-factor
            </Button>
          )}

          {setupSecret && (
            <div className="space-y-3 rounded-md border p-4">
              <div className="flex items-start gap-2 text-sm">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
                <p>
                  Scan this provisioning URL with an authenticator app
                  (Google Authenticator, 1Password, …); the secret is shown once.
                  Then enter the 6-digit code below to activate.
                </p>
              </div>
              <div className="space-y-1">
                <Label className="text-xs text-muted-foreground">Provisioning URL</Label>
                <div className="flex items-center gap-2">
                  <code className="flex-1 truncate rounded border bg-muted px-2 py-1 text-xs">{setupSecret.url}</code>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      void navigator.clipboard.writeText(setupSecret.url);
                      toast.success("Copied");
                    }}
                  >
                    <Copy className="h-4 w-4" />
                  </Button>
                </div>
                <p className="pt-1 text-xs text-muted-foreground">
                  Secret: <code className="font-mono">{setupSecret.secret}</code>
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="code">Verify code</Label>
                <Input
                  id="code"
                  inputMode="numeric"
                  maxLength={6}
                  placeholder="000000"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                />
              </div>
              <div className="flex gap-2">
                <Button onClick={() => code && enable.mutate(code)} disabled={!code || enable.isPending}>
                  <Check className="mr-1 h-4 w-4" /> Activate
                </Button>
                <Button variant="ghost" onClick={() => setSetupSecret(null)}>
                  Cancel
                </Button>
              </div>
            </div>
          )}

          {status.data?.enabled && (
            <div className="space-y-3">
              <p className="text-sm">To disable, enter a current code:</p>
              <div className="flex max-w-xs gap-2">
                <Input
                  inputMode="numeric"
                  maxLength={6}
                  placeholder="000000"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                />
                <Button variant="outline" onClick={() => code && disable.mutate(code)} disabled={!code || disable.isPending}>
                  Disable
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}