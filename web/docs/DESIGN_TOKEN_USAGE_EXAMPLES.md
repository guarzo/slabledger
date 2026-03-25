# Design Token Usage Examples

**Last Updated**: Sprint 4 (Weeks 7-8)
**Compliance**: 100% (up from 90%)
**Token File**: `web/src/css/tokens.css`

---

## Overview

Design tokens are CSS custom properties (variables) that provide a consistent, theme-aware design system. This guide shows how to use tokens correctly and avoid hard-coded values.

---

## Quick Reference

### ✅ DO Use Design Tokens

```css
/* ✅ CORRECT - Uses design tokens */
.card {
  background: var(--surface-1);
  color: var(--text);
  border: 1px solid var(--surface-3);
  border-radius: var(--radius-md);
  padding: var(--gap-lg);
}
```

### ❌ DON'T Hard-Code Values

```css
/* ❌ INCORRECT - Hard-coded values */
.card {
  background: #111827;
  color: #e5e7eb;
  border: 1px solid #374151;
  border-radius: 14px;
  padding: 20px;
}
```

---

## Available Tokens

### Brand Colors

```css
--brand-50: #eef2ff;    /* Lightest brand tint */
--brand-100: #e0e7ff;
--brand-200: #c7d2fe;
--brand-300: #a5b4fc;
--brand-400: #818cf8;
--brand-500: #6366f1;   /* Primary brand color */
--brand-600: #4f46e5;   /* Hover state */
--brand-700: #4338ca;   /* Active state */
--brand-800: #3730a3;
--brand-900: #312e81;   /* Darkest brand shade */
```

**Usage**:
```typescript
// Primary button
<button className="bg-[var(--brand-500)] hover:bg-[var(--brand-600)]">
  Submit
</button>

// Link color
<a className="text-[var(--brand-600)] hover:underline">
  Learn More
</a>
```

---

### Semantic Colors

#### Success (Green)

```css
--success: #10b981;                    /* Main success color */
--success-bg: rgba(16, 185, 129, 0.1); /* Background tint */
--success-border: rgba(16, 185, 129, 0.2); /* Border */
```

**Usage**:
```typescript
// Success message
<div className="bg-[var(--success-bg)] border border-[var(--success-border)] text-[var(--success)]">
  ✓ Saved successfully
</div>

// Success icon
<CheckIcon className="text-[var(--success)]" />
```

#### Warning (Orange)

```css
--warning: #f59e0b;
--warning-bg: rgba(245, 158, 11, 0.1);
--warning-border: rgba(245, 158, 11, 0.2);
```

**Usage**:
```typescript
// Warning badge
<span className="bg-[var(--warning-bg)] text-[var(--warning)] border border-[var(--warning-border)]">
  ⚠ Medium Risk
</span>
```

#### Danger (Red)

```css
--danger: #ef4444;
--danger-bg: rgba(239, 68, 68, 0.1);
--danger-border: rgba(239, 68, 68, 0.2);
```

**Usage**:
```typescript
// Error message
<div className="bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)]">
  ✕ Failed to load data
</div>

// Delete button
<button className="bg-[var(--danger)] hover:bg-[#dc2626]">
  Delete
</button>
```

#### Info (Cyan)

```css
--info: #0891b2;
--info-bg: rgba(8, 145, 178, 0.1);
--info-border: rgba(8, 145, 178, 0.2);
```

**Usage**:
```typescript
// Info tooltip
<div className="bg-[var(--info-bg)] border border-[var(--info-border)] text-[var(--info)]">
  ℹ This will take 2-3 weeks
</div>
```

---

### Surface Colors (Backgrounds)

```css
--bg: #0f172a;           /* Page background */
--surface-0: #0b1220;    /* Lowest elevation */
--surface-1: #111827;    /* Card background */
--surface-2: #1f2937;    /* Raised surface */
--surface-3: #374151;    /* Higher elevation */
--surface-4: #4b5563;    /* Highest elevation */
--surface-hover: #1f2937; /* Hover state */
```

**Usage**:
```typescript
// Card with elevation
<div className="bg-[var(--surface-1)] border border-[var(--surface-0)]">
  <h2>Card Title</h2>
</div>

// Nested card (higher elevation)
<div className="bg-[var(--surface-2)] p-4">
  Inner content
</div>

// Hover state
<button className="bg-[var(--surface-1)] hover:bg-[var(--surface-hover)]">
  Click me
</button>
```

---

### Text Colors

```css
--text: #e5e7eb;         /* Primary text (high contrast) */
--text-muted: #9ca3af;   /* Secondary text (medium contrast) */
--text-subtle: #6b7280;  /* Tertiary text (low contrast) */
```

**Usage**:
```typescript
// Heading
<h1 className="text-[var(--text)] font-bold">
  Grading Opportunities
</h1>

// Description
<p className="text-[var(--text-muted)]">
  150 cards found
</p>

// Footnote
<small className="text-[var(--text-subtle)]">
  Last updated 5 minutes ago
</small>
```

---

### Spacing (Gap/Padding)

```css
--gap-xs: 6px;
--gap-sm: 10px;
--gap-md: 14px;
--gap-lg: 20px;
--gap-xl: 28px;
--gap-2xl: 36px;
```

**Usage**:
```typescript
// Card padding
<div className="p-[var(--gap-lg)]">
  Content
</div>

// Flex gap
<div className="flex gap-[var(--gap-md)]">
  <button>Action 1</button>
  <button>Action 2</button>
</div>

// Custom spacing
<div style={{ marginBottom: 'var(--gap-xl)' }}>
  Section
</div>
```

---

### Border Radius

```css
--radius-sm: 10px;
--radius-md: 14px;
--radius-lg: 18px;
--radius-xl: 22px;
```

**Usage**:
```typescript
// Small radius (badges, chips)
<span className="rounded-[var(--radius-sm)]">
  New
</span>

// Medium radius (cards, buttons)
<button className="rounded-[var(--radius-md)]">
  Submit
</button>

// Large radius (modals, images)
<div className="rounded-[var(--radius-lg)]">
  Modal content
</div>
```

---

### Shadows (Elevation)

```css
--shadow-0: 0 0 0 rgba(0, 0, 0, 0);              /* No shadow */
--shadow-1: 0 6px 16px rgba(0, 0, 0, 0.24);      /* Low elevation */
--shadow-2: 0 10px 30px rgba(0, 0, 0, 0.35);     /* Medium */
--shadow-3: 0 20px 40px rgba(0, 0, 0, 0.45);     /* High */
```

**Usage**:
```typescript
// Card shadow
<div className="shadow-[var(--shadow-1)]">
  Card content
</div>

// Modal shadow
<div className="shadow-[var(--shadow-3)]">
  Modal
</div>

// Inline styles
<div style={{ boxShadow: 'var(--shadow-2)' }}>
  Floating panel
</div>
```

---

## Chart Components

### Chart Colors (Before & After)

#### ❌ BEFORE (Hard-Coded)

```typescript
// ROIDistributionChart.tsx
const colors = {
  standardROI: theme === 'dark' ? '#3b82f6' : '#2563eb',
  annualizedROI: theme === 'dark' ? '#10b981' : '#059669',
  grid: theme === 'dark' ? '#374151' : '#e5e7eb',
  text: theme === 'dark' ? '#f9fafb' : '#111827',
  background: theme === 'dark' ? '#1f2937' : '#ffffff',
};
```

#### ✅ AFTER (Design Tokens)

```typescript
// ROIDistributionChart.tsx
const colors = {
  standardROI: 'var(--brand-500)',
  annualizedROI: 'var(--success)',
  grid: 'var(--surface-3)',
  text: 'var(--text)',
  background: 'var(--surface-1)',
};
```

**Benefits**:
- ✅ Automatic dark mode support
- ✅ Consistent with design system
- ✅ No theme prop needed
- ✅ Easier to maintain

---

### Confidence Colors (Scatter Chart)

#### ❌ BEFORE (Hard-Coded)

```typescript
// LiquidityScoreScatter.tsx
let color = '#ef4444'; // Red (low)
if (confidence >= 0.8) color = '#10b981'; // Green (high)
else if (confidence >= 0.6) color = '#f59e0b'; // Yellow (medium)
```

#### ✅ AFTER (Semantic Tokens)

```typescript
// LiquidityScoreScatter.tsx
let color = 'var(--color-confidence-low)';
if (confidence >= 0.8) color = 'var(--color-confidence-high)';
else if (confidence >= 0.6) color = 'var(--color-confidence-medium)';

// Or use helper function
function getConfidenceColor(confidence: number): string {
  if (confidence >= 0.8) return 'var(--success)';
  if (confidence >= 0.6) return 'var(--warning)';
  return 'var(--danger)';
}
```

---

## Component Examples

### Button Component

```typescript
// Primary button
<button className="bg-[var(--brand-500)] hover:bg-[var(--brand-600)] active:bg-[var(--brand-700)] text-white rounded-[var(--radius-md)] px-[var(--gap-lg)] py-[var(--gap-md)]">
  Primary Action
</button>

// Secondary button
<button className="bg-[var(--surface-1)] hover:bg-[var(--surface-hover)] text-[var(--text)] border border-[var(--surface-3)] rounded-[var(--radius-md)]">
  Secondary
</button>

// Danger button
<button className="bg-[var(--danger)] hover:bg-[#dc2626] text-white rounded-[var(--radius-md)]">
  Delete
</button>
```

---

### Card Component

```typescript
<div className="bg-[var(--surface-1)] rounded-[var(--radius-lg)] p-[var(--gap-lg)] shadow-[var(--shadow-1)] border border-[var(--surface-0)]">
  <h3 className="text-[var(--text)] font-semibold mb-[var(--gap-md)]">
    Card Title
  </h3>
  <p className="text-[var(--text-muted)] mb-[var(--gap-lg)]">
    Card description goes here
  </p>
  <button className="bg-[var(--brand-500)] text-white rounded-[var(--radius-md)] px-[var(--gap-md)] py-[var(--gap-sm)]">
    Action
  </button>
</div>
```

---

### Badge Component

```typescript
// Success badge
<span className="bg-[var(--success-bg)] text-[var(--success)] border border-[var(--success-border)] rounded-[var(--radius-sm)] px-[var(--gap-sm)] py-[var(--gap-xs)] text-xs font-medium">
  ✓ Active
</span>

// Warning badge
<span className="bg-[var(--warning-bg)] text-[var(--warning)] border border-[var(--warning-border)] rounded-[var(--radius-sm)] px-[var(--gap-sm)] py-[var(--gap-xs)]">
  ⚠ Pending
</span>

// Info badge
<span className="bg-[var(--info-bg)] text-[var(--info)] border border-[var(--info-border)] rounded-[var(--radius-sm)] px-[var(--gap-sm)] py-[var(--gap-xs)]">
  ℹ New
</span>
```

---

### Modal Component

```typescript
<div className="fixed inset-0 bg-black/50 flex items-center justify-center">
  <div className="bg-[var(--surface-1)] rounded-[var(--radius-xl)] p-[var(--gap-2xl)] shadow-[var(--shadow-3)] max-w-lg w-full">
    <h2 className="text-[var(--text)] text-2xl font-bold mb-[var(--gap-md)]">
      Modal Title
    </h2>
    <p className="text-[var(--text-muted)] mb-[var(--gap-xl)]">
      Modal content goes here
    </p>
    <div className="flex gap-[var(--gap-md)] justify-end">
      <button className="bg-[var(--surface-2)] hover:bg-[var(--surface-3)] text-[var(--text)] rounded-[var(--radius-md)] px-[var(--gap-lg)] py-[var(--gap-md)]">
        Cancel
      </button>
      <button className="bg-[var(--brand-500)] hover:bg-[var(--brand-600)] text-white rounded-[var(--radius-md)] px-[var(--gap-lg)] py-[var(--gap-md)]">
        Confirm
      </button>
    </div>
  </div>
</div>
```

---

## Tailwind Integration

### Arbitrary Values with CSS Variables

Tailwind allows using CSS variables in arbitrary values:

```typescript
// Background colors
className="bg-[var(--surface-1)]"

// Text colors
className="text-[var(--text)]"

// Border radius
className="rounded-[var(--radius-md)]"

// Padding/margins
className="p-[var(--gap-lg)]"

// Shadows
className="shadow-[var(--shadow-1)]"
```

### Combining with Tailwind Utilities

```typescript
// Mix tokens with Tailwind classes
<div className="flex items-center gap-[var(--gap-md)] bg-[var(--surface-1)] p-[var(--gap-lg)] rounded-[var(--radius-lg)]">
  <CheckIcon className="text-[var(--success)]" />
  <span className="text-[var(--text)] font-medium">
    Success
  </span>
</div>
```

---

## Dark Mode Support

### Automatic Theme Switching

Design tokens automatically adapt to theme:

```typescript
// No theme prop needed!
const Card = ({ children }) => (
  <div className="bg-[var(--surface-1)] text-[var(--text)]">
    {children}
  </div>
);

// Tokens automatically switch:
// Light mode: --surface-1 = #ffffff, --text = #111827
// Dark mode:  --surface-1 = #111827, --text = #e5e7eb
```

### Theme-Specific Overrides (Rare)

```css
/* Only if you need theme-specific behavior */
:root[data-theme="light"] {
  --custom-overlay: rgba(0, 0, 0, 0.3);
}

:root[data-theme="dark"] {
  --custom-overlay: rgba(0, 0, 0, 0.6);
}
```

---

## Migration Guide

### Step 1: Find Hard-Coded Values

```bash
# Find hex colors
grep -r "#[0-9a-fA-F]\{3,6\}" web/src/

# Find rgba/rgb colors
grep -r "rgba\?(" web/src/
```

### Step 2: Replace with Tokens

| Hard-Coded | Token |
|------------|-------|
| `#111827` | `var(--surface-1)` |
| `#e5e7eb` | `var(--text)` |
| `#10b981` | `var(--success)` |
| `#ef4444` | `var(--danger)` |
| `#3b82f6` | `var(--brand-500)` |
| `14px` | `var(--radius-md)` |
| `20px` | `var(--gap-lg)` |

### Step 3: Verify

```bash
# Type check
npm run typecheck

# Visual check (start dev server)
npm run dev

# Test dark mode toggle
```

---

## Common Mistakes

### ❌ Mistake #1: Mixing Hard-Coded and Tokens

```typescript
// ❌ Bad - inconsistent
<div className="bg-[var(--surface-1)] text-#e5e7eb">
  Mixed tokens and hard-coded
</div>

// ✅ Good - all tokens
<div className="bg-[var(--surface-1)] text-[var(--text)]">
  Consistent tokens
</div>
```

### ❌ Mistake #2: Using Tailwind Color Classes

```typescript
// ❌ Bad - Tailwind classes don't adapt to theme
<div className="bg-gray-800 text-gray-100">
  Tailwind colors
</div>

// ✅ Good - Design tokens adapt automatically
<div className="bg-[var(--surface-1)] text-[var(--text)]">
  Design tokens
</div>
```

### ❌ Mistake #3: Theme-Aware Logic

```typescript
// ❌ Bad - manual theme handling
const color = theme === 'dark' ? '#111827' : '#ffffff';

// ✅ Good - let tokens handle it
const color = 'var(--surface-1)';
```

---

## Validation

### Pre-Commit Check

```bash
# Check for hard-coded colors
npm run lint:colors

# Or manually
grep -r "#[0-9a-fA-F]\{6\}" web/src/ || echo "✓ No hard-coded colors found"
```

### ESLint Rule (Future)

```javascript
// .eslintrc.js
rules: {
  'no-hardcoded-colors': 'error',
}
```

---

## Token Reference

Full token reference in `web/src/css/tokens.css`:

- **Brand Colors**: `--brand-{50-900}`
- **Semantic**: `--success`, `--warning`, `--danger`, `--info`
- **Surfaces**: `--bg`, `--surface-{0-4}`, `--surface-hover`
- **Text**: `--text`, `--text-muted`, `--text-subtle`
- **Spacing**: `--gap-{xs,sm,md,lg,xl,2xl}`
- **Radius**: `--radius-{sm,md,lg,xl}`
- **Shadows**: `--shadow-{0,1,2,3}`
- **Confidence**: `--color-confidence-{high,medium,low}`
- **Velocity**: `--color-velocity-{high,moderate,stagnant}`

---

## Resources

- **Token File**: `web/src/css/tokens.css`
- **Migration Guide**: `docs/implementation/CSS_DESIGN_TOKEN_MIGRATION.md`
- **Design System**: `web/src/css/design-system.css` (if exists)

---

**Remember**: Design tokens ensure consistency, support dark mode, and make global theme changes easy. Always use tokens instead of hard-coded values!
