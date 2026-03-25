import type { Meta, StoryObj } from '@storybook/react';
import Button from './Button';

/**
 * Button component with multiple variants, sizes, and states.
 *
 * ## Features
 * - Multiple variants (primary, secondary, success, danger, warning, ghost, link)
 * - Sizes (sm, md, lg, icon)
 * - Loading state
 * - Icon support (left/right positioning)
 * - Full width option
 * - WCAG-compliant minimum sizes
 */
const meta: Meta<typeof Button> = {
  title: 'UI/Button',
  component: Button,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['primary', 'secondary', 'success', 'danger', 'warning', 'ghost', 'link'],
      description: 'Visual style variant',
    },
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg', 'icon'],
      description: 'Button size',
    },
    loading: {
      control: 'boolean',
      description: 'Show loading spinner',
    },
    disabled: {
      control: 'boolean',
      description: 'Disable button',
    },
    fullWidth: {
      control: 'boolean',
      description: 'Full width button',
    },
  },
};

export default meta;
type Story = StoryObj<typeof Button>;

/**
 * Primary button - main call to action
 */
export const Primary: Story = {
  args: {
    children: 'Primary Button',
    variant: 'primary',
  },
};

/**
 * Secondary button - secondary actions
 */
export const Secondary: Story = {
  args: {
    children: 'Secondary Button',
    variant: 'secondary',
  },
};

/**
 * Success button - confirmation actions
 */
export const Success: Story = {
  args: {
    children: 'Success Button',
    variant: 'success',
  },
};

/**
 * Danger button - destructive actions
 */
export const Danger: Story = {
  args: {
    children: 'Danger Button',
    variant: 'danger',
  },
};

/**
 * Warning button - caution actions
 */
export const Warning: Story = {
  args: {
    children: 'Warning Button',
    variant: 'warning',
  },
};

/**
 * Ghost button - subtle actions
 */
export const Ghost: Story = {
  args: {
    children: 'Ghost Button',
    variant: 'ghost',
  },
};

/**
 * Link button - text-only actions
 */
export const Link: Story = {
  args: {
    children: 'Link Button',
    variant: 'link',
  },
};

/**
 * Small size button
 */
export const Small: Story = {
  args: {
    children: 'Small Button',
    size: 'sm',
  },
};

/**
 * Medium size button (default)
 */
export const Medium: Story = {
  args: {
    children: 'Medium Button',
    size: 'md',
  },
};

/**
 * Large size button
 */
export const Large: Story = {
  args: {
    children: 'Large Button',
    size: 'lg',
  },
};

/**
 * Icon-only button
 */
export const IconOnly: Story = {
  args: {
    size: 'icon',
    icon: <span>🔍</span>,
    'aria-label': 'Search',
  },
};

/**
 * Loading state
 */
export const Loading: Story = {
  args: {
    children: 'Loading...',
    loading: true,
  },
};

/**
 * Disabled state
 */
export const Disabled: Story = {
  args: {
    children: 'Disabled Button',
    disabled: true,
  },
};

/**
 * Button with left icon
 */
export const WithLeftIcon: Story = {
  args: {
    children: 'Download',
    icon: <span>⬇️</span>,
    iconPosition: 'left',
  },
};

/**
 * Button with right icon
 */
export const WithRightIcon: Story = {
  args: {
    children: 'Next',
    icon: <span>➡️</span>,
    iconPosition: 'right',
  },
};

/**
 * Full width button
 */
export const FullWidth: Story = {
  args: {
    children: 'Full Width Button',
    fullWidth: true,
  },
  decorators: [
    (Story) => (
      <div className="w-64">
        <Story />
      </div>
    ),
  ],
};

/**
 * All variants showcase
 */
export const AllVariants: Story = {
  render: () => (
    <div className="flex flex-col gap-3">
      <Button variant="primary">Primary</Button>
      <Button variant="secondary">Secondary</Button>
      <Button variant="success">Success</Button>
      <Button variant="danger">Danger</Button>
      <Button variant="warning">Warning</Button>
      <Button variant="ghost">Ghost</Button>
      <Button variant="link">Link</Button>
    </div>
  ),
};

/**
 * All sizes showcase
 */
export const AllSizes: Story = {
  render: () => (
    <div className="flex items-center gap-3">
      <Button size="sm">Small</Button>
      <Button size="md">Medium</Button>
      <Button size="lg">Large</Button>
      <Button size="icon" icon={<span>🔍</span>} aria-label="Search" />
    </div>
  ),
};

/**
 * All states showcase
 */
export const AllStates: Story = {
  render: () => (
    <div className="flex flex-col gap-3">
      <Button>Normal</Button>
      <Button loading>Loading</Button>
      <Button disabled>Disabled</Button>
    </div>
  ),
};
