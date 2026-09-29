import { Outlet } from 'react-router-dom'

import { useAuth } from '../../auth/useAuth'
import { ErrorPage } from '../../layout/ErrorPage'

// AdminLayout lets admins through to the admin subtabs; everyone else gets the page that says why not.
export function AdminLayout() {
  const auth = useAuth()
  if (!auth.user) {
    return <ErrorPage kind="signIn" status={401} />
  }
  if (auth.user.role !== 'admin') {
    return <ErrorPage kind="noAccess" status={403} />
  }
  return <Outlet />
}
