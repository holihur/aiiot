import { useState } from "react";
import { ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ConfirmButton } from "@/components/ConfirmButton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

// AccountSecurityDialog exposes password change and "sign out everywhere".
// Both operations revoke outstanding tokens server-side; the fresh token
// returned by the API keeps this session signed in.
export function AccountSecurityDialog() {
  const { t } = useI18n();
  const { applyToken } = useAuth();
  const [open, setOpen] = useState(false);
  const [oldPw, setOldPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [busy, setBusy] = useState(false);

  async function changePassword() {
    setBusy(true);
    try {
      const r = await api.changePassword(oldPw, newPw);
      applyToken(r.token);
      toast.success(t("account.passwordChanged"));
      setOldPw("");
      setNewPw("");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function revoke() {
    try {
      const r = await api.revokeSessions();
      applyToken(r.token);
      toast.success(t("account.revoked"));
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="w-full">
          <ShieldCheck className="h-4 w-4" /> {t("account.security")}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t("account.security")}</DialogTitle>
          <DialogDescription>{t("account.securityDesc")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>{t("account.oldPassword")}</Label>
            <Input type="password" value={oldPw} onChange={(e) => setOldPw(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label>{t("account.newPassword")}</Label>
            <Input type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} />
          </div>
          <Button onClick={changePassword} disabled={busy || !oldPw || newPw.length < 8}>
            {t("account.changePassword")}
          </Button>
          <div className="border-t pt-3">
            <p className="mb-2 text-xs text-muted-foreground">{t("account.revokeDesc")}</p>
            <ConfirmButton title={t("account.revoke")} description={t("account.revokeConfirm")} onConfirm={revoke}>
              <Button variant="destructive" size="sm">
                {t("account.revoke")}
              </Button>
            </ConfirmButton>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
