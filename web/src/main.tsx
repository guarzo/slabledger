/**
 * Main Entry Point - React Application
 *
 * This is the new React-based entry point that replaces js/main.js
 * Sets up context providers and initializes the app
 */
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

// CSS imports - Tailwind + minimal base styles
import './css/base.css';

// Context providers
import { UserPreferencesProvider } from './react/contexts/UserPreferencesContext';
// App component
import App from './react/App';

// Global error handler setup
import { setupGlobalErrorHandler } from './js/errors';

// Development tools
if (import.meta.env.DEV) {
  // Accessibility auditing - logs violations to browser console
  import('react').then((React) => {
    import('react-dom').then((ReactDOM) => {
      import('@axe-core/react').then((axe) => {
        axe.default(React, ReactDOM, 1000);
      });
    });
  }).catch(() => {
    // axe-core is optional
  });
}

// Setup global error handlers before React renders
setupGlobalErrorHandler();

// Force dark mode (light mode removed)
document.documentElement.setAttribute('data-theme', 'dark');
document.documentElement.classList.add('dark');

// Initialize React app with context providers
const rootElement = document.getElementById('root');

if (!rootElement) {
  console.error('Root element not found. Make sure index.html has <div id="root"></div>');
} else {
  const root = createRoot(rootElement);

  root.render(
    <StrictMode>
      <UserPreferencesProvider>
        <App />
      </UserPreferencesProvider>
    </StrictMode>
  );
}
