import { Navigate, Route, Routes } from "react-router-dom";
import { Loader2 } from "lucide-react";
import { useAuth } from "./lib/auth";
import { getAdminToken } from "./lib/api";
import { AppLayout } from "./components/layout/AppLayout";
import { AdminLayout } from "./components/layout/AdminLayout";
import LoginPage from "./pages/Login";
import RegisterPage from "./pages/Register";
import AdminLoginPage from "./pages/AdminLogin";
import DashboardPage from "./pages/Dashboard";
import ProjectsPage from "./pages/Projects";
import ProjectDetailPage from "./pages/ProjectDetail";
import ProductsPage from "./pages/Products";
import ProductDetailPage from "./pages/ProductDetail";
import DevicesPage from "./pages/Devices";
import DeviceDetailPage from "./pages/DeviceDetail";
import RulesPage from "./pages/Rules";
import OtaPage from "./pages/Ota";
import AlertsPage from "./pages/Alerts";
import StoragePage from "./pages/Storage";
import GroupsPage from "./pages/Groups";
import AuditPage from "./pages/Audit";
import GatewaysPage from "./pages/Gateways";
import NatsPage from "./pages/Nats";

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
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
