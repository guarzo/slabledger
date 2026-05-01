import js from '@eslint/js';
import tseslint from '@typescript-eslint/eslint-plugin';
import tsparser from '@typescript-eslint/parser';
import react from 'eslint-plugin-react';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import globals from 'globals';

export default [
  {
    ignores: ['dist/**', 'node_modules/**', 'playwright-report/**', 'test-results/**', '.lighthouseci/**', 'storybook-static/**', 'screenshots/**'],
  },
  js.configs.recommended,
  {
    files: ['**/*.{ts,tsx,js,jsx}'],
    languageOptions: {
      parser: tsparser,
      parserOptions: {
        ecmaVersion: 'latest',
        sourceType: 'module',
        ecmaFeatures: {
          jsx: true,
        },
      },
      globals: {
        ...globals.browser,
        ...globals.es2021,
        // React types (provided by JSX transform)
        React: 'readonly',
        // TypeScript DOM types
        RequestInit: 'readonly',
        EventListener: 'readonly',
        KeyboardEventInit: 'readonly',
        // Test globals
        describe: 'readonly',
        it: 'readonly',
        test: 'readonly',
        expect: 'readonly',
        beforeEach: 'readonly',
        afterEach: 'readonly',
        beforeAll: 'readonly',
        afterAll: 'readonly',
        vi: 'readonly',
      },
    },
    plugins: {
      '@typescript-eslint': tseslint,
      'react': react,
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      // TypeScript rules
      '@typescript-eslint/no-unused-vars': ['warn', {
        argsIgnorePattern: '^_',
        varsIgnorePattern: '^_',
      }],
      '@typescript-eslint/no-explicit-any': 'warn',

      // React rules
      'react/react-in-jsx-scope': 'off', // Not needed in React 17+
      'react/prop-types': 'off', // Using TypeScript for prop validation
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],

      // General rules
      // no-console: all application code must route errors through
      // reportError() from src/js/errors.ts so we have a single telemetry
      // funnel. The outermost crash paths (errors.ts, main.tsx, the two
      // ErrorBoundary files) need raw console access and are allowlisted
      // via an override below.
      'no-console': 'error',
      'no-debugger': 'warn',
      'no-unused-vars': 'off', // Using TypeScript version instead
      'prefer-const': 'warn',
      'no-var': 'error',
    },
    settings: {
      react: {
        version: 'detect',
      },
    },
  },
  // Allowlist: files that may call console.* directly. These are the
  // outermost crash paths where reportError() is not available, plus
  // test code and service workers which don't have access to the React
  // error-reporting funnel.
  {
    files: [
      'src/js/errors.ts',
      'src/main.tsx',
      'src/react/ErrorBoundary.tsx',
      'src/react/ui/SectionErrorBoundary.tsx',
      'public/**/*.{js,ts}',
      'tests/**/*.{js,ts}',
    ],
    rules: {
      'no-console': 'off',
    },
  },
  // Node.js config files and CommonJS modules
  {
    files: ['*.config.{js,ts}', 'tests/**/*.{js,ts}', 'tailwind-plugins/**/*.js', 'vitest.d.ts'],
    languageOptions: {
      globals: {
        ...globals.node,
        ...globals.browser,
        ...globals.es2021,
        // CommonJS globals
        require: 'readonly',
        module: 'readonly',
        exports: 'readonly',
        __dirname: 'readonly',
        __filename: 'readonly',
        // Test globals
        describe: 'readonly',
        it: 'readonly',
        test: 'readonly',
        expect: 'readonly',
        beforeEach: 'readonly',
        afterEach: 'readonly',
        beforeAll: 'readonly',
        afterAll: 'readonly',
        vi: 'readonly',
      },
    },
    rules: {
      // Disable React Hook rules for test files (they may use "use" from test frameworks)
      'react-hooks/rules-of-hooks': 'off',
      'react-hooks/exhaustive-deps': 'off',
    },
  },
];
