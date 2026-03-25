import type { Preview } from '@storybook/react-vite';
import '../src/css/base.css';
import React from 'react';

const preview: Preview = {
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
    backgrounds: {
      default: 'light',
      values: [
        {
          name: 'light',
          value: '#fefefe',
        },
        {
          name: 'dark',
          value: '#0f172a',
        },
      ],
    },
    layout: 'centered',
  },
  decorators: [
    (Story) => (
      <div className="p-4">
        <Story />
      </div>
    ),
  ],
};

export default preview;