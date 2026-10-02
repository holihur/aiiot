import { NavLink } from "react-router-dom";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

export function ProjectNav({ projectId }: { projectId: number }) {
  const { t } = useI18n();
  const base = `/projects/${projectId}`;
  const tabs = [
    { to: base, label: "tab.overview", end: true },
    { to: `${base}/devices`, label: "tab.devices", end: false },
    { to: `${base}/groups`, label: "tab.groups", end: false },
    { to: `${base}/rules`, label: "tab.rules", end: false },
    { to: `${base}/ota`, label: "tab.ota", end: false },
    { to: `${base}/alerts`, label: "tab.alerts", end: false },
  ];
  return (
    <div className="flex flex-wrap gap-1 border-b">
      {tabs.map((tab) => (
        <NavLink
          key={tab.to}
          to={tab.to}
          end={tab.end}
          className={({ isActive }) =>
            cn(
              "-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors",
              isActive
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )
          }
        >
          {t(tab.label)}
        </NavLink>
      ))}
    </div>
  );
}
