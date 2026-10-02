import { useState } from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { Boxes, LayoutDashboard, LogOut, Menu, Radio, FolderKanban, X } from "lucide-react";
import { useAuth } from "@/lib/auth";
import { ProjectSwitcher } from "@/components/ProjectSwitcher";
import { LanguageSwitcher, useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

const operations = [
  { to: "/", label: "nav.dashboard", icon: LayoutDashboard, end: true },
  { to: "/projects", label: "nav.projects", icon: FolderKanban, end: false },
  { to: "/products", label: "nav.products", icon: Boxes, end: false },
];

export function AppLayout() {
  const { user, logout } = useAuth();
  const { t } = useI18n();
  const navigate = useNavigate();
  const [mobileOpen, setMobileOpen] = useState(false);

  const signOut = () => {
    logout();
    navigate("/login");
  };

  const navLinks = (items: typeof operations, onNavigate?: () => void) =>
    items.map((item) => (
      <NavLink
        key={item.to}
        to={item.to}
        end={item.end}
        onClick={onNavigate}
        className={({ isActive }) =>
          cn(
            "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
            isActive ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-accent hover:text-foreground",
          )
        }
      >
        <item.icon className="h-4 w-4" />
        {t(item.label)}
      </NavLink>
    ));

  const sectionLabel = (key: string, top = false) => (    <div className={cn("px-3 pb-1 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground", top ? "pt-4" : "pt-2")}>
      {t(key)}
    </div>
  );

  const navigation = (onNavigate?: () => void) => (
    <>
      {sectionLabel("nav.section.operations")}
      {navLinks(operations, onNavigate)}
    </>
  );

  return (
    <div className="flex min-h-screen bg-muted/30">
      {/* Desktop sidebar */}
      <aside className="hidden w-60 flex-col border-r bg-background md:flex">
        <div className="flex h-14 items-center gap-2 border-b px-5">
          <Radio className="h-5 w-5 text-primary" />
          <span className="font-semibold tracking-tight">{t("app.title")}</span>
        </div>
        <div className="border-b p-3">
          <ProjectSwitcher />
        </div>
        <nav className="flex-1 space-y-1 overflow-y-auto p-3">{navigation()}</nav>
        <div className="border-t p-3">
          <div className="mb-2 flex items-center justify-between px-2">
            <div className="min-w-0">
              <div className="truncate text-sm font-medium">{user?.displayName || user?.username}</div>
              <div className="truncate text-xs text-muted-foreground">{user?.email}</div>
            </div>
            {user?.systemRole === "admin" && <Badge variant="secondary">admin</Badge>}
          </div>
          <Button variant="outline" size="sm" className="w-full" onClick={signOut}>
            <LogOut className="h-4 w-4" /> {t("nav.signOut")}
          </Button>
          <div className="mt-2 flex justify-center">
            <LanguageSwitcher />
          </div>
        </div>
      </aside>

      {/* Mobile slide-over navigation */}
      {mobileOpen && (
        <div className="fixed inset-0 z-50 md:hidden">
          <div className="absolute inset-0 bg-black/50" onClick={() => setMobileOpen(false)} />
          <aside className="absolute left-0 top-0 flex h-full w-72 max-w-[85%] flex-col bg-background shadow-xl">
            <div className="flex h-14 items-center justify-between border-b px-4">
              <div className="flex items-center gap-2">
                <Radio className="h-5 w-5 text-primary" />
                <span className="font-semibold">{t("app.title")}</span>
              </div>
              <Button variant="ghost" size="icon" onClick={() => setMobileOpen(false)}>
                <X className="h-5 w-5" />
              </Button>
            </div>
            <div className="border-b p-3">
              <ProjectSwitcher onNavigate={() => setMobileOpen(false)} />
            </div>
            <nav className="flex-1 space-y-1 overflow-y-auto p-3">{navigation(() => setMobileOpen(false))}</nav>
            <div className="border-t p-3">
              <Button variant="outline" size="sm" className="w-full" onClick={signOut}>
                <LogOut className="h-4 w-4" /> {t("nav.signOut")}
              </Button>
              <div className="mt-2 flex justify-center">
                <LanguageSwitcher />
              </div>
            </div>
          </aside>
        </div>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b bg-background px-4 md:hidden">
          <Button variant="ghost" size="icon" onClick={() => setMobileOpen(true)}>
            <Menu className="h-5 w-5" />
          </Button>
          <Radio className="h-5 w-5 text-primary" />
          <span className="font-semibold">{t("app.title")}</span>
          {user?.systemRole === "admin" && <Badge variant="secondary" className="ml-auto">admin</Badge>}
        </header>
        <main className="flex-1 p-4 sm:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
