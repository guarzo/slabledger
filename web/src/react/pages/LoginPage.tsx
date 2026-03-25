import React from 'react';
import { Navigate, useLocation, useSearchParams } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { CardPriceCard } from '../ui';
import type { GradeKey, GradeData } from '../../types/pricing';
import '../../css/LoginPage.css';

const SHOWCASE_CARD = {
  name: 'Charizard ex',
  setName: 'Obsidian Flames',
  number: '223',
  imageUrl: 'https://assets.tcgdex.net/en/sv/sv3/223/high.webp',
};

const SHOWCASE_PRICES = { raw: 28.50, psa8: 65.00, psa9: 155.00, psa10: 850.00 };

const SHOWCASE_GRADE_DATA: Partial<Record<GradeKey, GradeData>> = {
  raw: { ebay: { price: 28.50, confidence: 'high', salesCount: 42, trend: 'stable', median: 27.00, min: 22.00, max: 35.00, avg7day: 29.10, volume7day: 8 }, estimate: null },
  psa8: { ebay: { price: 65.00, confidence: 'high', salesCount: 18, trend: 'up', median: 62.00, min: 55.00, max: 78.00, avg7day: 63.50, volume7day: 4 }, estimate: null },
  psa9: { ebay: { price: 155.00, confidence: 'high', salesCount: 24, trend: 'up', median: 150.00, min: 130.00, max: 185.00, avg7day: 148.00, volume7day: 5 }, estimate: null },
  psa10: { ebay: { price: 850.00, confidence: 'medium', salesCount: 6, trend: 'up', median: 825.00, min: 750.00, max: 950.00, avg7day: 820.00, volume7day: 2 }, estimate: null },
};

const LoginPage: React.FC = () => {
  const { user, loading, login } = useAuth();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const error = searchParams.get('error');

  if (loading) {
    return (
      <div className="login-container">
        <div className="login-loading">Loading...</div>
      </div>
    );
  }

  if (user) {
    const from = (location.state as { from?: { pathname: string } })?.from?.pathname || '/';
    return <Navigate to={from} replace />;
  }

  return (
    <div className="login-page">
      {/* Header */}
      <div className="login-header">
        <h1>SlabLedger</h1>
        <p>Graded Card Portfolio Tracker</p>
      </div>

      {/* Showcase Card */}
      <div className="login-showcase">
        <div className="showcase-card-wrapper">
          <CardPriceCard
            card={SHOWCASE_CARD}
            prices={SHOWCASE_PRICES}
            variant="featured"
            gradeData={SHOWCASE_GRADE_DATA}
          />
        </div>
        <p className="showcase-caption">Track your graded card investments</p>
      </div>

      {/* Login Button */}
      <div className="login-actions">
        {error && (
          <div className="login-error" role="alert">
            {getErrorMessage(error)}
          </div>
        )}

        <button
          onClick={login}
          className="google-login-button"
          aria-label="Sign in with Google"
        >
          <svg className="google-icon" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
            <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
            <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
            <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
          </svg>
          <span>Sign in with Google</span>
        </button>

        <div className="login-features">
          <div className="feature">
            <span className="feature-icon" role="img" aria-hidden="true">📊</span>
            <span className="feature-text">Campaign tracking</span>
          </div>
          <div className="feature">
            <span className="feature-icon" role="img" aria-hidden="true">💰</span>
            <span className="feature-text">P&L analytics</span>
          </div>
          <div className="feature">
            <span className="feature-icon" role="img" aria-hidden="true">🔍</span>
            <span className="feature-text">Price lookup</span>
          </div>
        </div>
      </div>
    </div>
  );
};

function getErrorMessage(error: string): string {
  switch (error) {
    case 'oauth_failed':
      return 'Authentication failed. Please try again.';
    case 'token_exchange_failed':
      return 'Failed to complete authentication. Please try again.';
    case 'user_info_failed':
      return 'Failed to retrieve user information. Please try again.';
    case 'session_expired':
      return 'Your session has expired. Please sign in again.';
    case 'not_authorized':
      return 'Your account is not authorized. Contact an administrator for access.';
    default:
      return 'An error occurred. Please try again.';
  }
}

export default LoginPage;
