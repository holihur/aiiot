import { useLocation, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, FolderKanban } from "lucide-react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

// ProjectSwitcher is the primary way to move between projects from anywhere.
export function ProjectSwitcher({ onNavigate }: { onNavigate?: () => void }) {
  const navigate = useNavigate();
  const location = useLocation();
  const { t } = useI18n();
  const { data: projects } = useQuery({ queryKey: ["projects"], queryFn: api.listProjects });

  const match = location.pathname.match(/\/projects\/(\d+)/);
  const currentId = match ? Number(match[1]) : Number(localStorage.getItem("aiiot_project") || 0);
  const current = (projects ?? []).find((p) => p.id === currentId);

  function go(id: number) {
    localStorage.setItem("aiiot_project", String(id));
    navigate(`/projects/${id}`);
    onNavigate?.();
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button className="flex w-full items-center gap-2 rounded-md border px-3 py-2 text-left text-sm hover:bg-accent">
          <FolderKanban className="h-4 w-4 text-muted-foreground" />
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium">{current?.name ?? t("project.select")}</span>
            {current && <span className="block truncate text-xs text-muted-foreground">{current.key}</span>}
          </span>
          <ChevronDown className="h-4 w-4 text-muted-foreground" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-60">
        <DropdownMenuLabel>{t("project.switch")}</DropdownMenuLabel>
        {(projects ?? []).length === 0 && (
          <DropdownMenuItem disabled>{t("project.none")}</DropdownMenuItem>
        )}
        {(projects ?? []).map((p) => (
          <DropdownMenuItem key={p.id} onClick={() => go(p.id)}>
            <span className="truncate">{p.name}</span>
            <span className="ml-auto text-xs text-muted-foreground">{p.key}</span>
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => {
            navigate("/projects");
            onNavigate?.();
          }}
        >
          {t("project.manage")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
