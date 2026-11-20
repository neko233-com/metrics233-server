import { BrowserClock } from "./ui/BrowserClock";
import { PageRecoveryBoundary } from "./ui/PageRecoveryBoundary";
import { t as tr } from "./i18n";
import { getLocale, setLocale, type Locale } from "./i18n";
import { useLocation, useNavigate, useNavigationType } from "react-router";
import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import {
  Activity,
  Bell,
  ChartNoAxesColumn,
  CircleCheck,
  CircleAlert,
  Info,
  TriangleAlert,
  House,
  LogOut,
  Menu,
  Monitor,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Puzzle,
  Search,
  Server,
  Terminal,
  UsersRound,
  X,
} from "lucide-react";
import {
  api,
  authLogin,
  authLogout,
  authSetup,
  authStatus,
  dashboardUID,
  ApiError,
  message,
  type AuthUser,
  type Dashboard,
  type Metric,
  type Stats,
  type Target,
} from "./api";
import { Alert, Button, Field, Input } from "./ui";
import { SessionContext } from "./ui/session";
import {
  connectAppEvents,
  refreshWorkspace,
  updateWorkspaceTimeRange,
  type NoticeSeverity,
} from "./grafana/app-events";
import {
  rangeFromURL,
  rangeFromDashboard,
  resolveTimeRange,
  writeRangeURL,
  type TimeSelection,
  refreshFromDashboard,
  refreshOptionsFromDashboard,
  refreshMilliseconds,
} from "./grafana/time-range";
const AppPluginView = lazy(() => import("./grafana/AppPluginView"));
const ExtensionHost = lazy(() => import("./grafana/ExtensionHost"));
const BusinessMonitoring = lazy(() => import("./features/BusinessMonitoring"));
const Users = lazy(() => import("./features/Users").then((module) => ({ default: module.Users })));
const DashboardView = lazy(() => import("./features/DashboardView").then((module) => ({ default: module.DashboardView })));
const Dashboards = lazy(() => import("./features/Dashboards").then((module) => ({ default: module.Dashboards })));
const Explore = lazy(() => import("./features/Explore").then((module) => ({ default: module.Explore })));
const Collectors = lazy(() => import("./features/Collectors").then((module) => ({ default: module.Collectors })));
const ReportingSources = lazy(() => import("./features/ReportingSources"));
const ManagedHosts = lazy(() => import("./features/ManagedHosts"));
const AgentCLI = lazy(() => import("./features/AgentCLI").then((module) => ({ default: module.AgentCLI })));
const TemplateView = lazy(() => import("./features/TemplateView").then((module) => ({ default: module.TemplateView })));
const Alerts = lazy(() => import("./features/Alerts").then((module) => ({ default: module.Alerts })));
const Plugins = lazy(() => import("./features/Plugins").then((module) => ({ default: module.Plugins })));
type View =
  | "overview"
  | "dashboards"
  | "dashboard"
  | "explore"
  | "collectors"
  | "reporting-sources"
  | "hosts"
  | "business"
  | "alerts"
  | "plugins"
  | "users"
  | "cli";
type AppView = View | "app";
const baseNavigation = [
  { id: "overview", name: "Home", icon: House },
  { id: "business", name: "Servers", icon: Server },
  { id: "dashboards", name: "Saved views", icon: ChartNoAxesColumn },
  { id: "explore", name: "Search details", icon: Search },
  { id: "collectors", name: "Data connections", icon: Network },
  { id: "reporting-sources", name: "Reporting access", icon: Activity },
  { id: "hosts", name: "Hosts", icon: Monitor },
  { id: "alerts", name: "Alerts", icon: Bell },
  { id: "cli", name: "CLI guide", icon: Terminal },
] as const;
const navigationGroups = [
  { name: "Monitoring", items: ["overview", "business", "hosts", "dashboards", "alerts"] },
  { name: "Data", items: ["explore", "collectors", "reporting-sources"] },
  { name: "Settings", items: ["cli", "users", "plugins"], showExtensions: true },
] satisfies { name: string; items: View[]; showExtensions?: boolean }[];

function routeResourceID(pathname: string, prefix: "d" | "a"): string {
  const [root, id] = pathname.split("/").slice(1);
  if (root !== prefix || !id) return "";
  try { return decodeURIComponent(id); } catch { return ""; }
}

function workspaceView(pathname: string, hash: string): AppView {
  if (/^\/a\/[^/]+/.test(pathname)) return "app";
  if (/^\/d\/[^/]+/.test(pathname)) return "dashboard";
  const id = hash.slice(1);
  // These administrative routes remain reachable directly.
  if (id === "plugins" || id === "users") return id;
  return baseNavigation.find(item => item.id === id)?.id || "overview";
}

export default function App() {
  const { pathname, hash, state: routeState } = useLocation();
  const navigationType = useNavigationType();
  useLayoutEffect(() => {
    if (navigationType !== "POP") window.scrollTo(0, 0);
  }, [pathname, hash, navigationType]);
  const navigateURL = useNavigate();
  const view = workspaceView(pathname, hash);
  const currentUID = routeResourceID(pathname, "d");
  const currentAppID = routeResourceID(pathname, "a");
  const routeLoaded = useRef(false);
  const [initialRange] = useState(() => {
    try {
      return { value: rangeFromURL() || "1h", error: "" };
    } catch (error) {
      return { value: "1h", error: message(error) };
    }
  });
  const [rangeError, setRangeError] = useState(initialRange.error);
  const [refreshOverride, setRefreshOverride] = useState<string | null>(() =>
    new URLSearchParams(location.search).get("refresh"),
  );
  const [refreshError, setRefreshError] = useState("");
  const [refreshWidth, setRefreshWidth] = useState(window.innerWidth);
  const [locale, updateLocale] = useState<Locale>(getLocale());
  const languageButton = (
    <button
      className="language-button"
      aria-label="Switch language"
      onClick={() => {
        const next = locale === "en" ? "zh" : "en";
        setLocale(next);
        updateLocale(next);
      }}
    >
      {locale === "en" ? "中文" : "English"}
    </button>
  );
  const [sidebarOpen, setSidebarOpen] = useState(false),
    [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
      try {
        return localStorage.getItem("metricspanel-sidebar") === "collapsed";
      } catch {
        return false;
      }
    });
  const [dashboards, setDashboards] = useState<Dashboard[]>([]),
    [apps, setApps] = useState<
      {
        id: string;
        name: string;
        pinned?: boolean;
        enabled?: boolean;
        type: string;
      }[]
    >([]),
    [targets, setTargets] = useState<Target[]>([]),
    [metrics, setMetrics] = useState<Metric[]>([]),
    [stats, setStats] = useState<Stats | null>(null);
  const [hasExtensions, setHasExtensions] = useState(false);
  const [tick, setTick] = useState(0),
    [range, setRange] = useState<TimeSelection>(initialRange.value),
    [auth, setAuth] = useState(false),
    [authMode, setAuthMode] = useState<"unknown" | "local" | "token">(
      "unknown",
    ),
    [setupRequired, setSetupRequired] = useState(false),
    [user, setUser] = useState<AuthUser | null>(null),
    [token, setToken] = useState(""),
    [credentials, setCredentials] = useState({ login: "", password: "" }),
    [setup, setSetup] = useState({
      token: "",
      login: "",
      name: "",
      email: "",
      password: "",
      confirm: "",
    }),
    [authBusy, setAuthBusy] = useState(false),
    [connectionError, setConnectionError] = useState(""),
    [notice, setNotice] = useState<{
      text: string;
      severity: NoticeSeverity;
    } | null>(null);
  const navigation = user?.isServerAdmin
    ? [
        ...baseNavigation,
        {
          id: "users" as const,
          name: "Accounts",
          icon: UsersRound,
        },
      ]
    : baseNavigation;
  const notify = useCallback(
    (text: string, error = false) =>
      setNotice({ text, severity: error ? "error" : "success" }),
    [],
  );
  const current = view === "overview" ? dashboards.find(dashboard => dashboard.id === "system")
    : currentUID ? dashboards.find(dashboard => dashboardUID(dashboard) === currentUID) : undefined;
  const dashboardRouteMissing = view === "dashboard" && !current && dashboards.length > 0;
  const dashboardNavigation = routeState?.fromSavedViews === true ? "dashboards"
    : (!user || user.isServerAdmin) && /^(host|mysql)-[a-f0-9]{32}$/.test(current?.id || "")
    ? "hosts"
    : /^conn-[a-f0-9]{32}$/.test(current?.id || "") ? "business" : "dashboards";
  const refreshChoice =
    refreshOverride ??
    ((view === "dashboard" || view === "overview") && current?.grafana
      ? refreshFromDashboard(current.grafana)
      : "5s");
  const refreshResolution = refreshChoice === "auto" ? refreshWidth : 0;
  const changeRefresh = useCallback(
    (next: string) => {
      try {
        refreshMilliseconds(next, range);
        setRefreshOverride(next);
        setRefreshError("");
        const params = new URLSearchParams(location.search);
        params.set("refresh", next);
        history.replaceState(
          history.state,
          "",
          `${location.pathname}?${params}${location.hash}`,
        );
      } catch (error) {
        notify(tr(message(error)), true);
      }
    },
    [range, notify],
  );
  const load = useCallback(async () => {
    try {
      const [ds, ts, ms, st, ps] = await Promise.all([
        api<Dashboard[]>("/dashboards"),
        api<Target[]>("/targets"),
        api<Metric[]>("/metrics"),
        api<Stats>("/stats"),
        api<
          {
            id: string;
            name: string;
            pinned?: boolean;
            enabled?: boolean;
            type: string;
            extensions?: {
              addedLinks?: unknown[];
              addedComponents?: unknown[];
              addedFunctions?: unknown[];
              exposedComponents?: unknown[];
            };
          }[]
        >("/api/plugins"),
      ]);
      setDashboards(ds);
      setTargets(ts);
      setMetrics(ms);
      setStats(st);
      setHasExtensions(
        ps.some(
          (plugin) =>
            plugin.type === "app" &&
            plugin.enabled &&
            Object.values(plugin.extensions || {}).some(
              (entries) => entries?.length,
            ),
        ),
      );
      setApps(
        ps.filter(
          (plugin) => plugin.type === "app" && plugin.enabled && plugin.pinned,
        ),
      );
      setTick((t) => t + 1);
      setConnectionError("");
      setAuth(false);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        setAuth(true);
        setConnectionError(
          sessionStorage.getItem("metricspanel-token")
            ? "Invalid API token."
            : "",
        );
      } else setConnectionError(message(e));
    }
  }, []);
  useEffect(
    () =>
      connectAppEvents({
        refresh: load,
        notify: (text, severity) => setNotice({ text, severity }),
      }),
    [load],
  );
  const changeRange = useCallback(
    (next: TimeSelection) => {
      try {
        resolveTimeRange(next);
        setRange(next);
        setRangeError("");
        writeRangeURL(next);
        updateWorkspaceTimeRange(next);
      } catch (error) {
        notify(tr(message(error)), true);
      }
    },
    [notify],
  );
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const status = await authStatus();
        if (cancelled) return;
        if (!status || status.mode === "token") {
          setAuthMode("token");
          await load();
          return;
        }
        setAuthMode("local");
        setSetupRequired(status.setupRequired);
        if (status.authenticated && status.user) {
          sessionStorage.removeItem("metricspanel-token");
          setUser(status.user);
          setAuth(false);
          await load();
        } else {
          setAuth(true);
        }
      } catch (error) {
        if (!cancelled) {
          setAuthMode("token");
          setConnectionError(message(error));
          await load();
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [load]);
  useEffect(() => {
    const resized = () => setRefreshWidth(window.innerWidth);
    window.addEventListener("resize", resized);
    return () => window.removeEventListener("resize", resized);
  }, []);
  useEffect(() => {
    let interval: number;
    try {
      interval = refreshMilliseconds(
        refreshChoice,
        range,
        refreshResolution || window.innerWidth,
      );
      setRefreshError("");
    } catch (error) {
      setRefreshError(message(error));
      return;
    }
    if (!interval) return;
    let timer: ReturnType<typeof setTimeout>,
      blocked = false, nextDeadline = 0;
    const schedule = (deadline = Date.now() + interval) => {
      nextDeadline = deadline;
      timer = setTimeout(
        () => {
          if (Date.now() < deadline) {
            schedule(deadline);
            return;
          }
          try {
            if (document.hidden) blocked = true;
            else refreshWorkspace();
          } finally { schedule(); }
        },
        Math.min(2147483647, Math.max(1, deadline - Date.now())),
      );
    };
    const resume = () => {
      if (!document.hidden && (blocked || Date.now() >= nextDeadline)) {
        blocked = false;
        clearTimeout(timer);
        try { refreshWorkspace(); } finally { schedule(); }
      }
    };
    schedule();
    document.addEventListener("visibilitychange", resume);
    window.addEventListener("pageshow", resume);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", resume);
      window.removeEventListener("pageshow", resume);
    };
  }, [refreshChoice, range, refreshResolution]);
  useEffect(() => {
    if (!notice) return;
    const timer = setTimeout(
      () => setNotice(null),
      notice.severity === "error" ? 8000 : 3500,
    );
    return () => clearTimeout(timer);
  }, [notice]);
  useEffect(() => {
    if (routeLoaded.current || !dashboards.length) return;
    routeLoaded.current = true;
    const uid = routeResourceID(location.pathname, "d");
    const dashboard = uid
      ? dashboards.find((d) => dashboardUID(d) === uid)
      : undefined;
    if (dashboard) {
      try {
        const next =
          rangeFromURL(
            new URLSearchParams(location.search),
            rangeFromDashboard(dashboard.grafana) || "1h",
          ) ||
          rangeFromDashboard(dashboard.grafana) ||
          "1h";
        setRange(next);
        setRangeError("");
      } catch (error) {
        setRangeError(message(error));
      }
    }
  }, [dashboards]);
  useEffect(() => {
    const restore = () => {
      setRefreshOverride(new URLSearchParams(location.search).get("refresh"));
      const uid = routeResourceID(location.pathname, "d");
      const dashboard = uid
        ? dashboards.find(
            (item) => dashboardUID(item) === uid,
          )
        : undefined;
      try {
        const fallback = rangeFromDashboard(dashboard?.grafana) || "1h";
        const next =
          rangeFromURL(new URLSearchParams(location.search), fallback) ||
          fallback;
        setRange(next);
        setRangeError("");
        updateWorkspaceTimeRange(next);
      } catch (error) {
        setRangeError(message(error));
      }
    };
    window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, [dashboards]);
  function navigate(next: Exclude<View, "dashboard">) {
    setSidebarOpen(false);
    const destination = "#" + next;
    const params = new URLSearchParams(location.search);
    const clearBusinessSelection = view === "business";
    if (clearBusinessSelection) {
      for (const key of ["surface", "instance", "section"]) params.delete(key);
    }
    if (location.pathname !== "/" || location.hash !== destination || params.toString() !== new URLSearchParams(location.search).toString()) {
      void navigateURL({ pathname: "/", search: params.toString(), hash: destination });
    }
  }
  function toggleSidebar() {
    setSidebarCollapsed((collapsed) => {
      const next = !collapsed;
      try {
        localStorage.setItem(
          "metricspanel-sidebar",
          next ? "collapsed" : "expanded",
        );
      } catch {
        /* storage can be unavailable in private browsing */
      }
      return next;
    });
  }
  function openDashboard(d: Dashboard) {
    setRefreshOverride(null);
    // Carry an explicit URL/user time choice between dashboards, without
    // carrying another dashboard's scoped variable values.
    const params = new URLSearchParams();
    const currentParams = new URLSearchParams(location.search);
    for (const key of ["from", "to", "time", "time.window", "timezone"])
      if (currentParams.has(key)) params.set(key, currentParams.get(key)!);
    const destination = `/d/${encodeURIComponent(dashboardUID(d))}/${encodeURIComponent(d.name)}`;
    void navigateURL({ pathname: destination, search: params.toString() }, {
      replace: location.pathname === destination,
      state: { fromSavedViews: view === "dashboards" || routeState?.fromSavedViews === true },
    });
    try {
      const fallback = rangeFromDashboard(d.grafana) || "1h";
      const next = rangeFromURL(params, fallback) || fallback;
      setRange(next);
      setRangeError("");
      updateWorkspaceTimeRange(next);
    } catch (error) {
      setRangeError(message(error));
    }
    setSidebarOpen(false);
  }
  const title =
    view === "app"
      ? "Application"
      : view === "dashboard"
        ? current?.name || "Dashboard"
        : navigation.find((n) => n.id === view)?.name;
  async function finishLogin() {
    const status = await authStatus();
    if (status?.authenticated && status.user) {
      sessionStorage.removeItem("metricspanel-token");
      setUser(status.user);
      setSetupRequired(status.setupRequired);
      setAuth(false);
      setConnectionError("");
      await load();
    }
  }
  async function submitLogin(e: FormEvent) {
    e.preventDefault();
    setAuthBusy(true);
    setConnectionError("");
    try {
      await authLogin(credentials.login, credentials.password);
      await finishLogin();
    } catch (error) {
      setConnectionError(message(error));
    } finally {
      setAuthBusy(false);
    }
  }
  async function submitSetup(e: FormEvent) {
    e.preventDefault();
    setConnectionError("");
    if (setup.password !== setup.confirm) {
      setConnectionError(tr("Passwords do not match."));
      return;
    }
    if (setup.password.length < 12) {
      setConnectionError(tr("Use at least 12 characters for the password."));
      return;
    }
    setAuthBusy(true);
    try {
      await authSetup({
        token: setup.token.trim(),
        login: setup.login,
        name: setup.name,
        email: setup.email,
        password: setup.password,
      });
      await finishLogin();
    } catch (error) {
      setConnectionError(message(error));
    } finally {
      setAuthBusy(false);
    }
  }
  async function disconnect() {
    if (authMode === "local") {
      await authLogout().catch(() => {});
      setUser(null);
      setAuth(true);
    } else {
      sessionStorage.removeItem("metricspanel-token");
      setAuth(true);
    }
  }
  if (authMode === "unknown")
    return (
      <main className="login" role="status" aria-live="polite">
        <Activity size={42} />
        <h1>{"Metrics233"}</h1>
        <p>{tr("Connecting to your workspace…")}</p>
      </main>
    );
  if (auth && authMode === "local" && setupRequired)
    return (
      <main className="login">
        {languageButton}
        <Activity size={42} />
        <h1>{tr("Set up MetricsPanel233")}</h1>
        <p>
          {tr(
            "Enter the setup token printed by the server, then create the administrator account.",
          )}
        </p>
        <form onSubmit={submitSetup}>
          <Field label={tr("Setup token")} required>
            <Input
              autoFocus
              aria-label={tr("Setup token")}
              value={setup.token}
              onChange={(e) => setSetup({ ...setup, token: e.target.value })}
              required
              autoComplete="off"
            />
          </Field>
          <Field label={tr("Login")} required>
            <Input
              aria-label={tr("Login")}
              value={setup.login}
              onChange={(e) => setSetup({ ...setup, login: e.target.value })}
              required
              autoComplete="username"
            />
          </Field>
          <Field label={tr("Name")} required>
            <Input
              aria-label={tr("Name")}
              value={setup.name}
              onChange={(e) => setSetup({ ...setup, name: e.target.value })}
              required
              autoComplete="name"
            />
          </Field>
          <Field label={tr("Email")}>
            <Input
              type="email"
              aria-label={tr("Email")}
              value={setup.email}
              onChange={(e) => setSetup({ ...setup, email: e.target.value })}
              autoComplete="email"
            />
          </Field>
          <Field label={tr("Password")} required>
            <Input
              type="password"
              aria-label={tr("Password")}
              value={setup.password}
              onChange={(e) =>
                setSetup({ ...setup, password: e.target.value })
              }
              required
              minLength={12}
              autoComplete="new-password"
            />
          </Field>
          <Field label={tr("Confirm password")} required>
            <Input
              type="password"
              aria-label={tr("Confirm password")}
              value={setup.confirm}
              onChange={(e) => setSetup({ ...setup, confirm: e.target.value })}
              required
              autoComplete="new-password"
            />
          </Field>
          {connectionError && (
            <Alert severity="error" title={tr(connectionError)} />
          )}
          <Button type="submit" variant="primary" disabled={authBusy}>
            {tr("Create administrator")}
          </Button>
        </form>
        <small>
          {tr(
            "The setup token is shown once in the server console and disappears after initialization.",
          )}
        </small>
      </main>
    );
  if (auth && authMode === "local")
    return (
      <main className="login">
        {languageButton}
        <Activity size={42} />
        <h1>{"Metrics233"}</h1>
        <p>{tr("Sign in to your workspace.")}</p>
        <form onSubmit={submitLogin}>
          <Field label={tr("Login")} required>
            <Input
              autoFocus
              aria-label={tr("Login")}
              value={credentials.login}
              onChange={(e) =>
                setCredentials({ ...credentials, login: e.target.value })
              }
              required
              autoComplete="username"
            />
          </Field>
          <Field label={tr("Password")} required>
            <Input
              type="password"
              aria-label={tr("Password")}
              value={credentials.password}
              onChange={(e) =>
                setCredentials({ ...credentials, password: e.target.value })
              }
              required
              autoComplete="current-password"
            />
          </Field>
          {connectionError && (
            <Alert severity="error" title={tr(connectionError)} />
          )}
          <Button type="submit" variant="primary" disabled={authBusy}>
            {tr("Sign in")}
          </Button>
        </form>
        <small>
          {tr("Sessions are stored in an HttpOnly same-site cookie.")}
        </small>
      </main>
    );
  if (auth)
    return (
      <main className="login">
        {languageButton}
        <Activity size={42} />
        <h1>{"Metrics233"}</h1>
        <p>{tr("Enter your workspace API token.")}</p>
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            sessionStorage.setItem("metricspanel-token", token);
            await load();
          }}
        >
          <Field label={tr("API token")} required>
            <Input
              autoFocus
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              required
              autoComplete="current-password"
            />
          </Field>
          {connectionError && (
            <Alert severity="error" title={tr(connectionError)} />
          )}
          <Button type="submit" variant="primary">
            {tr("Connect workspace")}
          </Button>
        </form>
        <small>{tr("The token is kept in this browser tab's session.")}</small>
      </main>
    );
  return (
    <SessionContext.Provider
      value={{
        user,
        canEdit: !user || user.role === "Editor" || user.role === "Admin",
        canAdmin: !user || user.isServerAdmin,
      }}
    >
      <div
        className={`app-shell ${sidebarCollapsed ? "sidebar-collapsed" : ""}`}
      >
        <a className="skip-to-content" href="#workspace-content" onClick={event => {
          event.preventDefault();
          document.getElementById("workspace-content")?.focus();
        }}>{tr("Skip to content")}</a>
        {sidebarOpen && (
        <button
          className="sidebar-overlay"
          onClick={() => setSidebarOpen(false)}
          aria-label={tr("Close navigation")}
        />
      )}
      <aside className={`sidebar ${sidebarOpen ? "open" : ""}`}>
        <a
          href="#overview"
          onClick={(e) => {
            e.preventDefault();
            navigate("overview");
          }}
          className="brand"
          aria-label={"Metrics233"}
          title={"Metrics233"}
        >
          <span className="brand-mark"><Activity size={20} strokeWidth={2} aria-hidden="true" /></span>
          <span>{"Metrics233"}</span>
        </a>
        <nav aria-label={tr("Main navigation")}>
          {navigationGroups.map(group => <section className="nav-group" key={group.name} aria-label={tr(group.name)}>
            <h2 className="nav-group-title">{tr(group.name)}</h2>
          {group.items.flatMap(id => navigation.filter(item => item.id === id)).map((item) => (
            <button
              key={item.id}
              aria-label={tr(item.name)}
              title={tr(item.name)}
              aria-current={view === item.id || (view === "dashboard" && item.id === dashboardNavigation) ? "page" : undefined}
              className={
                view === item.id ||
                (view === "dashboard" && item.id === dashboardNavigation)
                  ? "active"
                  : ""
              }
              onClick={() => navigate(item.id)}
            >
              <item.icon size={17} strokeWidth={1.8} aria-hidden="true" />
              <span>{tr(item.name)}</span>
            </button>
          ))}
          {"showExtensions" in group && group.showExtensions && apps.map((app) => (
            <a
              className="app-nav-link"
              key={app.id}
              href={`/a/${encodeURIComponent(app.id)}/`}
              aria-label={app.name}
              title={app.name}
              aria-current={
                view === "app" && currentAppID === app.id
                  ? "page"
                  : undefined
              }
            >
              <Puzzle size={17} aria-hidden="true" />
              <span>{app.name}</span>
            </a>
          ))}
          </section>)}
        </nav>
        <div className="workspace">
          <Monitor size={22} />
          <div>
            <span>{user ? user.name : tr("Local workspace")}</span>
            <small className="mono">
              {user
                ? `${user.role}${user.email ? ` \u00B7 ${user.email}` : ""}`
                : tr("SQLite \u00B7 self-hosted")}
            </small>
          </div>
          {(user || sessionStorage.getItem("metricspanel-token")) && (
            <button
              className="icon-button"
              title={tr("Sign out")}
              aria-label={tr("Sign out")}
              onClick={() => void disconnect()}
            >
              <LogOut size={17} />
            </button>
          )}
        </div>
      </aside>
      <div className="workspace-main">
        {hasExtensions && (
          <Suspense fallback={null}>
            <ExtensionHost />
          </Suspense>
        )}
        <header className="topbar">
          <div>
            <button
              className="icon-button sidebar-toggle"
              onClick={toggleSidebar}
              aria-label={
                sidebarCollapsed
                  ? tr("Expand navigation")
                  : tr("Collapse navigation")
              }
              aria-expanded={!sidebarCollapsed}
              title={
                sidebarCollapsed
                  ? tr("Expand navigation")
                  : tr("Collapse navigation")
              }
            >
              {sidebarCollapsed ? (
                <PanelLeftOpen size={20} />
              ) : (
                <PanelLeftClose size={20} />
              )}
            </button>
            <button
              className="icon-button mobile-menu"
              onClick={() => setSidebarOpen(true)}
              aria-label={tr("Open navigation")}
            >
              <Menu size={22} />
            </button>
            <span>{tr("Workspace")}</span>
            <span className="breadcrumb-slash">/</span>
            <span>{tr(title || "Dashboard")}</span>
          </div>
          <div className="topbar-status">
            {languageButton}
            <BrowserClock />
          </div>
        </header>
        <main id="workspace-content" tabIndex={-1}>
          <PageRecoveryBoundary key={view}>
          <Suspense fallback={<p role="status">{tr("Loading…")}</p>}>
          {rangeError && (
            <p className="form-error" role="alert">
              {tr(rangeError)}
            </p>
          )}
          {refreshError && (
            <p className="form-error" role="alert">
              {tr(refreshError)}
            </p>
          )}
          {connectionError && (
            <div className="connection-error" role="alert">
              <span>
                {tr("Connection failed:")}
                {connectionError}
              </span>
              <button onClick={() => void load()}>{tr("Retry")}</button>
            </div>
          )}
          {(view === "overview" || view === "dashboard") &&
            (current ? (
              current.grafana ? (
                <TemplateView
                  key={current.id}
                  dashboard={current}
                  range={range}
                  onRange={changeRange}
                  tick={tick}
                  refresh={refreshWorkspace}
                  reload={load}
                  refreshChoice={refreshChoice}
                  refreshOptions={refreshOptionsFromDashboard(current.grafana)}
                  onRefreshChoice={changeRefresh}
                />
              ) : (
                <DashboardView
                  dashboard={current}
                  stats={stats}
                  targets={targets}
                  metrics={metrics}
                  tick={tick}
                  range={range}
                  onRange={changeRange}
                  refresh={refreshWorkspace}
                  refreshChoice={refreshChoice}
                  onRefreshChoice={changeRefresh}
                  reload={load}
                  notify={notify}
                  onNavigate={navigate}
                />
              )
            ) : (
              <div
                className="loading-state"
                role={dashboardRouteMissing ? "alert" : "status"}
              >
                <Activity size={28} />
                <p>
                  {tr(
                    dashboardRouteMissing
                      ? "Dashboard not found. Open the dashboard list to choose another dashboard."
                      : connectionError
                        ? "Start metricspanel serve to connect this workspace."
                        : "Connecting to your workspace…",
                  )}
                </p>
              </div>
            ))}
          {view === "dashboards" && (
            <Dashboards
              tick={tick}
              dashboards={dashboards.filter(dashboard=>!/^conn-[a-f0-9]{32}$/.test(dashboard.id))}
              reload={load}
              notify={notify}
              open={openDashboard}
            />
          )}
          {view === "business" && <Suspense fallback={<p role="status">{tr("Loading…")}</p>}><BusinessMonitoring metrics={metrics} dashboards={dashboards} targets={targets} reload={load} open={openDashboard} navigate={navigate} tick={tick} /></Suspense>}
          {view === "explore" && <Explore metrics={metrics} tick={tick} />}
          {view === "collectors" && (
            <Collectors targets={targets} reload={load} notify={notify} open={openDashboard} />
          )}
          {view === "reporting-sources" && <ReportingSources notify={notify} />}
          {view === "hosts" && <ManagedHosts notify={notify} dashboards={dashboards} open={openDashboard} reloadDashboards={load} />}
          {view === "cli" && <AgentCLI />}
          {view === "plugins" && <Plugins notify={notify} />}
          {view === "app" && (
            <Suspense fallback={<p>{tr("Loading plugin…")}</p>}>
              <AppPluginView
                id={currentAppID}
                tick={tick}
              />
            </Suspense>
          )}
          {view === "alerts" && (
            <Alerts tick={tick} metrics={metrics} notify={notify} />
          )}
          {view === "users" && user?.isServerAdmin && (
            <Users notify={notify} currentUser={user} />
          )}
          </Suspense>
          </PageRecoveryBoundary>
        </main>
      </div>
      {notice && (
        <div
          className={`toast ${notice.severity}`}
          role={
            notice.severity === "error" || notice.severity === "warning"
              ? "alert"
              : "status"
          }
        >
          {notice.severity === "error" ? (
            <CircleAlert size={18} />
          ) : notice.severity === "warning" ? (
            <TriangleAlert size={18} />
          ) : notice.severity === "info" ? (
            <Info size={18} />
          ) : (
            <CircleCheck size={18} />
          )}
          <span>{notice.text}</span>
          <button
            className="icon-button"
            onClick={() => setNotice(null)}
            aria-label={tr("Dismiss notification")}
          >
            <X size={17} />
          </button>
        </div>
      )}
      </div>
    </SessionContext.Provider>
  );
}
