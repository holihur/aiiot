import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, ScrollText, Search } from "lucide-react";
import { adminApi } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatTime } from "@/lib/utils";

export default function AuditPage() {
  const { t } = useI18n();
  const [path, setPath] = useState("");
  const [projectId, setProjectId] = useState("");
  const logs = useQuery({
    queryKey: ["admin", "audit", path, projectId],
    queryFn: () =>
      adminApi.listAuditLogs({
        path: path || undefined,
        projectId: projectId ? Number(projectId) : undefined,
        limit: 300,
      }),
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight whitespace-nowrap">{t("audit.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("audit.subtitle")}</p>
        </div>
        <Button variant="outline" onClick={() => logs.refetch()}>
          <RefreshCw className="h-4 w-4" /> {t("common.refresh")}
        </Button>
      </div>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="relative w-full sm:max-w-xs">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input className="pl-8" placeholder={t("audit.filterPath")} value={path} onChange={(e) => setPath(e.target.value)} />
        </div>
        <Input
          className="w-full sm:w-44"
          placeholder={t("audit.projectId")}
          value={projectId}
          onChange={(e) => setProjectId(e.target.value)}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ScrollText className="h-4 w-4" /> {t("audit.recent")}
          </CardTitle>
          <CardDescription>{t("audit.recentDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("audit.time")}</TableHead>
                <TableHead className="hidden md:table-cell">{t("audit.user")}</TableHead>
                <TableHead>{t("audit.method")}</TableHead>
                <TableHead>{t("audit.path")}</TableHead>
                <TableHead>{t("common.status")}</TableHead>
                <TableHead className="hidden md:table-cell">IP</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(logs.data ?? []).length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="py-8 text-center text-muted-foreground">
                    {t("audit.empty")}
                  </TableCell>
                </TableRow>
              )}
              {(logs.data ?? []).map((l) => (
                <TableRow key={l.id}>
                  <TableCell className="whitespace-nowrap text-xs">{formatTime(l.createdAt)}</TableCell>
                  <TableCell className="hidden text-sm md:table-cell">{l.username || `#${l.userId}`}</TableCell>
                  <TableCell>
                    <Badge variant="outline">{l.method}</Badge>
                  </TableCell>
                  <TableCell className="max-w-md truncate font-mono text-xs">{l.path}</TableCell>
                  <TableCell>
                    <Badge variant={l.status < 400 ? "success" : "destructive"}>{l.status}</Badge>
                  </TableCell>
                  <TableCell className="hidden text-xs text-muted-foreground md:table-cell">{l.ip}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
