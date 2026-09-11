
import { Suspense, lazy, useEffect, Component, ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { HelmetProvider } from "react-helmet-async";
import { I18nProvider } from "./i18n/context";
import { ToastProvider, Spinner } from "./components/ui";
import { AuthProvider, useAuth, hasCreds } from "./providers/AuthProvider";
import { Layout } from "./components/Layout";
import { isStatusOnly } from "./utils/host";

interface EBState { hasError: boolean; error?: Error }
class ErrorBoundary extends Component<{ children: ReactNode }, EBState> {
  state: EBState = { hasError: false };
  static getDerivedStateFromError(error: Error): EBState {
    return { hasError: true, error };
  }
  componentDidCatch(error: Error) {
    console.error("[VEBOX] Render xatosi:", error);
  }
  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-dvh flex-col items-center justify-center gap-4 bg-ink p-6 text-center font-mono">
          <div className="text-4xl text-err">⚠</div>
          <h1 className="text-lg text-fg">Sahifani yuklab bo'lmadi</h1>
          <p className="max-w-md text-sm text-mut">
            {this.state.error?.message || "Noma'lum xato yuz berdi."}
          </p>
          <button
            onClick={() => window.location.reload()}
            className="border border-accent bg-accent px-6 py-2 text-xs text-white hover:bg-accent2"
          >
            Sahifani yangilash
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

const Landing = lazy(() => import("./pages/Landing").then((m) => ({ default: m.Landing })));
const Login = lazy(() => import("./pages/AuthPages").then((m) => ({ default: m.Login })));
const Signup = lazy(() => import("./pages/AuthPages").then((m) => ({ default: m.Signup })));
const Console = lazy(() => import("./pages/Console").then((m) => ({ default: m.Console })));
const ShortcutsHelp = lazy(() => import("./pages/ShortcutsHelp").then((m) => ({ default: m.ShortcutsHelp })));
const MediaDetail = lazy(() => import("./pages/MediaDetail").then((m) => ({ default: m.MediaDetail })));
const Analytics = lazy(() => import("./pages/Analytics").then((m) => ({ default: m.Analytics })));
const KeysPage = lazy(() => import("./pages/Keys").then((m) => ({ default: m.KeysPage })));
const ProfilePage = lazy(() => import("./pages/Profile").then((m) => ({ default: m.ProfilePage })));
const Admin = lazy(() => import("./pages/Admin").then((m) => ({ default: m.Admin })));
const Status = lazy(() => import("./pages/Status").then((m) => ({ default: m.Status })));
const Docs = lazy(() => import("./pages/Docs").then((m) => ({ default: m.Docs })));
const Legal = lazy(() => import("./pages/Legal").then((m) => ({ default: m.Legal })));
const NotFound = lazy(() => import("./pages/NotFound").then((m) => ({ default: m.NotFound })));

function RequireAuth({ children }: { children: ReactNode }) {
  const { session, loading } = useAuth();
  if (loading) return <Spinner />;
  if (!session?.user && !hasCreds()) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

function StatusOnlyRouter() {
  const nav = useNavigate();
  const { pathname } = useLocation();
  useEffect(() => {
    if (pathname !== "/") nav("/", { replace: true });
  }, [pathname, nav]);

  return (
    <Suspense fallback={<Spinner />}>
      <Routes>
        <Route element={<Layout statusOnly />}>
          <Route path="*" element={<Status />} />
        </Route>
      </Routes>
    </Suspense>
  );
}

function Router() {
  return (
    <Suspense fallback={<Spinner />}>
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Landing />} />
        <Route path="/login" element={<Login />} />
        <Route path="/signup" element={<Signup />} />
        <Route path="/status" element={<Status />} />
        <Route path="/docs" element={<Docs />} />
        <Route path="/terms" element={<Legal kind="terms" />} />
        <Route path="/privacy" element={<Legal kind="privacy" />} />
        <Route path="*" element={<NotFound />} />
      </Route>

      <Route element={<Layout user />}>
        <Route
          path="/console"
          element={
            <RequireAuth>
              <Console />
            </RequireAuth>
          }
        />
        <Route
          path="/console/help"
          element={
            <RequireAuth>
              <ShortcutsHelp />
            </RequireAuth>
          }
        />
        <Route
          path="/media/:id/*"
          element={
            <RequireAuth>
              <MediaDetail />
            </RequireAuth>
          }
        />
        <Route
          path="/analytics"
          element={
            <RequireAuth>
              <Analytics />
            </RequireAuth>
          }
        />
        <Route
          path="/keys/*"
          element={
            <RequireAuth>
              <KeysPage />
            </RequireAuth>
          }
        />
        <Route
          path="/profile/*"
          element={
            <RequireAuth>
              <ProfilePage />
            </RequireAuth>
          }
        />
        <Route
          path="/admin"
          element={
            <RequireAuth>
              <Admin />
            </RequireAuth>
          }
        />
      </Route>
    </Routes>
    </Suspense>
  );
}

export default function App() {
  const statusOnly = isStatusOnly();
  return (
    <ErrorBoundary>
      <HelmetProvider>
        <I18nProvider>
          <ToastProvider>
            <AuthProvider>
              <BrowserRouter>{statusOnly ? <StatusOnlyRouter /> : <Router />}</BrowserRouter>
            </AuthProvider>
          </ToastProvider>
        </I18nProvider>
      </HelmetProvider>
    </ErrorBoundary>
  );
}
