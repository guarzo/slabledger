# SlabLedger - Web Frontend

Modern React/TypeScript web application for analyzing Pokemon card grading opportunities.

## Tech Stack

- **React 19.2** - UI library
- **TypeScript 5.7** - Type safety
- **Tailwind CSS 4.1** - Utility-first styling
- **React Router 7.9** - Client-side routing
- **Vite 7.1** - Build tool & dev server
- **Vitest** - Testing framework
- **TanStack Virtual** - Virtual scrolling for large lists

## Quick Start

```bash
# Install dependencies
npm install

# Start development server
npm run dev

# Open browser
open http://localhost:5173
```

## Available Scripts

| Command | Description |
|---------|-------------|
| `npm run dev` | Start development server on port 5173 |
| `npm run build` | Build for production (outputs to ../dist/) |
| `npm run preview` | Preview production build |
| `npm run test` | Run unit tests |
| `npm run test:watch` | Run tests in watch mode |
| `npm run test:coverage` | Generate coverage report |
| `npm run typecheck` | Check TypeScript types |
| `npm run format` | Format code with Prettier |

## Project Structure

```
web/
├── src/
│   ├── main.tsx           # Entry point
│   ├── react/
│   │   ├── App.tsx        # Main app component
│   │   ├── pages/         # Route pages (3)
│   │   │   ├── OpportunitiesPage.tsx
│   │   │   ├── CollectionPage.tsx
│   │   │   └── PricingPage.tsx
│   │   ├── components/    # Feature components (10+)
│   │   │   ├── SearchBox.tsx
│   │   │   ├── CriteriaChips.tsx
│   │   │   ├── StatsBar.tsx
│   │   │   └── modals/    # Modal components
│   │   ├── ui/            # UI components (7)
│   │   │   ├── Button.tsx
│   │   │   ├── Badge.tsx
│   │   │   ├── Card.tsx
│   │   │   └── ...
│   │   ├── contexts/      # Context providers (4)
│   │   │   ├── AppStateContext.tsx
│   │   │   ├── ThemeContext.tsx
│   │   │   ├── ModalContext.tsx
│   │   │   └── UserPreferencesContext.tsx
│   │   ├── hooks/         # Custom hooks (8)
│   │   │   ├── useDebounce.ts
│   │   │   ├── useLocalStorage.ts
│   │   │   └── ...
│   │   └── charts/        # Chart components
│   ├── js/
│   │   ├── api.ts         # API client
│   │   └── errors.ts      # Error handling
│   └── css/
│       └── base.css       # Base styles
├── tests/                 # Test files (192+)
├── tailwind.config.cjs    # Tailwind config
├── vite.config.ts         # Vite config
└── package.json
```

## Features

- 🎨 **Dark Mode** - Full dark mode support with theme toggle
- 📱 **Responsive** - Mobile-first design (320px to 4K+)
- ♿ **Accessible** - WCAG 2.1 AA compliant, keyboard navigation
- ⚡ **Fast** - 86 KB gzipped main bundle, code splitting
- 🧪 **Tested** - 192+ tests with Vitest and React Testing Library
- 📊 **Charts** - Interactive price history with Chart.js (lazy loaded)
- 🔄 **Virtual Scrolling** - Handles 1000+ items efficiently
- 🎯 **Type Safe** - 100% TypeScript with strict mode

## Build Output

Current bundle sizes (gzipped):

| Asset | Size | Status |
|-------|------|--------|
| **main.js** | 86.46 KB | ✅ Excellent |
| **OpportunitiesPage** | 6.12 KB | ✅ Excellent |
| **CollectionPage** | 5.92 KB | ✅ Excellent |
| **PricingPage** | 4.75 KB | ✅ Excellent |
| **HistoryChart** | 92.31 KB | ✅ Acceptable (Chart.js) |
| **CSS** | 7.60 KB | ✅ Excellent |

**Total Initial Load:** ~94 KB gzipped (main + CSS)

## Development

### Adding a New Component

```bash
# Create component file
touch src/react/components/MyComponent.tsx

# Create test file
touch tests/components/MyComponent.test.tsx

# Create Storybook story (if using Storybook)
touch src/react/components/MyComponent.stories.tsx
```

### Running Tests

```bash
# Run tests in watch mode
npm run test:watch

# Run specific test file
npm run test MyComponent.test.tsx

# Generate coverage
npm run test:coverage
open coverage/index.html
```

### Type Checking

```bash
# Check types
npm run typecheck

# Watch mode (useful during development)
npx tsc --noEmit --watch
```

## Building for Production

```bash
# Build production bundle
npm run build

# Preview production build
npm run preview

# Check bundle size
du -sh ../dist/js/*.js
```

Build output goes to `../dist/` (parent directory) for Go server integration.

## Integration with Go Server

The Go server serves static files from `dist/` in production mode.

**Development:**
1. Go server runs on `:8080` (API endpoints)
2. Vite dev server runs on `:5173` (frontend with HMR)
3. Vite proxies `/api/*` requests to Go server

**Production:**
```go
// Go server serves static files from dist/
http.Handle("/", http.FileServer(http.Dir("dist")))
```

## API Client

The `api.ts` module provides typed API methods:

```typescript
import { api } from './js/api';

// Fetch opportunities
const opportunities = await api.getOpportunities();

// Fetch collection items (requires EBAY_APP_ID)
const items = await api.getCollectionItems();

// Get cache status
const status = await api.getCacheStatus();
```

## State Management

Application state is managed with React Context:

```typescript
import { useAppState } from './contexts/AppStateContext';
import { useTheme } from './contexts/ThemeContext';

function MyComponent() {
  const { activeFilters, applyFilters } = useAppState();
  const { theme, setTheme } = useTheme();

  // ...
}
```

## Styling

Uses Tailwind CSS utility classes:

```tsx
// ✅ Good - Tailwind utilities
<div className="bg-white dark:bg-gray-800 p-4 rounded-lg shadow-md">
  <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
    Title
  </h1>
</div>

// ❌ Avoid - Custom CSS classes
<div className="custom-card">
  <h1 className="custom-title">Title</h1>
</div>
```

## Performance Optimizations

- ✅ Route-based code splitting
- ✅ Chart.js lazy loaded (92 KB)
- ✅ Virtual scrolling for lists >150 items
- ✅ React.memo for card components
- ✅ useCallback for event handlers
- ✅ useMemo for expensive computations
- ✅ Image lazy loading
- ✅ Responsive images with srcSet

## Browser Support

- Chrome 90+
- Firefox 88+
- Safari 14+
- Edge 90+

(ES2020+ features, modern ESM support)

## Documentation

- [Migration Complete](../docs/ui/MIGRATION_COMPLETE.md) - Full migration summary
- [Component Library](../docs/ui/COMPONENT_LIBRARY.md) - Component documentation
- [Testing Guide](../docs/operations/TESTING.md) - Testing strategies

## Contributing

1. Create feature branch
2. Make changes
3. Add/update tests
4. Run `npm run typecheck`
5. Run `npm run test`
6. Create PR

## Common Issues

### TypeScript Errors

```bash
# Clear TypeScript cache
rm -rf node_modules/.cache

# Rebuild
npm run typecheck
```

### Build Failures

```bash
# Clear Vite cache
rm -rf node_modules/.vite

# Clean install
rm -rf node_modules package-lock.json
npm install
```

### Test Failures

```bash
# Clear test cache
npm run test -- --clearCache

# Run in watch mode for debugging
npm run test:watch
```

## License

ISC
