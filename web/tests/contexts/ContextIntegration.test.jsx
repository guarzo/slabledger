import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { UserPreferencesProvider, useUserPreferences } from '../../src/react/contexts/UserPreferencesContext';

describe('Context Integration Tests', () => {
  describe('UserPreferencesContext', () => {
    beforeEach(() => {
      localStorage.clear();
    });

    // Note: localStorage persistence is thoroughly tested in useLocalStorage.test.js (20 tests)
    // These tests focus on the context API itself

    it('should provide access to preferences methods', () => {
      const TestComponent = () => {
        const { addRecentPriceCheck, clearRecentPriceChecks } = useUserPreferences();

        return (
          <div>
            <div data-testid="has-add">{typeof addRecentPriceCheck === 'function' ? 'yes' : 'no'}</div>
            <div data-testid="has-clear">{typeof clearRecentPriceChecks === 'function' ? 'yes' : 'no'}</div>
          </div>
        );
      };

      render(
        <UserPreferencesProvider>
          <TestComponent />
        </UserPreferencesProvider>
      );

      expect(screen.getByTestId('has-add')).toHaveTextContent('yes');
      expect(screen.getByTestId('has-clear')).toHaveTextContent('yes');
    });

  });
});
