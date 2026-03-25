# Type Definitions Guide

**Last Updated**: Sprint 4 (Weeks 7-8)
**Purpose**: Comprehensive guide to TypeScript type definitions in the web frontend

---

## Overview

The frontend uses a well-organized type system with dedicated type definition files for different feature areas. This guide explains the structure, usage, and best practices for working with types.

## Type File Structure

```
web/src/types/
├── api.ts          # API responses and backend communication
├── opportunity.ts  # Opportunity/grading analysis types
├── card.ts         # Pokemon card types
├── chart.ts        # Chart and visualization types
├── modal.ts        # Modal component types
├── collection.ts   # Collection management types
└── filter.ts       # Filter and search types
```

---

## Core Type Files

### 1. `card.ts` - Pokemon Card Types

**Purpose**: Base card types and grading-related structures

#### Key Types

```typescript
// Base card representation
interface Card {
  id: string;
  name: string;
  number: string;
  setId: string;
  setName: string;
  imageUrl?: string;
  rarity?: string;
  supertype?: string;
  subtypes?: string[];
}

// Graded card with certification
interface GradedCard extends Card {
  grade: number;
  certNumber: string;
  gradingCompany: 'PSA' | 'BGS' | 'CGC';
  gradedDate?: string;
}

// Card with pricing data
interface PricedCard extends Card {
  rawPrice?: number;
  psa9Price?: number;
  psa10Price?: number;
}
```

#### Usage Example

```typescript
import type { Card, GradedCard, CardCondition } from '../types/card';

const displayCard = (card: Card) => {
  console.log(`${card.name} #${card.number} from ${card.setName}`);
};

const isHighGrade = (card: GradedCard): boolean => {
  return card.grade >= 9;
};
```

#### Available Types

- `Card` - Base card interface
- `GradedCard` - Card with grading info
- `PricedCard` - Card with price data
- `GradingCompany` - Union type: `'PSA' | 'BGS' | 'CGC' | 'SGC'`
- `CardGrade` - Literal type: `1 | 2 | ... | 9 | 9.5 | 10`
- `CardCondition` - Ungraded conditions: `'Near Mint' | 'Lightly Played' | ...`
- `CardRarity` - All Pokemon card rarities

---

### 2. `chart.ts` - Chart & Visualization Types

**Purpose**: Types for chart components (Recharts)

#### Key Types

```typescript
// Generic data point
interface ChartDataPoint {
  x: number;
  y: number;
  label: string;
  color?: string;
}

// Chart configuration
interface ChartConfig {
  title: string;
  xAxisLabel: string;
  yAxisLabel: string;
  showGrid: boolean;
  showLegend: boolean;
}

// Custom Recharts tooltip
type CustomTooltipProps = TooltipProps<number, string>;
```

#### Usage Example

```typescript
import type { ChartDataPoint, ChartConfig } from '../types/chart';

const chartConfig: ChartConfig = {
  title: 'ROI Distribution',
  xAxisLabel: 'ROI Range',
  yAxisLabel: 'Count',
  showGrid: true,
  showLegend: true,
};

const dataPoints: ChartDataPoint[] = opportunities.map(opp => ({
  x: opp.roi,
  y: opp.score,
  label: opp.name,
}));
```

---

### 3. `modal.ts` - Modal Component Types

**Purpose**: Props for modal/dialog components

#### Key Types

```typescript
// Base modal props
interface BaseModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'full';
}

// Confirmation modal
interface ConfirmModalProps extends BaseModalProps {
  onConfirm: () => void | Promise<void>;
  confirmText?: string;
  cancelText?: string;
  variant?: 'danger' | 'warning' | 'info' | 'success';
}
```

#### Usage Example

```typescript
import type { ConfirmModalProps } from '../types/modal';

const DeleteConfirmModal: React.FC<ConfirmModalProps> = ({
  isOpen,
  onClose,
  onConfirm,
  title,
  children,
  variant = 'danger',
}) => {
  // Implementation
};
```

---

### 4. `collection.ts` - Collection Management Types

**Purpose**: User collection and inventory types

#### Key Types

```typescript
// Collection item
interface CollectionItem {
  id: string;
  card: Card;
  quantity: number;
  condition: CardCondition | 'Graded';
  purchasePrice?: number;
  gradedInfo?: {
    grade: number;
    certNumber: string;
    gradingCompany: 'PSA' | 'BGS' | 'CGC';
  };
}

// Collection statistics
interface CollectionStats {
  totalItems: number;
  totalValue: number;
  gradedCount: number;
  ungradedCount: number;
}
```

#### Usage Example

```typescript
import type { CollectionItem, CollectionStats } from '../types/collection';

const calculateStats = (items: CollectionItem[]): CollectionStats => {
  return {
    totalItems: items.length,
    totalValue: items.reduce((sum, item) => sum + (item.purchasePrice || 0), 0),
    gradedCount: items.filter(item => item.condition === 'Graded').length,
    ungradedCount: items.filter(item => item.condition !== 'Graded').length,
  };
};
```

---

### 5. `filter.ts` - Filter & Search Types

**Purpose**: Filter criteria, presets, and search options

#### Key Types

```typescript
// Filter criteria
interface FilterCriteria {
  minROI?: number;
  maxROI?: number;
  minScore?: number;
  liquidity?: LiquidityLevel[];
  scarcity?: ScarcityLevel[];
  sets?: string[];
}

// Filter preset (saved filter)
interface FilterPreset {
  id: string;
  name: string;
  criteria: FilterCriteria;
  isDefault: boolean;
}

// Built-in presets
const BUILT_IN_PRESETS: Record<string, FilterCriteria>;
```

#### Usage Example

```typescript
import type { FilterCriteria, FilterPreset } from '../types/filter';
import { BUILT_IN_PRESETS } from '../types/filter';

// Apply built-in preset
const applyHighROIFilter = () => {
  const criteria = BUILT_IN_PRESETS['high-roi'];
  filterOpportunities(opportunities, criteria);
};

// Create custom filter
const customFilter: FilterCriteria = {
  minROI: 100,
  liquidity: ['High'],
  sets: ['sv8', 'sv7'],
};
```

---

## Best Practices

### ✅ DO

1. **Import types explicitly**:
   ```typescript
   import type { Card } from '../types/card';
   ```

2. **Use specific types over generic ones**:
   ```typescript
   // ✅ Good
   const card: Card = { ... };

   // ❌ Bad
   const card: any = { ... };
   ```

3. **Extend existing types when needed**:
   ```typescript
   interface EnhancedCard extends Card {
     marketPrice: number;
   }
   ```

4. **Use union types for variants**:
   ```typescript
   type Status = 'pending' | 'processing' | 'complete' | 'failed';
   ```

### ❌ DON'T

1. **Don't use `any` in production code**
2. **Don't duplicate type definitions** - import from shared files
3. **Don't mix type imports with value imports unnecessarily**
4. **Don't create overly complex nested types** - break them down

---

## Type Guards

Use type guards for runtime type checking:

```typescript
// Type guard for Card
function isCard(value: unknown): value is Card {
  return (
    typeof value === 'object' &&
    value !== null &&
    'id' in value &&
    'name' in value
  );
}

// Usage
if (isCard(data)) {
  console.log(data.name); // TypeScript knows this is a Card
}
```

---

## Utility Types

Common TypeScript utility types used in the codebase:

```typescript
// Partial - make all properties optional
type PartialCard = Partial<Card>;

// Pick - select specific properties
type CardBasics = Pick<Card, 'id' | 'name' | 'number'>;

// Omit - exclude specific properties
type CardWithoutImage = Omit<Card, 'imageUrl'>;

// Record - object with specific key/value types
type CardMap = Record<string, Card>;
```

---

## Type Safety Checklist

Before committing code:

- [ ] All function parameters have explicit types
- [ ] All function return types are specified
- [ ] No `any` types in production code
- [ ] API responses use proper type definitions
- [ ] Component props use defined interfaces
- [ ] Event handlers have proper type annotations
- [ ] `tsc --noEmit` passes without errors

---

## Common Patterns

### API Response Handling

```typescript
import type { OpportunityRow } from '../types/opportunity';

async function fetchOpportunities(): Promise<OpportunityRow[]> {
  const response = await api.getOpportunities();

  // Type narrowing for wrapped responses
  const data = Array.isArray(response)
    ? response
    : (response?.rows || response?.opportunities || []);

  return data;
}
```

### Component Props

```typescript
import type { Card } from '../types/card';

interface CardDisplayProps {
  card: Card;
  showImage?: boolean;
  onSelect?: (card: Card) => void;
}

export default function CardDisplay({
  card,
  showImage = true,
  onSelect
}: CardDisplayProps) {
  // Implementation
}
```

---

## IDE Integration

### VS Code

TypeScript types provide:
- **IntelliSense**: Auto-completion for properties and methods
- **Type Checking**: Real-time error detection
- **Go to Definition**: Jump to type definitions with F12
- **Find References**: See where types are used

### Type Hints

Hover over variables to see inferred types:

```typescript
const card = getCard(); // Hover shows: const card: Card
```

---

## Resources

- [TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html)
- [React TypeScript Cheatsheet](https://react-typescript-cheatsheet.netlify.app/)
- [Type vs Interface](https://www.typescriptlang.org/docs/handbook/2/everyday-types.html#differences-between-type-aliases-and-interfaces)

---

## Troubleshooting

### Common Errors

**Error**: `Property 'xyz' does not exist on type 'Card'`
- **Fix**: Check if property exists in type definition or use optional chaining

**Error**: `Type 'X' is not assignable to type 'Y'`
- **Fix**: Ensure object structure matches interface definition

**Error**: `Cannot find module '../types/xyz'`
- **Fix**: Check import path and ensure file exists

### Getting Help

1. Run `npm run typecheck` to see all type errors
2. Check type definition files in `web/src/types/`
3. Review this guide for usage examples
4. Consult TypeScript documentation
