import type { Meta, StoryObj } from '@storybook/react';
import Select from './Select';

/**
 * Select component for dropdown selections.
 *
 * ## Features
 * - Label support with optional required indicator
 * - Error state with error messages
 * - Helper text for guidance
 * - Multiple sizes (sm, md, lg)
 * - Accessible with proper ARIA attributes
 */
const meta: Meta<typeof Select> = {
  title: 'UI/Select',
  component: Select,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    state: {
      control: 'select',
      options: ['default', 'error', 'success'],
      description: 'Select state',
    },
  },
};

export default meta;
type Story = StoryObj<typeof Select>;

/**
 * Default select
 */
export const Default: Story = {
  args: {
    children: (
      <>
        <option value="">Select an option...</option>
        <option value="1">Option 1</option>
        <option value="2">Option 2</option>
        <option value="3">Option 3</option>
      </>
    ),
  },
};

/**
 * Select with label
 */
export const WithLabel: Story = {
  args: {
    label: 'Pokemon Set',
    children: (
      <>
        <option value="">Choose a set...</option>
        <option value="base">Base Set</option>
        <option value="jungle">Jungle</option>
        <option value="fossil">Fossil</option>
        <option value="team-rocket">Team Rocket</option>
      </>
    ),
  },
};

/**
 * Required select
 */
export const Required: Story = {
  args: {
    label: 'Grading Company',
    required: true,
    children: (
      <>
        <option value="">Select company...</option>
        <option value="psa">PSA</option>
        <option value="bgs">BGS</option>
        <option value="cgc">CGC</option>
      </>
    ),
  },
};

/**
 * Select with helper text
 */
export const WithHelper: Story = {
  args: {
    label: 'Sort By',
    helper: 'Choose how to sort the results',
    children: (
      <>
        <option value="roi">ROI (High to Low)</option>
        <option value="price">Price (Low to High)</option>
        <option value="name">Name (A-Z)</option>
      </>
    ),
  },
};

/**
 * Error state
 */
export const WithError: Story = {
  args: {
    label: 'Grade',
    error: 'Please select a grade',
    children: (
      <>
        <option value="">Select grade...</option>
        <option value="10">PSA 10</option>
        <option value="9">PSA 9</option>
        <option value="8">PSA 8</option>
      </>
    ),
  },
};

/**
 * Success state
 */
export const Success: Story = {
  args: {
    label: 'Category',
    state: 'success',
    children: (
      <>
        <option value="">Select category...</option>
        <option value="pokemon" selected>Pokemon</option>
        <option value="trainer">Trainer</option>
        <option value="energy">Energy</option>
      </>
    ),
  },
};

/**
 * Disabled state
 */
export const Disabled: Story = {
  args: {
    label: 'Disabled Select',
    disabled: true,
    children: (
      <>
        <option value="1">Option 1</option>
        <option value="2">Option 2</option>
      </>
    ),
  },
};

/**
 * Small size
 */
export const Small: Story = {
  args: {
    label: 'Small Select',
    children: (
      <>
        <option value="1">Small option</option>
        <option value="2">Another option</option>
      </>
    ),
  },
};

/**
 * Large size
 */
export const Large: Story = {
  args: {
    label: 'Large Select',
    children: (
      <>
        <option value="1">Large option</option>
        <option value="2">Another option</option>
      </>
    ),
  },
};

/**
 * All sizes showcase
 */
export const AllSizes: Story = {
  render: () => (
    <div className="space-y-4 w-80">
      <Select label="Small">
        <option>Small select</option>
      </Select>
      <Select label="Medium">
        <option>Medium select</option>
      </Select>
      <Select label="Large">
        <option>Large select</option>
      </Select>
    </div>
  ),
};

/**
 * Filter example
 */
export const FilterExample: Story = {
  render: () => (
    <div className="space-y-4 w-96">
      <Select label="Pokemon Set" helper="Filter by set">
        <option value="">All Sets</option>
        <option value="base">Base Set</option>
        <option value="jungle">Jungle</option>
        <option value="fossil">Fossil</option>
        <option value="team-rocket">Team Rocket</option>
        <option value="gym-heroes">Gym Heroes</option>
      </Select>
      <Select label="Grade" helper="Filter by minimum grade">
        <option value="">Any Grade</option>
        <option value="10">PSA 10</option>
        <option value="9">PSA 9+</option>
        <option value="8">PSA 8+</option>
      </Select>
      <Select label="Liquidity">
        <option value="">Any Liquidity</option>
        <option value="high">High</option>
        <option value="medium">Medium</option>
        <option value="low">Low</option>
      </Select>
    </div>
  ),
};
