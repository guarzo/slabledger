import type { Meta, StoryObj } from '@storybook/react';
import Input from './Input';

/**
 * Input component for text entry with labels, errors, and helper text.
 *
 * ## Features
 * - Label support with optional required indicator
 * - Error state with error messages
 * - Helper text for guidance
 * - Multiple sizes (sm, md, lg)
 * - Full form integration
 * - Accessible with proper ARIA attributes
 */
const meta: Meta<typeof Input> = {
  title: 'UI/Input',
  component: Input,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    state: {
      control: 'select',
      options: ['default', 'error', 'success'],
      description: 'Input state',
    },
  },
};

export default meta;
type Story = StoryObj<typeof Input>;

/**
 * Default input
 */
export const Default: Story = {
  args: {
    placeholder: 'Enter text...',
  },
};

/**
 * Input with label
 */
export const WithLabel: Story = {
  args: {
    label: 'Email Address',
    placeholder: 'you@example.com',
    type: 'email',
  },
};

/**
 * Required input
 */
export const Required: Story = {
  args: {
    label: 'Username',
    placeholder: 'Enter username',
    required: true,
  },
};

/**
 * Input with helper text
 */
export const WithHelper: Story = {
  args: {
    label: 'Password',
    type: 'password',
    placeholder: 'Enter password',
    helper: 'Must be at least 8 characters',
  },
};

/**
 * Error state
 */
export const WithError: Story = {
  args: {
    label: 'Email',
    type: 'email',
    placeholder: 'you@example.com',
    error: 'Please enter a valid email address',
  },
};

/**
 * Success state
 */
export const Success: Story = {
  args: {
    label: 'Username',
    value: 'johndoe',
    state: 'success',
  },
};

/**
 * Disabled state
 */
export const Disabled: Story = {
  args: {
    label: 'Disabled Input',
    value: 'Cannot edit this',
    disabled: true,
  },
};

/**
 * Small size
 */
export const Small: Story = {
  args: {
    label: 'Small Input',
    placeholder: 'Small size',
  },
};

/**
 * Medium size (default)
 */
export const Medium: Story = {
  args: {
    label: 'Medium Input',
    placeholder: 'Medium size',
  },
};

/**
 * Large size
 */
export const Large: Story = {
  args: {
    label: 'Large Input',
    placeholder: 'Large size',
  },
};

/**
 * Search input
 */
export const Search: Story = {
  args: {
    type: 'search',
    placeholder: 'Search cards...',
    'aria-label': 'Search',
  },
};

/**
 * Number input
 */
export const Number: Story = {
  args: {
    type: 'number',
    label: 'Price',
    placeholder: '0.00',
    helper: 'Enter price in USD',
  },
};

/**
 * All sizes showcase
 */
export const AllSizes: Story = {
  render: () => (
    <div className="space-y-4 w-80">
      <Input label="Small" placeholder="Small input" />
      <Input label="Medium" placeholder="Medium input" />
      <Input label="Large" placeholder="Large input" />
    </div>
  ),
};

/**
 * All states showcase
 */
export const AllStates: Story = {
  render: () => (
    <div className="space-y-4 w-80">
      <Input label="Default" placeholder="Default state" />
      <Input label="Success" value="Valid input" state="success" />
      <Input label="Error" error="This field is required" />
      <Input label="Disabled" value="Cannot edit" disabled />
    </div>
  ),
};

/**
 * Form example
 */
export const FormExample: Story = {
  render: () => (
    <div className="space-y-4 w-96">
      <Input
        label="Full Name"
        placeholder="John Doe"
        required
      />
      <Input
        label="Email Address"
        type="email"
        placeholder="you@example.com"
        helper="We'll never share your email"
        required
      />
      <Input
        label="Phone Number"
        type="tel"
        placeholder="(555) 123-4567"
      />
      <Input
        label="Min ROI %"
        type="number"
        placeholder="0"
        helper="Minimum return on investment percentage"
      />
    </div>
  ),
};
