/**
 * Main App Component (React Router Version)
 *
 * Fully migrated React application using React Router for navigation.
 * All pages are now React components - no more vanilla JS pages.
 * Uses lazy loading for route-based code splitting.
 */
import { Suspense, lazy } from 'react';
import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom';
import { AuthProvider } from './contexts/AuthContext';
import { UserPreferencesProvider } from './contexts/UserPreferencesContext';
import { ToastProvider } from './contexts/ToastContext';
import Header from './components/Header';
import ProtectedRoute from './components/ProtectedRoute';
import AdminRoute from './components/AdminRoute';
import PokeballLoader from './PokeballLoader';
import PageTransition from './components/PageTransition';
import ErrorBoundary from './ErrorBoundary';
import QueryProvider from './providers/QueryProvider';

// Lazy load pages for route-based code splitting
const DashboardPage = lazy(() => import('./pages/DashboardPage'));
const LoginPage = lazy(() => import('./pages/LoginPage'));
const CampaignsPage = lazy(() => import('./pages/CampaignsPage'));
const CampaignDetailPage = lazy(() => import('./pages/CampaignDetailPage'));
const AdminPage = lazy(() => import('./pages/AdminPage'));
const GlobalInventoryPage = lazy(() => import('./pages/GlobalInventoryPage'));
const InsightsPage = lazy(() => import('./pages/InsightsPage'));
const ToolsPage = lazy(() => import('./pages/ToolsPage'));
const InvoicesPage = lazy(() => import('./pages/InvoicesPage'));
const LiquidationPage = lazy(() => import('./pages/LiquidationPage'));

function AppContent() {
  const location = useLocation();
  const isLoginPage = location.pathname === '/login';

  return (
      <ToastProvider>
        {!isLoginPage && <Header />}
        <main
          id="main-content"
          className="min-h-screen bg-bg-primary dark:bg-slate-900 py-8"

        >
          <Suspense fallback={
            <div className="flex items-center justify-center min-h-screen">
              <PokeballLoader />
            </div>
          }>
            <Routes>
              <Route path="/" element={
                <ProtectedRoute>
                  <PageTransition>
                    <DashboardPage />
                  </PageTransition>
                </ProtectedRoute>
              } />
              <Route path="/login" element={
                <PageTransition>
                  <LoginPage />
                </PageTransition>
              } />
              {/* Tools */}
              <Route path="/tools" element={
                <ProtectedRoute>
                  <PageTransition>
                    <ToolsPage />
                  </PageTransition>
                </ProtectedRoute>
              } />
              {/* Canonical redirects for legacy URLs */}
              <Route path="/watchlist" element={<Navigate to="/" replace />} />
              <Route path="/favorites" element={<Navigate to="/" replace />} />
              <Route path="/opportunities" element={<Navigate to="/" replace />} />
              <Route path="/pricing" element={<Navigate to="/" replace />} />
              <Route path="/suggestions" element={<Navigate to="/" replace />} />
              {/* Campaigns */}
              <Route path="/campaigns" element={
                <ProtectedRoute>
                  <PageTransition>
                    <CampaignsPage />
                  </PageTransition>
                </ProtectedRoute>
              } />
              <Route path="/campaigns/:id" element={
                <ProtectedRoute>
                  <PageTransition>
                    <CampaignDetailPage />
                  </PageTransition>
                </ProtectedRoute>
              } />
              {/* Global Inventory */}
              <Route path="/inventory" element={
                <ProtectedRoute>
                  <PageTransition>
                    <GlobalInventoryPage />
                  </PageTransition>
                </ProtectedRoute>
              } />
              {/* Liquidation */}
              <Route path="/liquidation" element={
                <ProtectedRoute>
                  <PageTransition>
                    <LiquidationPage />
                  </PageTransition>
                </ProtectedRoute>
              } />
              {/* Invoices */}
              <Route path="/invoices" element={
                <ProtectedRoute>
                  <PageTransition>
                    <InvoicesPage />
                  </PageTransition>
                </ProtectedRoute>
              } />
              {/* Insights (AI reports hub) */}
              <Route path="/insights" element={
                <ProtectedRoute>
                  <PageTransition>
                    <InsightsPage />
                  </PageTransition>
                </ProtectedRoute>
              } />
              {/* Admin page (accessible via header indicator for admins) */}
              <Route path="/admin" element={
                <AdminRoute>
                  <PageTransition>
                    <AdminPage />
                  </PageTransition>
                </AdminRoute>
               } />
               <Route path="*" element={<Navigate to="/login" replace />} />
            </Routes>
          </Suspense>
        </main>
      </ToastProvider>
  );
}

function AppWithErrorBoundary() {
  const location = useLocation();
  return (
    <ErrorBoundary key={location.pathname}>
      <QueryProvider>
        <AuthProvider>
          <UserPreferencesProvider>
            <AppContent />
          </UserPreferencesProvider>
        </AuthProvider>
      </QueryProvider>
    </ErrorBoundary>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AppWithErrorBoundary />
    </BrowserRouter>
  );
}
