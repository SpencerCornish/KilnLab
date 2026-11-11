import { BrowserRouter, Navigate, Route, Routes } from 'react-router'
import Layout from './components/Layout'
import { RequireLogin, SessionProvider } from './lib/session'
import { AdminIndexPage, AdminUsersPage } from './pages/AdminPages'
import { AdminQRPage, AdminSettingsPage } from './pages/AdminSettingsPage'
import FiringPage from './pages/FiringPage'
import { KilnFiringPage, KilnPage } from './pages/KilnPage'
import HomePage from './pages/HomePage'
import LoginPage from './pages/LoginPage'
import NewFiringPage from './pages/NewFiringPage'
import UpcomingPage from './pages/UpcomingPage'

// Keep in sync with isSPARoute in internal/api/spa.go.
export default function App() {
  return (
    <BrowserRouter>
      <SessionProvider>
        <Routes>
          <Route element={<Layout />}>
            {/* Public */}
            <Route path="/login" element={<LoginPage />} />
            <Route path="/upcoming" element={<UpcomingPage />} />

            {/* Signed in */}
            <Route path="/" element={<RequireLogin><HomePage /></RequireLogin>} />
            <Route path="/kiln" element={<RequireLogin><KilnPage /></RequireLogin>} />
            <Route path="/kiln/:id" element={<RequireLogin><KilnFiringPage /></RequireLogin>} />
            <Route path="/firings/new" element={<RequireLogin><NewFiringPage /></RequireLogin>} />
            <Route path="/firings/:id" element={<RequireLogin><FiringPage /></RequireLogin>} />
            <Route path="/firings" element={<Navigate to="/" replace />} />
            <Route path="/admin" element={<RequireLogin><AdminIndexPage /></RequireLogin>} />
            <Route path="/admin/users" element={<RequireLogin><AdminUsersPage /></RequireLogin>} />
            <Route path="/admin/settings" element={<RequireLogin><AdminSettingsPage /></RequireLogin>} />
            <Route path="/admin/qr" element={<RequireLogin><AdminQRPage /></RequireLogin>} />
            <Route path="/admin/*" element={<Navigate to="/admin" replace />} />

            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </SessionProvider>
    </BrowserRouter>
  )
}
