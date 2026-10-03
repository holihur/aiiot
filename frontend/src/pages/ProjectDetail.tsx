import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Layers, Plus, UserPlus, Trash2, Pencil } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ProjectNav } from "@/components/ProjectNav";
import { ConfirmButton } from "@/components/ConfirmButton";
import { useI18n } from "@/lib/i18n";

export default function ProjectDetailPage() {
  const projectId = Number(useParams().projectId);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { t } = useI18n();
  const project = useQuery({ queryKey: ["project", projectId], queryFn: () => api.getProject(projectId) });
  const workspaces = useQuery({ queryKey: ["workspaces", projectId], queryFn: () => api.listWorkspaces(projectId) });
  const members = useQuery({ queryKey: ["members", projectId], queryFn: () => api.listMembers(projectId) });
  const fences = useQuery({ queryKey: ["geofences", projectId], queryFn: () => api.listGeofences(projectId) });
  const [fenceOpen, setFenceOpen] = useState(false);
  const [fenceForm, setFenceForm] = useState({ name: "", polygon: "" });
  const createFence = useMutation({
    mutationFn: () => {
      const coordinates = JSON.parse(fenceForm.polygon) as number[][][];
      return api.createGeofence(projectId, { name: fenceForm.name.trim(), polygon: { type: "Polygon", coordinates } });
    },
    onSuccess: () => {
      toast.success("Geofence created");
      setFenceOpen(false);
      setFenceForm({ name: "", polygon: "" });
      void qc.invalidateQueries({ queryKey: ["geofences", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const toggleFence = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) => {
      const f = fences.data?.find((x) => x.id === id);
      if (!f) throw new Error("missing fence");
      return api.updateGeofence(id, { name: f.name, polygon: f.polygon, enabled });
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["geofences", projectId] }),
    onError: (e: Error) => toast.error(e.message),
  });
  const removeFence = useMutation({
    mutationFn: (id: number) => api.deleteGeofence(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["geofences", projectId] }),
    onError: (e: Error) => toast.error(e.message),
  });
  // Role-aware UI: viewers get read-only affordances.
  const canManage = ["owner", "admin", "member"].includes(project.data?.myRole ?? "");
  const canAdmin = ["owner", "admin"].includes(project.data?.myRole ?? "");

  const [projectEdit, setProjectEdit] = useState<{ name: string; description: string } | null>(null);
  const updateProject = useMutation({
    mutationFn: () =>
      api.updateProject(projectId, { name: projectEdit!.name, description: projectEdit!.description }),
    onSuccess: () => {
      toast.success(t("common.save"));
      setProjectEdit(null);
      void qc.invalidateQueries({ queryKey: ["project", projectId] });
      void qc.invalidateQueries({ queryKey: ["projects"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const deleteProject = useMutation({
    mutationFn: () => api.deleteProject(projectId),
    onSuccess: () => {
      toast.success(t("common.delete"));
      navigate("/projects");
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const [wsOpen, setWsOpen] = useState(false);
  const [wsForm, setWsForm] = useState({ key: "", name: "", description: "" });
  const [memberOpen, setMemberOpen] = useState(false);
  const [memberForm, setMemberForm] = useState({ username: "", role: "member" });

  const createWs = useMutation({
    mutationFn: () => api.createWorkspace(projectId, wsForm),
    onSuccess: () => {
      toast.success("Workspace created");
      setWsOpen(false);
      setWsForm({ key: "", name: "", description: "" });
      void qc.invalidateQueries({ queryKey: ["workspaces", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const deleteWs = useMutation({
    mutationFn: (id: number) => api.deleteWorkspace(id),
    onSuccess: () => {
      toast.success("Workspace deleted");
      void qc.invalidateQueries({ queryKey: ["workspaces", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const [wsEdit, setWsEdit] = useState<{ id: number; name: string; description: string } | null>(null);
  const updateWs = useMutation({
    mutationFn: () =>
      api.updateWorkspace(wsEdit!.id, { name: wsEdit!.name, description: wsEdit!.description }),
    onSuccess: () => {
      toast.success("Workspace updated");
      setWsEdit(null);
      void qc.invalidateQueries({ queryKey: ["workspaces", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const addMember = useMutation({
    mutationFn: () => api.addMember(projectId, memberForm),
    onSuccess: () => {
      toast.success("Member added");
      setMemberOpen(false);
      setMemberForm({ username: "", role: "member" });
      void qc.invalidateQueries({ queryKey: ["members", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const removeMember = useMutation({
    mutationFn: (memberId: number) => api.removeMember(projectId, memberId),
    onSuccess: () => {
      toast.success("Member removed");
      void qc.invalidateQueries({ queryKey: ["members", projectId] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-3">
            <h1 className="min-w-0 truncate text-2xl font-semibold tracking-tight whitespace-nowrap">
              {project.data?.name ?? t("project.title")}
            </h1>
            <code className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{project.data?.key}</code>
          </div>
          <p className="text-sm text-muted-foreground">{project.data?.description || t("projects.noDescription")}</p>
        </div>
        {canAdmin && (
          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={() =>
                setProjectEdit({ name: project.data?.name ?? "", description: project.data?.description ?? "" })
              }
            >
              <Pencil className="h-4 w-4" /> {t("common.edit")}
            </Button>
            <ConfirmButton
              title={t("project.deleteConfirm")}
              description={t("project.deleteDesc")}
              onConfirm={() => deleteProject.mutateAsync()}
            >
              <Button variant="destructive">
                <Trash2 className="h-4 w-4" /> {t("common.delete")}
              </Button>
            </ConfirmButton>
          </div>
        )}
      </div>

      <ProjectNav projectId={projectId} />

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2 text-base">
                <Layers className="h-4 w-4" /> {t("project.workspaces")}
              </CardTitle>
              <CardDescription>{t("project.workspacesDesc")}</CardDescription>
            </div>
            <Dialog open={wsOpen} onOpenChange={setWsOpen}>
              <DialogTrigger asChild>
                <Button size="sm">
                  <Plus className="h-4 w-4" /> {t("project.add")}
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>{t("project.newWorkspace")}</DialogTitle>
                  <DialogDescription>{t("project.newWorkspaceDesc")}</DialogDescription>
                </DialogHeader>
                <div className="space-y-4">
                  <div className="space-y-2">
                    <Label>{t("common.key")}</Label>
                    <Input value={wsForm.key} onChange={(e) => setWsForm({ ...wsForm, key: e.target.value })} placeholder="factory1" />
                  </div>
                  <div className="space-y-2">
                    <Label>{t("common.name")}</Label>
                    <Input value={wsForm.name} onChange={(e) => setWsForm({ ...wsForm, name: e.target.value })} />
                  </div>
                </div>
                <DialogFooter>
                  <Button onClick={() => createWs.mutate()} disabled={createWs.isPending || !wsForm.key || !wsForm.name}>
                    {t("common.create")}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          </CardHeader>
          <CardContent className="space-y-2">
            {(workspaces.data ?? []).length === 0 && <p className="text-sm text-muted-foreground">{t("project.noWorkspaces")}</p>}
            {(workspaces.data ?? []).map((w) => (
              <div key={w.id} className="flex items-center justify-between rounded-lg border p-3 text-sm">
                <div>
                  <div className="font-medium">{w.name}</div>
                  <code className="text-xs text-muted-foreground">{w.key}</code>
                </div>
                {canAdmin && (
                  <div className="flex items-center">
                    <Button
                      variant="ghost"
                      size="icon"
                      title="Edit workspace"
                      onClick={() => setWsEdit({ id: w.id, name: w.name, description: w.description ?? "" })}
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => deleteWs.mutate(w.id)}
                      title="Delete workspace"
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle className="text-base">{t("project.members")}</CardTitle>
              <CardDescription>{t("project.membersDesc")}</CardDescription>
            </div>
            <Dialog open={memberOpen} onOpenChange={setMemberOpen}>
              <DialogTrigger asChild>
                <Button size="sm">
                  <UserPlus className="h-4 w-4" /> {t("project.add")}
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>{t("project.addMember")}</DialogTitle>
                  <DialogDescription>{t("project.addMemberDesc")}</DialogDescription>
                </DialogHeader>
                <div className="space-y-4">
                  <div className="space-y-2">
                    <Label>{t("project.username")}</Label>
                    <Input value={memberForm.username} onChange={(e) => setMemberForm({ ...memberForm, username: e.target.value })} />
                  </div>
                  <div className="space-y-2">
                    <Label>{t("project.role")}</Label>
                    <Select value={memberForm.role} onValueChange={(v) => setMemberForm({ ...memberForm, role: v })}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="viewer">viewer</SelectItem>
                        <SelectItem value="member">member</SelectItem>
                        <SelectItem value="admin">admin</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <DialogFooter>
                  <Button onClick={() => addMember.mutate()} disabled={addMember.isPending || !memberForm.username}>
                    {t("project.add")}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          </CardHeader>
          <CardContent className="space-y-2">
            {(members.data ?? []).map((m) => (
              <div key={m.id} className="flex items-center justify-between rounded-lg border p-3 text-sm">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{m.user?.displayName || m.user?.username || `user #${m.userId}`}</span>
                  <Badge variant="secondary">{m.role}</Badge>
                </div>
                {canAdmin && (
                  <Button variant="ghost" size="icon" onClick={() => removeMember.mutate(m.id)}>
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                )}
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="text-base">Geofences <Badge variant="secondary">{fences.data?.length ?? 0}</Badge></CardTitle>
            <Button size="sm" variant="outline" onClick={() => setFenceOpen(true)}>
              <Plus className="h-4 w-4" /> Add
            </Button>
          </CardHeader>
          <CardContent className="space-y-2">
            {(fences.data ?? []).length === 0 && (
              <p className="text-sm text-muted-foreground">
                No fences yet. Devices reporting a gps attribute inside a fence are tagged so rules can match
                {" "}"zone" in params["geofences"].
              </p>
            )}
            {(fences.data ?? []).map((f) => (
              <div key={f.id} className="flex items-center justify-between rounded-lg border p-3 text-sm">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{f.name}</span>
                  <Badge variant={f.enabled ? "success" : "secondary"}>{f.enabled ? "on" : "off"}</Badge>
                </div>
                <div className="flex items-center gap-2">
                  <Switch checked={f.enabled} onCheckedChange={(v) => toggleFence.mutate({ id: f.id, enabled: v })} />
                  <Button variant="ghost" size="icon" onClick={() => removeFence.mutate(f.id)}>
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
      </div>

      <Dialog open={!!wsEdit} onOpenChange={(o) => !o && setWsEdit(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("project.editWorkspace")}</DialogTitle>
            <DialogDescription>{t("project.editWorkspaceDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("common.name")}</Label>
              <Input value={wsEdit?.name ?? ""} onChange={(e) => setWsEdit((s) => (s ? { ...s, name: e.target.value } : s))} />
            </div>
            <div className="space-y-2">
              <Label>{t("common.description")}</Label>
              <Input
                value={wsEdit?.description ?? ""}
                onChange={(e) => setWsEdit((s) => (s ? { ...s, description: e.target.value } : s))}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setWsEdit(null)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={() => updateWs.mutate()} disabled={updateWs.isPending || !wsEdit?.name}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={!!projectEdit} onOpenChange={(o) => !o && setProjectEdit(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("project.editTitle")}</DialogTitle>
            <DialogDescription>{t("project.editDesc")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t("common.name")}</Label>
              <Input
                value={projectEdit?.name ?? ""}
                onChange={(e) => setProjectEdit((s) => (s ? { ...s, name: e.target.value } : s))}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("common.description")}</Label>
              <Input
                value={projectEdit?.description ?? ""}
                onChange={(e) => setProjectEdit((s) => (s ? { ...s, description: e.target.value } : s))}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setProjectEdit(null)}>
              {t("common.cancel")}
            </Button>
            <Button onClick={() => updateProject.mutate()} disabled={updateProject.isPending || !projectEdit?.name}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={fenceOpen} onOpenChange={setFenceOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New geofence</DialogTitle>
            <DialogDescription>Paste a GeoJSON polygon ring coordinates ([[lng,lat],...], first == last).</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-2">
              <Label>Name</Label>
              <Input value={fenceForm.name} onChange={(e) => setFenceForm({ ...fenceForm, name: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>Polygon coordinates</Label>
              <Textarea
                className="font-mono text-xs"
                rows={4}
                placeholder="[[[121.46,31.22],[121.49,31.22],[121.49,31.24],[121.46,31.24],[121.46,31.22]]]"
                value={fenceForm.polygon}
                onChange={(e) => setFenceForm({ ...fenceForm, polygon: e.target.value })}
              />
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setFenceOpen(false)}>{t("common.cancel")}</Button>
              <Button
                onClick={() => createFence.mutate()}
                disabled={createFence.isPending || !fenceForm.name.trim() || !fenceForm.polygon.trim()}
              >
                {t("common.save")}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
