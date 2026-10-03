import { Suspense, lazy } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { Loader2 } from "lucide-react";
import { useAuth } from "./lib/auth";
import { useI18n } from "./lib/i18n";
import { getAdminToken } from "./lib/api";
import { AppLayout } from "./components/layout/AppLayout";
import { AdminLayout } from "./components/layout/AdminLayout";
const LoginPage = lazy(() => import("./pages/Login"));
const RegisterPage = lazy(() => import("./pages/Register"));
const AdminLoginPage = lazy(() => import("./pages/AdminLogin"));
const AdminSecurityPage = lazy(() => import("./pages/AdminSecurity"));
const DashboardPage = lazy(() => import("./pages/Dashboard"));
const ProjectsPage = lazy(() => import("./pages/Projects"));
const ProjectDetailPage = lazy(() => import("./pages/ProjectDetail"));
const ProductsPage = lazy(() => import("./pages/Products"));
const ProductDetailPage = lazy(() => import("./pages/ProductDetail"));
const DevicesPage = lazy(() => import("./pages/Devices"));
const DeviceDetailPage = lazy(() => import("./pages/DeviceDetail"));
const RulesPage = lazy(() => import("./pages/Rules"));
const OtaPage = lazy(() => import("./pages/Ota"));
const AlertsPage = lazy(() => import("./pages/Alerts"));
const StoragePage = lazy(() => import("./pages/Storage"));
const GroupsPage = lazy(() => import("./pages/Groups"));
const AuditPage = lazy(() => import("./pages/Audit"));
const GatewaysPage = lazy(() => import("./pages/Gateways"));
const NatsPage = lazy(() => import("./pages/Nats"));

function FullPageLoader() {
  return (
    <div className="flex h-screen items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  );
}

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <FullPageLoader />;
  if (!user) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

// RequireAdminToken guards the isolated administration console, which uses its
// own token rather than a business-user session.
function RequireAdminToken({ children }: { children: React.ReactNode }) {
  if (!getAdminToken()) return <Navigate to="/admin/login" replace />;
  return <>{children}</>;
}

export default function App() {
  return (
    <Suspense fallback={<PageLoader />}>
      <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/admin/login" element={<AdminLoginPage />} />

      {/* Business / operations application */}
      <Route
        element={
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<DashboardPage />} />
        <Route path="/projects" element={<ProjectsPage />} />
        <Route path="/projects/:projectId" element={<ProjectDetailPage />} />
        <Route path="/projects/:projectId/devices" element={<DevicesPage />} />
        <Route path="/projects/:projectId/groups" element={<GroupsPage />} />
        <Route path="/projects/:projectId/rules" element={<RulesPage />} />
        <Route path="/projects/:projectId/ota" element={<OtaPage />} />
        <Route path="/projects/:projectId/alerts" element={<AlertsPage />} />
        <Route path="/products" element={<ProductsPage />} />
        <Route path="/products/:productId" element={<ProductDetailPage />} />
        <Route path="/devices/:deviceId" element={<DeviceDetailPage />} />
        {/* Legacy administration paths now live in the admin console. */}
        <Route path="/gateways" element={<Navigate to="/admin/gateways" replace />} />
        <Route path="/storage" element={<Navigate to="/admin/storage" replace />} />
        <Route path="/audit" element={<Navigate to="/admin/audit" replace />} />
      </Route>

      {/* Isolated administration console */}
      <Route
        path="/admin"
        element={
          <RequireAdminToken>
            <AdminLayout />
          </RequireAdminToken>
        }
      >
        <Route index element={<Navigate to="/admin/gateways" replace />} />
        <Route path="gateways" element={<GatewaysPage />} />
        <Route path="nats" element={<NatsPage />} />
        <Route path="storage" element={<StoragePage />} />
        <Route path="audit" element={<AuditPage />} />
        <Route path="security" element={<AdminSecurityPage />} />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
    </Suspense>
  );
}


// PageLoader is a lightweight full-screen loading state shown while a
// route chunk is being fetched.
function PageLoader() {
  const { t } = useI18n();
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-background">
      <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
    </div>
  );
}
