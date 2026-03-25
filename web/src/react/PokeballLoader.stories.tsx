import type { Meta, StoryObj } from '@storybook/react';
import PokeballLoader from './PokeballLoader';

/**
 * PokeballLoader is a Pokemon-themed loading indicator.
 *
 * ## Features
 * - Animated spinning Pokeball
 * - Optional loading message
 * - Centered layout
 * - Smooth animations
 */
const meta: Meta<typeof PokeballLoader> = {
  title: 'UI/PokeballLoader',
  component: PokeballLoader,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
  argTypes: {
    text: {
      control: 'text',
      description: 'Loading message to display',
    },
  },
};

export default meta;
type Story = StoryObj<typeof PokeballLoader>;

/**
 * Default loader without message
 */
export const Default: Story = {
  args: {},
};

/**
 * With loading message
 */
export const WithMessage: Story = {
  args: {
    text: 'Loading opportunities...',
  },
};

/**
 * Different messages
 */
export const DifferentMessages: Story = {
  render: () => (
    <div className="space-y-8">
      <PokeballLoader text="Analyzing cards..." />
      <PokeballLoader text="Fetching prices..." />
      <PokeballLoader text="Loading collection..." />
    </div>
  ),
};

/**
 * In page context
 */
export const InPageContext: Story = {
  render: () => (
    <div className="min-h-screen bg-white dark:bg-gray-900 flex items-center justify-center">
      <PokeballLoader text="Loading your Pokemon collection..." />
    </div>
  ),
  parameters: {
    layout: 'fullscreen',
  },
};

/**
 * In card context
 */
export const InCardContext: Story = {
  render: () => (
    <div className="bg-white dark:bg-gray-800 rounded-lg p-8 border border-gray-200 dark:border-gray-700 w-96">
      <PokeballLoader text="Calculating ROI..." />
    </div>
  ),
};

/**
 * Long message
 */
export const LongMessage: Story = {
  args: {
    text: 'Please wait while we analyze thousands of Pokemon cards to find the best grading opportunities...',
  },
};
