const plugin = require('tailwindcss/plugin');

/**
 * Custom Tailwind plugin for SlabLedger design system
 *
 * This plugin adds custom component classes that are used frequently
 * and can't be easily composed from utility classes alone.
 */
module.exports = plugin(function({ addComponents, theme }) {
  // Add custom component classes
  addComponents({
    // Grade badge components (Pokemon TCG specific)
    '.grade-badge': {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      padding: `${theme('spacing.1')} ${theme('spacing.2')}`,
      fontSize: theme('fontSize.xs[0]'),
      fontWeight: theme('fontWeight.bold'),
      borderRadius: theme('borderRadius.full'),
      textTransform: 'uppercase',
      letterSpacing: '0.05em',
    },
    '.grade-badge-psa10': {
      backgroundColor: theme('colors.grade.psa10'),
      color: theme('colors.gray.900'),
      border: `2px solid ${theme('colors.grade.psa10')}`,
    },
    '.grade-badge-psa9': {
      backgroundColor: theme('colors.grade.psa9'),
      color: theme('colors.gray.900'),
      border: `2px solid ${theme('colors.grade.psa9')}`,
    },
    '.grade-badge-bgs': {
      backgroundColor: theme('colors.grade.bgs'),
      color: theme('colors.white'),
      border: `2px solid ${theme('colors.grade.bgs')}`,
    },
    '.grade-badge-cgc': {
      backgroundColor: theme('colors.grade.cgc'),
      color: theme('colors.white'),
      border: `2px solid ${theme('colors.grade.cgc')}`,
    },

    // Card glow effect (used for featured cards)
    '.card-glow': {
      position: 'relative',
      '&::before': {
        content: '""',
        position: 'absolute',
        inset: '-2px',
        borderRadius: 'inherit',
        background: 'linear-gradient(45deg, #ffd700, #ffed4e, #ffd700)',
        opacity: '0',
        transition: 'opacity 0.3s ease',
        zIndex: '-1',
      },
      '&:hover::before': {
        opacity: '0.3',
      },
    },

    // Glassmorphism effect
    '.glass': {
      backgroundColor: 'rgba(255, 255, 255, 0.1)',
      backdropFilter: 'blur(10px)',
      border: '1px solid rgba(255, 255, 255, 0.2)',
    },
    '.glass-dark': {
      backgroundColor: 'rgba(0, 0, 0, 0.3)',
      backdropFilter: 'blur(10px)',
      border: '1px solid rgba(255, 255, 255, 0.1)',
    },

    // Scrollbar styling
    '.scrollbar-thin': {
      scrollbarWidth: 'thin',
      scrollbarColor: `${theme('colors.gray.400')} ${theme('colors.gray.100')}`,
      '&::-webkit-scrollbar': {
        width: '8px',
        height: '8px',
      },
      '&::-webkit-scrollbar-track': {
        backgroundColor: theme('colors.gray.100'),
        borderRadius: theme('borderRadius.lg'),
      },
      '&::-webkit-scrollbar-thumb': {
        backgroundColor: theme('colors.gray.400'),
        borderRadius: theme('borderRadius.lg'),
        '&:hover': {
          backgroundColor: theme('colors.gray.500'),
        },
      },
    },

    // Container with constrained max-width and padding
    '.container-custom': {
      width: '100%',
      marginLeft: 'auto',
      marginRight: 'auto',
      paddingLeft: theme('spacing.4'),
      paddingRight: theme('spacing.4'),
      maxWidth: '1280px',
      '@media (min-width: 640px)': {
        paddingLeft: theme('spacing.6'),
        paddingRight: theme('spacing.6'),
      },
      '@media (min-width: 1024px)': {
        paddingLeft: theme('spacing.8'),
        paddingRight: theme('spacing.8'),
      },
    },

    // Focus ring that respects reduced motion
    '.focus-ring': {
      outline: 'none',
      '&:focus-visible': {
        boxShadow: theme('boxShadow.focus'),
        transition: 'box-shadow 0.15s ease',
      },
      '@media (prefers-reduced-motion: reduce)': {
        '&:focus-visible': {
          transition: 'none',
        },
      },
    },

    // Truncate text with ellipsis (multi-line support)
    '.line-clamp-2': {
      display: '-webkit-box',
      '-webkit-line-clamp': '2',
      '-webkit-box-orient': 'vertical',
      overflow: 'hidden',
    },
    '.line-clamp-3': {
      display: '-webkit-box',
      '-webkit-line-clamp': '3',
      '-webkit-box-orient': 'vertical',
      overflow: 'hidden',
    },

    // Shimmer loading effect
    '.shimmer': {
      background: 'linear-gradient(90deg, #f0f0f0 25%, #e0e0e0 50%, #f0f0f0 75%)',
      backgroundSize: '200% 100%',
      animation: 'shimmer 1.5s infinite',
    },
    '@keyframes shimmer': {
      '0%': {
        backgroundPosition: '200% 0',
      },
      '100%': {
        backgroundPosition: '-200% 0',
      },
    },
  });
});
