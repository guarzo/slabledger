import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import CardShell from './CardShell';

/**
 * CardShell - Foundation component for all cards
 *
 * ## Purpose
 * CardShell enforces design token usage and provides common card behaviors.
 * All card components should build on top of this component.
 *
 * ## Key Features
 * - ✅ Design token enforcement (no hard-coded colors)
 * - ✅ Dark mode support via CSS variables
 * - ✅ Accessibility (keyboard navigation, ARIA, focus management)
 * - ✅ Selection state (for comparison/multi-select UIs)
 * - ✅ Interactive behaviors (hover, click, keyboard)
 * - ✅ Flexible variants (default, elevated, interactive, premium)
 *
 * ## Design Tokens Used
 * - `--surface-0` - Flat surface (lowest elevation)
 * - `--surface-1` - Card surface (base elevation)
 * - `--surface-2` - Raised surface (highest elevation)
 * - `--surface-hover` - Hover state background
 * - `--text` - Primary text color
 * - `--text-muted` - Secondary text color
 * - `--brand-500` - Brand color for selection rings
 * - `--shadow-1`, `--shadow-2` - Elevation shadows
 * - `--radius-lg` - Border radius
 * - `--transition-base` - Transition duration
 *
 * @see /web/src/css/tokens.css for complete token definitions
 */
const meta: Meta<typeof CardShell> = {
  title: 'UI/CardShell',
  component: CardShell,
  parameters: {
    layout: 'centered',
    docs: {
      description: {
        component: 'Foundation card component that enforces design token usage and provides common behaviors.',
      },
    },
  },
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['default', 'elevated', 'interactive', 'premium'],
      description: 'Visual style variant',
      table: {
        defaultValue: { summary: 'default' },
      },
    },
    padding: {
      control: 'select',
      options: ['none', 'sm', 'md', 'lg'],
      description: 'Padding size',
      table: {
        defaultValue: { summary: 'md' },
      },
    },
    selectable: {
      control: 'boolean',
      description: 'Whether the card can be selected',
    },
    isSelected: {
      control: 'boolean',
      description: 'Whether the card is currently selected',
    },
    ariaLabel: {
      control: 'text',
      description: 'ARIA label for accessibility',
    },
  },
  decorators: [
    (Story) => (
      <div className="min-h-screen bg-[var(--bg)] p-8">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof CardShell>;

/**
 * Default variant - Standard card appearance
 * Uses base surface with standard shadow
 */
export const Default: Story = {
  args: {
    children: (
      <div>
        <h3 className="text-lg font-semibold mb-2">Default Card</h3>
        <p className="text-[var(--text-muted)]">
          Standard card appearance using --surface-1 background and --shadow-1 elevation.
        </p>
      </div>
    ),
  },
};

/**
 * Elevated variant - Raised card appearance
 * Higher elevation with more prominent shadow
 */
export const Elevated: Story = {
  args: {
    variant: 'elevated',
    children: (
      <div>
        <h3 className="text-lg font-semibold mb-2">Elevated Card</h3>
        <p className="text-[var(--text-muted)]">
          Uses --surface-2 background and --shadow-2 for increased prominence.
        </p>
      </div>
    ),
  },
};

/**
 * Interactive variant - Clickable card with hover effects
 * Includes hover states and keyboard focus ring
 */
export const Interactive: Story = {
  args: {
    variant: 'interactive',
    onClick: () => alert('Card clicked!'),
    ariaLabel: 'Interactive card example',
    children: (
      <div>
        <h3 className="text-lg font-semibold mb-2">Interactive Card</h3>
        <p className="text-[var(--text-muted)]">
          Hover over me! Includes hover lift effect, focus ring, and click handler.
          Try keyboard navigation (Tab + Enter/Space).
        </p>
      </div>
    ),
  },
};

/**
 * Premium variant - Featured card with gradient and glow
 * Subtle gradient background for special/featured cards
 */
export const Premium: Story = {
  args: {
    variant: 'premium',
    children: (
      <div>
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-lg font-semibold">Premium Card</h3>
          <span className="text-xs font-semibold px-2 py-0.5 rounded-full bg-[var(--warning-bg)] text-[var(--warning)] border border-[var(--warning-border)]">Featured</span>
        </div>
        <p className="text-[var(--text-muted)]">
          Gradient background with brand border and glow effect on hover.
          Perfect for highlighting special content.
        </p>
      </div>
    ),
  },
};

/**
 * Padding variants showcase
 */
export const PaddingVariants: Story = {
  render: () => (
    <div className="grid grid-cols-2 gap-6 w-[700px]">
      <CardShell padding="none" variant="elevated">
        <div className="bg-[var(--surface-0)] p-3">
          <p className="text-sm font-semibold">No Padding</p>
          <p className="text-xs text-[var(--text-muted)]">padding: none</p>
        </div>
      </CardShell>
      <CardShell padding="sm" variant="elevated">
        <p className="text-sm font-semibold">Small Padding</p>
        <p className="text-xs text-[var(--text-muted)]">padding: sm (12px)</p>
      </CardShell>
      <CardShell padding="md" variant="elevated">
        <p className="text-sm font-semibold">Medium Padding</p>
        <p className="text-xs text-[var(--text-muted)]">padding: md (16px)</p>
      </CardShell>
      <CardShell padding="lg" variant="elevated">
        <p className="text-sm font-semibold">Large Padding</p>
        <p className="text-xs text-[var(--text-muted)]">padding: lg (24px)</p>
      </CardShell>
    </div>
  ),
};

/**
 * Selectable card - For comparison/multi-select UIs
 * Click to toggle selection state with ring indicator
 */
export const Selectable: Story = {
  render: function SelectableCard() {
    const [isSelected, setIsSelected] = useState(false);

    return (
      <div className="space-y-4">
        <CardShell
          variant="interactive"
          selectable
          isSelected={isSelected}
          onToggleSelect={() => setIsSelected(!isSelected)}
          ariaLabel="Selectable card example"
          className="w-80"
        >
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-lg font-semibold mb-1">
                {isSelected ? 'Selected' : 'Not Selected'}
              </h3>
              <p className="text-sm text-[var(--text-muted)]">
                Click to toggle selection state
              </p>
            </div>
            <div className="w-5 h-5 rounded border-2 flex items-center justify-center"
                 style={{
                   borderColor: isSelected ? 'var(--brand-500)' : 'var(--text-muted)',
                   backgroundColor: isSelected ? 'var(--brand-500)' : 'transparent',
                 }}>
              {isSelected && (
                <svg className="w-3 h-3 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
                </svg>
              )}
            </div>
          </div>
        </CardShell>
        <p className="text-xs text-[var(--text-subtle)]">
          Try keyboard navigation: Tab to focus, Space/Enter to toggle
        </p>
      </div>
    );
  },
};

/**
 * Multiple selectable cards - Comparison mode example
 */
export const MultipleSelectable: Story = {
  render: function MultipleSelectableCards() {
    const [selected, setSelected] = useState<Set<number>>(new Set());

    const toggleCard = (id: number) => {
      setSelected(prev => {
        const next = new Set(prev);
        if (next.has(id)) {
          next.delete(id);
        } else {
          next.add(id);
        }
        return next;
      });
    };

    const cards = [
      { id: 1, title: 'Charizard', set: 'Base Set', price: '$5,000' },
      { id: 2, title: 'Blastoise', set: 'Base Set', price: '$2,500' },
      { id: 3, title: 'Venusaur', set: 'Base Set', price: '$1,800' },
    ];

    return (
      <div className="space-y-4">
        <div className="mb-4 p-3 bg-[var(--surface-1)] rounded-lg border border-[var(--surface-0)]">
          <p className="text-sm font-semibold">
            {selected.size} card{selected.size !== 1 ? 's' : ''} selected
          </p>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            Click cards to add/remove from comparison
          </p>
        </div>
        <div className="grid grid-cols-3 gap-4">
          {cards.map(card => (
            <CardShell
              key={card.id}
              variant="interactive"
              selectable
              isSelected={selected.has(card.id)}
              onToggleSelect={() => toggleCard(card.id)}
              ariaLabel={`${card.title} card`}
            >
              <h3 className="text-sm font-semibold mb-1">{card.title}</h3>
              <p className="text-xs text-[var(--text-muted)] mb-2">{card.set}</p>
              <p className="text-sm font-bold text-[var(--success)]">{card.price}</p>
            </CardShell>
          ))}
        </div>
      </div>
    );
  },
};

/**
 * All variants comparison
 */
export const AllVariants: Story = {
  render: () => (
    <div className="grid grid-cols-2 gap-6 w-[800px]">
      <CardShell variant="default">
        <p className="text-sm font-semibold">Default</p>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          --surface-1 + --shadow-1
        </p>
      </CardShell>
      <CardShell variant="elevated">
        <p className="text-sm font-semibold">Elevated</p>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          --surface-2 + --shadow-2
        </p>
      </CardShell>
      <CardShell variant="interactive" onClick={() => {}}>
        <p className="text-sm font-semibold">Interactive</p>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          Hover effects + focus ring
        </p>
      </CardShell>
      <CardShell variant="premium">
        <p className="text-sm font-semibold">Premium</p>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          Gradient + brand glow
        </p>
      </CardShell>
    </div>
  ),
};

/**
 * Accessibility showcase - Keyboard navigation
 */
export const KeyboardNavigation: Story = {
  render: function KeyboardNavigationDemo() {
    const [clickedCard, setClickedCard] = useState<string | null>(null);

    return (
      <div className="space-y-4">
        <div className="p-4 bg-[var(--info-bg)] border border-[var(--info-border)] rounded-lg">
          <p className="text-sm font-semibold mb-2">♿ Accessibility Features</p>
          <ul className="text-xs text-[var(--text-muted)] space-y-1">
            <li>• Use Tab to navigate between cards</li>
            <li>• Press Enter or Space to activate</li>
            <li>• Focus ring visible for keyboard users</li>
            <li>• ARIA labels for screen readers</li>
          </ul>
          {clickedCard && (
            <p className="text-sm font-semibold text-[var(--success)] mt-3">
              ✓ Activated: {clickedCard}
            </p>
          )}
        </div>
        <div className="grid grid-cols-3 gap-4">
          {['Card 1', 'Card 2', 'Card 3'].map(name => (
            <CardShell
              key={name}
              variant="interactive"
              onClick={() => setClickedCard(name)}
              ariaLabel={`${name} - Click to activate`}
            >
              <p className="text-sm font-semibold">{name}</p>
              <p className="text-xs text-[var(--text-muted)] mt-1">
                Tab + Enter/Space
              </p>
            </CardShell>
          ))}
        </div>
      </div>
    );
  },
};

/**
 * Complex card example with real content
 */
export const ComplexExample: Story = {
  render: function ComplexCard() {
    const [isSelected, setIsSelected] = useState(false);

    return (
      <CardShell
        variant="interactive"
        padding="none"
        selectable
        isSelected={isSelected}
        onToggleSelect={() => setIsSelected(!isSelected)}
        className="w-80"
      >
        {/* Card Image Header */}
        <div className="relative bg-gradient-to-br from-[var(--brand-700)] to-[var(--brand-900)] h-48 flex items-center justify-center">
          <div className="text-6xl">🔥</div>
          {/* Selection Checkbox */}
          <div className="absolute top-3 left-3 w-6 h-6 rounded border-2 border-white bg-white/20 backdrop-blur-sm flex items-center justify-center">
            {isSelected && (
              <svg className="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
              </svg>
            )}
          </div>
          {/* Rating Badge */}
          <div className="absolute top-3 right-3">
            <span className="text-xs font-semibold px-2 py-0.5 rounded-full bg-[var(--success-bg)] text-[var(--success)] border border-[var(--success-border)]">★ 4.5</span>
          </div>
        </div>

        {/* Card Content */}
        <div className="p-4">
          <h3 className="text-lg font-bold mb-1">Charizard</h3>
          <p className="text-sm text-[var(--text-muted)] mb-3">Base Set #4/102</p>

          {/* Profit Display */}
          <div className="bg-[var(--success-bg)] border-2 border-[var(--success-border)] rounded-[var(--radius-md)] p-3 mb-3">
            <p className="text-xs text-[var(--text-muted)] uppercase tracking-wide mb-1">
              💰 Expected Outcome Range
            </p>
            <p className="text-2xl font-bold text-[var(--success)]">$4,850</p>
            <p className="text-xs text-[var(--text-muted)] mt-1">in ~30 days (≈$162/month)</p>
          </div>

          {/* Investment Summary */}
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-[var(--text-muted)]">Investment:</span>
              <span className="font-semibold">$150</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--text-muted)]">Expected Sale:</span>
              <span className="font-semibold text-[var(--success)]">$5,000</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--text-muted)]">ROI:</span>
              <span className="font-bold text-[var(--success)]">3,233%</span>
            </div>
          </div>
        </div>
      </CardShell>
    );
  },
};

/**
 * Design token showcase - All colors used
 */
export const DesignTokens: Story = {
  render: () => (
    <div className="space-y-6 w-[700px]">
      <div>
        <h3 className="text-lg font-semibold mb-4">Surface Tokens</h3>
        <div className="grid grid-cols-3 gap-4">
          <div className="bg-[var(--surface-0)] border border-[var(--text-subtle)] rounded-lg p-4">
            <p className="text-sm font-semibold">--surface-0</p>
            <p className="text-xs text-[var(--text-muted)]">Flat surface</p>
          </div>
          <div className="bg-[var(--surface-1)] border border-[var(--text-subtle)] rounded-lg p-4">
            <p className="text-sm font-semibold">--surface-1</p>
            <p className="text-xs text-[var(--text-muted)]">Base card</p>
          </div>
          <div className="bg-[var(--surface-2)] border border-[var(--text-subtle)] rounded-lg p-4">
            <p className="text-sm font-semibold">--surface-2</p>
            <p className="text-xs text-[var(--text-muted)]">Elevated</p>
          </div>
        </div>
      </div>

      <div>
        <h3 className="text-lg font-semibold mb-4">Text Tokens</h3>
        <div className="bg-[var(--surface-1)] rounded-lg p-4 space-y-2">
          <p className="text-[var(--text)] text-sm">Primary text (--text)</p>
          <p className="text-[var(--text-muted)] text-sm">Muted text (--text-muted)</p>
          <p className="text-[var(--text-subtle)] text-sm">Subtle text (--text-subtle)</p>
        </div>
      </div>

      <div>
        <h3 className="text-lg font-semibold mb-4">Semantic Color Tokens</h3>
        <div className="grid grid-cols-2 gap-4">
          <div className="bg-[var(--success-bg)] border-2 border-[var(--success-border)] rounded-lg p-3">
            <p className="text-sm font-semibold text-[var(--success)]">Success</p>
          </div>
          <div className="bg-[var(--warning-bg)] border-2 border-[var(--warning-border)] rounded-lg p-3">
            <p className="text-sm font-semibold text-[var(--warning)]">Warning</p>
          </div>
          <div className="bg-[var(--danger-bg)] border-2 border-[var(--danger-border)] rounded-lg p-3">
            <p className="text-sm font-semibold text-[var(--danger)]">Danger</p>
          </div>
          <div className="bg-[var(--info-bg)] border-2 border-[var(--info-border)] rounded-lg p-3">
            <p className="text-sm font-semibold text-[var(--info)]">Info</p>
          </div>
        </div>
      </div>
    </div>
  ),
};
