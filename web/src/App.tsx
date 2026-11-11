import { BrowserRouter, Navigate, Route, Routes } from 'react-router'
import Layout from './components/Layout'
import { RequireLogin, SessionProvider } from './lib/session'
import HomePage from './pages/HomePage'
import LoginPage from './pages/LoginPage'
import Placeholder from './pages/Placeholder'

// Keep in sync with isSPARoute in internal/api/spa.go.
export default function App() {
  return (
    <BrowserRouter>
      <SessionProvider>
        <Routes>
          <Route element={<Layout />}>
            {/* Public */}
            <Route path="/login" element={<LoginPage />} />
            <Route path="/upcoming" element={<Placeholder title="Upcoming firings" milestone="M4" />} />

            {/* Signed in */}
            <Route path="/" element={<RequireLogin><HomePage /></RequireLogin>} />
            <Route path="/kiln" element={<RequireLogin><Placeholder title="Kiln check-in" milestone="M2" /></RequireLogin>} />
            <Route path="/firings/*" element={<RequireLogin><Placeholder title="Firings" milestone="M1" /></RequireLogin>} />
            <Route path="/admin/*" element={<RequireLogin><Placeholder title="Admin" milestone="M2–M4" /></RequireLogin>} />

            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </SessionProvider>
    </BrowserRouter>
  )
}
