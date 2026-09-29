import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { lazy, Suspense, useEffect } from 'react'
import {
  createBrowserRouter,
  createRoutesFromElements,
  Navigate,
  Outlet,
  Route,
  RouterProvider,
  useLocation,
  useParams,
  useRouteError,
} from 'react-router-dom'

import { ApiError } from './api/client'
import { AuthProvider } from './auth/AuthContext'
import { useAuth } from './auth/useAuth'
import { isObjectType } from './guest/store'
import { errorKind } from './layout/errorKind'
import { ErrorPage } from './layout/ErrorPage'
import { Layout } from './layout/Layout'
import { reloadForStaleChunk } from './offline/staleChunk'
import { pageMoved } from './offline/serviceWorker'
import { AdminAudit } from './pages/admin/AdminAudit'
import { AdminCatalog } from './pages/admin/AdminCatalog'
import { AdminInvitations } from './pages/admin/AdminInvitations'
import { AdminLayout } from './pages/admin/AdminLayout'
import { Calc } from './pages/Calc'
import { Export } from './pages/Export'
import { Login } from './pages/Login'
import { ObjectPage } from './pages/ObjectPage'
import { PasswordReset } from './pages/PasswordReset'
import { Register } from './pages/Register'
import { Robots } from './pages/Robots'
import { Runs } from './pages/Runs'
import { RunView } from './pages/RunView'
import { Start } from './pages/Start'
import { Trash } from './pages/Trash'
import { VerifyEmail } from './pages/VerifyEmail'

const Sim = lazy(() => import('./pages/Sim').then((m) => ({ default: m.Sim })))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (count, err) => {
        if (err instanceof ApiError && err.status < 500) {
          return false
        }
        return count < 3
      },
    },
  },
})

function Loading() {
  return <p>Загрузка...</p>
}

// RouteError stands in for a page that threw while loading or drawing. Inside the shell it replaces the page;
// standalone, it replaces the shell that failed.
function RouteError({ standalone }: { standalone?: boolean }) {
  const err = useRouteError()
  const auth = useAuth()
  const info = errorKind(err, { guest: !auth.user })
  // A page whose code chunk is gone reloads once; the error page is what a second failure shows.
  useEffect(() => {
    if (info.kind === 'updated') {
      reloadForStaleChunk()
    }
  }, [info.kind])
  return <ErrorPage {...info} standalone={standalone} />
}

function NotFound() {
  return <ErrorPage kind="notFound" status={404} />
}

// DemoRoute opens only the three demo types; any other /demo/<name> is an unknown address.
function DemoRoute() {
  const { demo } = useParams()
  return isObjectType(demo ?? null) ? <Outlet /> : <NotFound />
}

// OldAddress sends a link from before the tabs (/econ/<id>, /object?type=airport) to the same place now.
function OldAddress({ to }: { to: string }) {
  const { projectId, runId } = useParams()
  const loc = useLocation()
  const type = new URLSearchParams(loc.search).get('type')
  if (projectId) {
    return <Navigate to={`/p/${projectId}/${to}${runId ? `/${runId}` : ''}`} replace />
  }
  return <Navigate to={isObjectType(type) ? `/demo/${type}/${to}` : '/'} replace />
}

// OldResults sends a link to the former Результаты tab to the same subtab of Расчёт.
function OldResults({ sub }: { sub: string }) {
  const { runId } = useParams()
  return <Navigate to={`../calc/${sub}${runId ? `/${runId}` : ''}`} replace />
}

function OldSim() {
  const loc = useLocation()
  return <Navigate to={`../calc/sim${loc.search}`} replace />
}

const oldSteps: [string, string][] = [
  ['object', 'object'],
  ['params', 'object'],
  ['processes', 'object/processes'],
  ['map', 'object/map'],
  ['variants', 'robots'],
  ['match', 'robots'],
  ['compare', 'calc'],
  ['econ', 'calc'],
  ['sim', 'calc/sim'],
  ['export', 'calc/export'],
  ['runs', 'calc/history'],
]

// adminRoutes serve Админ in a project and outside one: the same pages under /p/<id>/admin and /admin.
function adminRoutes() {
  return (
    <Route path="admin" element={<AdminLayout />}>
      <Route index element={<Navigate to="catalog" replace />} />
      <Route path="catalog" element={<AdminCatalog />} />
      <Route path="invitations" element={<AdminInvitations />} />
      <Route path="audit" element={<AdminAudit />} />
      <Route path="*" element={<NotFound />} />
    </Route>
  )
}

// projectRoutes are the tabs of a project or a demo. A demo has no Админ: everyone there is a guest.
function projectRoutes(demo: boolean) {
  return (
    <>
      <Route index element={<Navigate to="object" replace />} />
      {demo ? null : adminRoutes()}
      <Route path="object" element={<ObjectPage />} />
      <Route path="object/:tab" element={<ObjectPage />} />
      <Route path="robots" element={<Robots />} />
      <Route path="calc" element={<Navigate to="summary" replace />} />
      <Route path="calc/export" element={<Export />} />
      <Route path="calc/history" element={<Runs />} />
      <Route path="calc/history/:runId" element={<RunView />} />
      <Route path="calc/sim" element={<Suspense fallback={<Loading />}><Sim /></Suspense>} />
      <Route path="calc/:tab" element={<Calc />} />
      <Route path="sim" element={<OldSim />} />
      <Route path="results" element={<OldResults sub="export" />} />
      <Route path="results/export" element={<OldResults sub="export" />} />
      <Route path="results/history" element={<OldResults sub="history" />} />
      <Route path="results/history/:runId" element={<OldResults sub="history" />} />
      <Route path="*" element={<NotFound />} />
    </>
  )
}

const router = createBrowserRouter(
  createRoutesFromElements(
    <Route element={<Layout />} errorElement={<RouteError standalone />}>
      <Route errorElement={<RouteError />}>
        <Route path="/" element={<Start />} />
        <Route path="/p/:projectId">{projectRoutes(false)}</Route>
        <Route path="/demo/:demo" element={<DemoRoute />}>
          {projectRoutes(true)}
        </Route>
        {oldSteps.map(([from, to]) => (
          <Route key={from} path={`/${from}`} element={<OldAddress to={to} />} />
        ))}
        {oldSteps.map(([from, to]) => (
          <Route key={`${from}-id`} path={`/${from}/:projectId`} element={<OldAddress to={to} />} />
        ))}
        <Route path="/runs/:projectId/:runId" element={<OldAddress to="calc/history" />} />
        <Route path="/projects" element={<Navigate to="/" replace />} />
        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />
        <Route path="/trash" element={<Trash />} />
        <Route path="/verify-email" element={<VerifyEmail />} />
        <Route path="/reset-password" element={<PasswordReset />} />
        {adminRoutes()}
        <Route path="*" element={<NotFound />} />
      </Route>
    </Route>,
  ),
)

let lastPath = window.location.pathname
router.subscribe((state) => {
  if (state.navigation.state === 'idle' && state.location.pathname !== lastPath) {
    lastPath = state.location.pathname
    pageMoved()
  }
})

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <RouterProvider router={router} />
      </AuthProvider>
    </QueryClientProvider>
  )
}
