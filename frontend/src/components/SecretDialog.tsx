import { useState } from "react";
import { Check, Copy, Download, KeyRound, TriangleAlert } from "lucide-react";
import { toast } from "sonner";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface SecretDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  secret: string;
  /** Optional context line, e.g. the device name. */
  subject?: string;
  /** Base name for the downloaded file (without extension). */
  filename?: string;
}

// SecretDialog reveals a one-time credential with explicit copy/download
// affordances. A transient toast is too easy to miss for values that are never
// shown again.
export function SecretDialog({ open, onOpenChange, secret, subject, filename }: SecretDialogProps) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);

  const copy = () => {
    void navigator.clipboard.writeText(secret);
    setCopied(true);
    toast.success(t("secret.copied"));
    setTimeout(() => setCopied(false), 1500);
  };

  const download = () => {
    const body = `${subject ? subject + "\n" : ""}${t("secret.passwordLabel")}: ${secret}\n`;
    const blob = new Blob([body], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${filename || "secret"}.txt`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyRound className="h-4 w-4" /> {t("secret.title")}
          </DialogTitle>
          <DialogDescription>{t("secret.desc")}</DialogDescription>
        </DialogHeader>

        <div className="flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-2 text-xs text-amber-700 dark:text-amber-400">
          <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{t("secret.warning")}</span>
        </div>

        {subject && <div className="text-sm text-muted-foreground">{subject}</div>}

        <div className="flex items-center gap-2 rounded-md border bg-muted/40 p-2">
          <code className="min-w-0 flex-1 truncate font-mono text-sm">{secret}</code>
          <Button variant="ghost" size="icon" onClick={copy} title={t("secret.copy")} aria-label={t("secret.copy")}>
            {copied ? <Check className="h-4 w-4 text-emerald-500" /> : <Copy className="h-4 w-4" />}
          </Button>
        </div>

        <DialogFooter className="gap-2 sm:justify-between">
          <Button variant="outline" onClick={download}>
            <Download className="h-4 w-4" /> {t("secret.download")}
          </Button>
          <Button onClick={() => onOpenChange(false)}>{t("secret.close")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
