/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./src/**/*.{html,js,jsx,ts,tsx}"],
  darkMode: "class",
  theme: {
    extend: {
      // Accessibility: Reduced motion is handled globally in design-system.css
      // Use motion-safe: and motion-reduce: variants for granular control
      // Example: motion-safe:animate-fadeIn motion-reduce:transition-none
      colors: {
        // Primary palette
        primary: {
          DEFAULT: '#2563eb',
          hover: '#1d4ed8',
          light: '#dbeafe',
          dark: '#1e40af',
        },
        // Success palette
        success: {
          DEFAULT: '#059669',
          hover: '#047857',
          light: '#d1fae5',
          text: '#065f46',
          bg: '#d1fae5',
          border: '#059669',
        },
        // Warning palette
        warning: {
          DEFAULT: '#d97706',
          hover: '#b45309',
          light: '#fef3c7',
          text: '#92400e',
          bg: '#fef3c7',
          border: '#d97706',
        },
        // Danger palette
        danger: {
          DEFAULT: '#dc2626',
          hover: '#b91c1c',
          light: '#fee2e2',
          text: '#991b1b',
          bg: '#fee2e2',
          border: '#dc2626',
        },
        // Info palette
        info: {
          DEFAULT: '#0891b2',
          hover: '#0e7490',
          light: '#cffafe',
          text: '#164e63',
          bg: '#cffafe',
          border: '#0891b2',
        },
        // Text colors
        text: {
          primary: '#111827',
          secondary: '#4b5563',
          tertiary: '#6b7280',
          inverse: '#ffffff',
          link: '#2563eb',
          'link-hover': '#1d4ed8',
        },
        // Background colors
        bg: {
          primary: '#fefefe',
          secondary: '#f9fafb',
          tertiary: '#f3f4f6',
          elevated: '#ffffff',
          // Legacy aliases for backwards compatibility
          DEFAULT: '#fefefe',
          surface: '#f9fafb',
          raised: '#ffffff',
        },
        // Surface colors
        surface: {
          raised: '#ffffff',
          sunken: '#f3f4f6',
          overlay: 'rgba(0, 0, 0, 0.5)',
        },
        // Border colors
        border: {
          DEFAULT: '#e5e7eb',
          hover: '#d1d5db',
          focus: '#2563eb',
          error: '#dc2626',
        },
        // Grade colors (Pokemon TCG specific)
        grade: {
          psa10: '#ffd700',
          psa9: '#c0c0c0',
          bgs: '#00a8ff',
          cgc: '#ff6b35',
        },
        // Legacy brand colors for backwards compatibility
        brand: {
          start: "#2563eb",
          end: "#60a5fa",
          warn: "#f59e0b",
          success: "#10b981",
          danger: "#ef4444",
        },
      },
      // Spacing extensions
      spacing: {
        '18': '4.5rem',
        '88': '22rem',
        '112': '28rem',
        '128': '32rem',
      },
      // Typography extensions
      fontSize: {
        '2xs': ['0.625rem', { lineHeight: '0.75rem' }],
        '3xl': ['1.875rem', { lineHeight: '2.25rem' }],
        '4xl': ['2.25rem', { lineHeight: '2.5rem' }],
        '5xl': ['3rem', { lineHeight: '1' }],
      },
      fontWeight: {
        medium: '500',
        semibold: '600',
        bold: '700',
        extrabold: '800',
      },
      // Border radius extensions
      borderRadius: {
        'xs': '0.125rem',
        'sm': '0.25rem',
        DEFAULT: '0.375rem',
        'md': '0.5rem',
        'lg': '0.75rem',
        'xl': '1rem',
        '2xl': '1.5rem',
        '3xl': '2rem',
        // Legacy alias
        'xl2': '1rem',
      },
      // Shadow extensions - Enhanced for modern UI
      boxShadow: {
        'xs': '0 1px 2px 0 rgba(0, 0, 0, 0.05)',
        'sm': '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
        DEFAULT: '0 2px 4px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
        'md': '0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)',
        'lg': '0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05)',
        'xl': '0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04)',
        '2xl': '0 25px 50px -12px rgba(0, 0, 0, 0.25)',
        'inner': 'inset 0 2px 4px 0 rgba(0, 0, 0, 0.06)',
        'focus': '0 0 0 3px rgba(37, 99, 235, 0.5)',
        // Modern card shadows with depth
        'card': '0 2px 8px rgba(0, 0, 0, 0.08), 0 1px 2px rgba(0, 0, 0, 0.05)',
        'card-hover': '0 12px 24px rgba(0, 0, 0, 0.15), 0 4px 8px rgba(0, 0, 0, 0.08)',
        'card-elevated': '0 8px 16px rgba(0, 0, 0, 0.12), 0 2px 4px rgba(0, 0, 0, 0.06)',
        // Glassmorphism effect
        'glass': '0 8px 32px 0 rgba(31, 38, 135, 0.15)',
        // Glow effects for premium cards
        'glow-sm': '0 0 10px rgba(37, 99, 235, 0.3)',
        'glow-md': '0 0 20px rgba(37, 99, 235, 0.4)',
        'glow-lg': '0 0 30px rgba(37, 99, 235, 0.5)',
        'glow-gold': '0 0 20px rgba(255, 215, 0, 0.5)',
        // Legacy aliases
        'soft': '0 8px 24px rgba(0,0,0,0.25)',
        'inset': 'inset 0 1px 0 rgba(255,255,255,0.03)',
      },
      backgroundImage: {
        // Pokemon-themed gradients
        // Usage: bg-gradient-pikachu, bg-gradient-charizard, etc.
        'gradient-brand': 'linear-gradient(135deg, #2563eb 0%, #60a5fa 100%)',
        'gradient-pikachu': 'linear-gradient(135deg, #FFD700 0%, #FFA500 100%)',
        'gradient-charizard': 'linear-gradient(135deg, #FF3C00 0%, #FFD700 100%)',
        'gradient-lugia': 'linear-gradient(135deg, #2563eb 0%, #60a5fa 100%)',
        // Glassmorphism gradient overlays
        'gradient-glass-light': 'linear-gradient(135deg, rgba(255, 255, 255, 0.1) 0%, rgba(255, 255, 255, 0.05) 100%)',
        'gradient-glass-dark': 'linear-gradient(135deg, rgba(255, 255, 255, 0.15) 0%, rgba(255, 255, 255, 0.05) 100%)',
        // Shimmer gradient for loading states
        'shimmer': 'linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.2), transparent)',
      },
      // Backdrop blur for glassmorphism
      backdropBlur: {
        xs: '2px',
        sm: '4px',
        DEFAULT: '8px',
        md: '12px',
        lg: '16px',
        xl: '24px',
        '2xl': '40px',
        '3xl': '64px',
      },
      keyframes: {
        // Fade animations
        fadeIn: {
          "0%": { opacity: "0" },
          "100%": { opacity: "1" },
        },
        fadeOut: {
          "0%": { opacity: "1" },
          "100%": { opacity: "0" },
        },
        fadeInUp: {
          "0%": { opacity: "0", transform: "translateY(10px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
        fadeInDown: {
          "0%": { opacity: "0", transform: "translateY(-10px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
        // Scale animations
        pop: {
          "0%": { transform: "scale(.99)" },
          "100%": { transform: "scale(1)" },
        },
        scaleIn: {
          "0%": { transform: "scale(.95)", opacity: "0" },
          "100%": { transform: "scale(1)", opacity: "1" },
        },
        // Glow animations
        "pulse-glow": {
          "0%, 100%": { filter: "drop-shadow(0 0 8px rgba(255, 215, 0, 0.5))" },
          "50%": { filter: "drop-shadow(0 0 16px rgba(255, 215, 0, 0.8))" },
        },
        "pulse-glow-blue": {
          "0%, 100%": { boxShadow: "0 0 10px rgba(37, 99, 235, 0.3)" },
          "50%": { boxShadow: "0 0 20px rgba(37, 99, 235, 0.5)" },
        },
        // Shimmer effect for loading states
        shimmer: {
          "0%": { backgroundPosition: "-1000px 0" },
          "100%": { backgroundPosition: "1000px 0" },
        },
        // Bounce effect for notifications
        bounceIn: {
          "0%": { transform: "scale(.3)", opacity: "0" },
          "50%": { transform: "scale(1.05)" },
          "70%": { transform: "scale(.9)" },
          "100%": { transform: "scale(1)", opacity: "1" },
        },
        // Slide animations
        slideInRight: {
          "0%": { transform: "translateX(100%)", opacity: "0" },
          "100%": { transform: "translateX(0)", opacity: "1" },
        },
        slideInLeft: {
          "0%": { transform: "translateX(-100%)", opacity: "0" },
          "100%": { transform: "translateX(0)", opacity: "1" },
        },
      },
      animation: {
        // Basic
        fadeIn: "fadeIn .25s ease-out",
        fadeOut: "fadeOut .2s ease-out",
        fadeInUp: "fadeInUp .4s ease-out",
        fadeInDown: "fadeInDown .4s ease-out",
        pop: "pop .15s ease-out",
        scaleIn: "scaleIn .3s ease-out",
        // Glow
        "pulse-glow": "pulse-glow 2s ease-in-out infinite",
        "pulse-glow-blue": "pulse-glow-blue 2s ease-in-out infinite",
        // Loading
        shimmer: "shimmer 2s linear infinite",
        // Notifications
        bounceIn: "bounceIn .6s cubic-bezier(0.68, -0.55, 0.265, 1.55)",
        // Slides
        slideInRight: "slideInRight .3s ease-out",
        slideInLeft: "slideInLeft .3s ease-out",
      },
    },
  },
  plugins: [
    require('./tailwind-plugins/design-system'),
  ],
};
