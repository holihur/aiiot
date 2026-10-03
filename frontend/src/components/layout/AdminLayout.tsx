import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { Database, LogOut, Network, ScrollText, Send, ShieldCheck } from "lucide-react";
import { setAdminToken } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { ThemeToggle } from "@/lib/theme";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

const items = [
  { to: "/admin/gateways", label: "gateways.title", icon: Network },
  { to: "/admin/nats", label: "nats.title", icon: Send },
  { to: "/admin/storage", label: "storage.title", icon: Database },
  { to: "/admin/audit", label: "audit.title", icon: ScrollText },
  { to: "/admin/security", label: "admin.security", icon: ShieldCheck },
];

// AdminLayout is the shell for the administration console, deliberately
// separate from the business application shell.
export function AdminLayout() {
  const navigate = useNavigate();
  const { t } = useI18n();
  return (
    <div className="flex min-h-screen bg-muted/30">
      <aside className="hidden w-56 flex-col border-r bg-background md:flex">
        <div className="flex h-14 items-center gap-2 border-b px-4">
          <ShieldCheck className="h-5 w-5 text-primary" />
          <span className="font-semibold tracking-tight">{t("admin.console")}</span>
        </div>
        <nav className="flex-1 space-y-1 p-3">
          {items.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
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
          ))}
        </nav>
        <div className="border-t p-3">
          <div className="mb-2 flex justify-end">
            <ThemeToggle />
          </div>
          <Button
            variant="outline"
            size="sm"
            className="w-full"
            onClick={() => {
              setAdminToken(null);
              navigate("/admin/login");
            }}
          >
            <LogOut className="h-4 w-4" /> {t("admin.signOut")}
          </Button>
        </div>
      </aside>
      <main className="flex-1 p-4 sm:p-6">
        <Outlet />
      </main>
    </div>
  );
}
