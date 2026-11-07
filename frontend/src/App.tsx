import { lazy, Suspense, type ReactNode } from 'react'
import { Redirect, Route, Switch, useLocation, useSearch } from 'wouter'
import { useAuthState } from './api/queries'
import { AppShell } from './components/layout/AppShell'
import { Spinner } from './components/ui/Misc'
import { LoginView } from './views/auth/LoginView'
import { RegisterView } from './views/auth/RegisterView'
import { DashboardView } from './views/dashboard/DashboardView'
import { BootErrorView, ForbiddenView, NotFoundView } from './views/StateViews'

const OnboardingView = lazy(() => import('./views/onboarding/OnboardingView'))
const PlanView = lazy(() => import('./views/plan/PlanView'))
const HealthView = lazy(() => import('./views/health/HealthView'))
const ShoesView = lazy(() => import('./views/shoes/ShoesView'))
const CoachView = lazy(() => import('./views/coach/CoachView'))
const SettingsView = lazy(() => import('./views/settings/SettingsView'))
const AdminView = lazy(() => import('./views/admin/AdminView'))
const SharePublicView = lazy(() => import('./views/share/SharePublicView'))

/** 4.0 routes kept as redirects so bookmarks and query strings survive the 4.1 navigation. */
function Moved({ to }: { to: string }) {
  const search = useSearch()
  return <Redirect to={search ? `${to}?${search}` : to} replace />
}

const PUBLIC = new Set(['/login', '/register'])

function Lazy({ children }: { children: ReactNode }) {
  return (
    <Suspense
      fallback={
        <div className="grid min-h-[40vh] place-items-center">
          <Spinner label="Chargement…" />
        </div>
      }
    >
      {children}
    </Suspense>
  )
}

export default function App() {
  const [path] = useLocation()
  const search = useSearch()
  const shareMatch = /^\/share\/([^/?#]+)$/.exec(path)
  const auth = useAuthState(!shareMatch)

  if (shareMatch) {
    return (
      <Lazy>
        <SharePublicView token={decodeURIComponent(shareMatch[1])} />
      </Lazy>
    )
  }

  if (auth.isPending) {
    return (
      <AppShell me={null}>
        <div className="grid min-h-[60vh] place-items-center">
          <Spinner label="Connexion à Stride…" />
        </div>
      </AppShell>
    )
  }
  if (auth.isError || !auth.data) {
    return (
      <AppShell me={null}>
        <BootErrorView error={auth.error} onRetry={() => void auth.refetch()} />
      </AppShell>
    )
  }

  const state = auth.data
  const me = state.authenticated ? state.user : null

  if (!me) {
    if (!PUBLIC.has(path)) {
      const next = path !== '/' ? `?next=${encodeURIComponent(path + (search ? `?${search}` : ''))}` : ''
      return <Redirect to={`/login${next}`} replace />
    }
    return (
      <AppShell me={null}>
        {path === '/register' ? <RegisterView state={state} /> : <LoginView state={state} />}
      </AppShell>
    )
  }

  if (PUBLIC.has(path)) {
    const next = new URLSearchParams(search).get('next')
    return <Redirect to={!me.onboardingCompleted ? '/onboarding' : next && next.startsWith('/') && !next.startsWith('//') ? next : '/'} replace />
  }
  if (!me.onboardingCompleted && path !== '/onboarding') return <Redirect to="/onboarding" replace />

  if (path === '/onboarding') {
    return (
      <AppShell me={me} minimal>
        <Lazy>
          <OnboardingView me={me} />
        </Lazy>
      </AppShell>
    )
  }

  return (
    <AppShell me={me}>
      <Lazy>
        <Switch>
          <Route path="/">
            <DashboardView me={me} />
          </Route>
          <Route path="/activities/:id">
            {(params) => <DashboardView me={me} routeActivityId={Number(params.id) || null} />}
          </Route>
          <Route path="/plan">
            <PlanView />
          </Route>
          <Route path="/health">
            <HealthView />
          </Route>
          <Route path="/body">
            <Moved to="/health" />
          </Route>
          <Route path="/shoes">
            <ShoesView />
          </Route>
          <Route path="/coach">
            <CoachView />
          </Route>
          <Route path="/export">
            <Moved to="/coach" />
          </Route>
          <Route path="/settings">
            <SettingsView me={me} />
          </Route>
          <Route path="/admin">{me.role === 'admin' ? <AdminView me={me} /> : <ForbiddenView />}</Route>
          <Route>
            <NotFoundView />
          </Route>
        </Switch>
      </Lazy>
    </AppShell>
  )
}
