// GlobalSearch is a command-palette style quick search across the current
// project's devices plus products/projects. Press / to focus, type to filter,
// arrow keys + Enter to jump, Esc to close.

import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Boxes, FolderKanban, Search, Users, Wifi, Zap } from "lucide-react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { Input } from "@/components/ui/input";

export default function GlobalSearch() {
  const navigate = useNavigate();
  const { t } = useI18n();
  const inputRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const [active, setActive] = useState(0);

  const projects = useQuery({ queryKey: ["projects"], queryFn: api.listProjects });
  const products = useQuery({ queryKey: ["products"], queryFn: () => api.listProducts() });
  const search = useQuery({
    queryKey: ["search", q],
    queryFn: () => api.search(q),
    enabled: q.trim().length > 0,
  });

  const rows = useMemo(() => {
    const list: { kind: "device" | "product" | "project" | "rule" | "group"; id: number; title: string; sub: string; to: string }[] = [];
    for (const x of search.data?.devices ?? []) {
      list.push({ kind: "device", id: x.id, title: x.name || x.key, sub: `${t("search.device")} · ${x.key}`, to: `/devices/${x.id}` });
    }
    for (const x of search.data?.rules ?? []) {
      list.push({ kind: "rule", id: x.id, title: x.name, sub: t("search.rule"), to: `/projects/${x.projectId}/rules` });
    }
    for (const x of search.data?.groups ?? []) {
      list.push({ kind: "group", id: x.id, title: x.name, sub: t("search.group"), to: `/projects/${x.projectId}/groups` });
    }
    for (const x of (products.data ?? [])) {
      if (!q || x.name.toLowerCase().includes(q.toLowerCase()) || x.key.toLowerCase().includes(q.toLowerCase())) {
        list.push({ kind: "product", id: x.id, title: x.name, sub: `${t("search.product")} · ${x.protocol}`, to: `/products/${x.id}` });
      }
    }
    for (const x of (projects.data ?? [])) {
      if (!q || x.name.toLowerCase().includes(q.toLowerCase()) || x.key.toLowerCase().includes(q.toLowerCase())) {
        list.push({ kind: "project", id: x.id, title: x.name, sub: `${t("search.project")} · ${x.key}`, to: `/projects/${x.id}` });
      }
    }
    return list.slice(0, 15);
  }, [search.data, products.data, projects.data, q, t]);

  // global "/" hotkey
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "/" && !(e.target instanceof HTMLInputElement) && !(e.target instanceof HTMLTextAreaElement)) {
        e.preventDefault();
        setOpen((v) => !v);
      }
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    if (open) setTimeout(() => inputRef.current?.focus(), 10);
    else setQ("");
  }, [open]);

  useEffect(() => setActive(0), [q]);

  const jump = (to: string) => {
    setOpen(false);
    navigate(to);
  };

  return (
    <>
      {/* trigger */}
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex h-9 w-full items-center gap-2 rounded-md border bg-muted/40 px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted"
      >
        <Search className="h-4 w-4" />
        <span className="flex-1 text-left">{t("search.placeholder")}</span>
        <kbd className="rounded border bg-background px-1.5 py-0.5 text-[10px]">/</kbd>
      </button>

      {open && (
        <div className="fixed inset-0 z-[60] bg-black/40" onClick={() => setOpen(false)}>
          <div
            className="mx-auto mt-16 w-full max-w-lg overflow-hidden rounded-xl border bg-background shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center gap-2 border-b px-3">
              <Search className="h-4 w-4 text-muted-foreground" />
              <Input
                ref={inputRef}
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder={t("search.inputPlaceholder")}
                className="border-0 shadow-none focus-visible:ring-0"
                onKeyDown={(e) => {
                  if (e.key === "ArrowDown") {
                    e.preventDefault();
                    setActive((a) => Math.min(a + 1, rows.length - 1));
                  } else if (e.key === "ArrowUp") {
                    e.preventDefault();
                    setActive((a) => Math.max(a - 1, 0));
                  } else if (e.key === "Enter" && rows[active]) {
                    jump(rows[active].to);
                  }
                }}
              />
            </div>
            <div className="max-h-80 overflow-y-auto p-1">
              {rows.length === 0 ? (
                <div className="px-3 py-6 text-center text-sm text-muted-foreground">
                  {q ? t("search.noResults") : t("search.hint")}
                </div>
              ) : (
                rows.map((r, i) => {
                  const Icon = r.kind === "device" ? Wifi : r.kind === "product" ? Boxes : r.kind === "project" ? FolderKanban : r.kind === "rule" ? Zap : Users;
                  return (
                    <button
                      key={`${r.kind}-${r.id}`}
                      type="button"
                      onMouseEnter={() => setActive(i)}
                      onClick={() => jump(r.to)}
                      className={`flex w-full items-center gap-3 rounded-md px-3 py-2 text-left text-sm ${
                        i === active ? "bg-primary/10 text-primary" : ""
                      }`}
                    >
                      <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate">{r.title}</span>
                      <span className="shrink-0 text-xs text-muted-foreground">{r.sub}</span>
                    </button>
                  );
                })
              )}
            </div>
          </div>
        </div>
      )}
    </>
  );
}