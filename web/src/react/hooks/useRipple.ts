/**
 * useRipple Hook
 *
 * Creates Material Design-style ripple effect on buttons and interactive elements.
 * Automatically handles cleanup and respects prefers-reduced-motion.
 */
import { useCallback } from 'react';

interface RippleOptions {
  /** Ripple color (CSS color value) */
  color?: string;
  /** Ripple duration in milliseconds */
  duration?: number;
  /** Whether to respect prefers-reduced-motion */
  respectMotionPreference?: boolean;
}

/**
 * Check if user prefers reduced motion
 */
function prefersReducedMotion(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/**
 * useRipple - Material Design ripple effect
 *
 * @example
 * ```tsx
 * function Button({ children, onClick }: ButtonProps) {
 *   const createRipple = useRipple({ color: 'rgba(255, 255, 255, 0.6)' });
 *
 *   const handleClick = (e: React.MouseEvent<HTMLButtonElement>) => {
 *     createRipple(e);
 *     onClick?.(e);
 *   };
 *
 *   return (
 *     <button
 *       className="relative overflow-hidden"
 *       onClick={handleClick}
 *     >
 *       {children}
 *     </button>
 *   );
 * }
 * ```
 */
export function useRipple(options: RippleOptions = {}) {
  const {
    color = 'rgba(255, 255, 255, 0.6)',
    duration = 600,
    respectMotionPreference = true,
  } = options;

  const createRipple = useCallback(
    (event: React.MouseEvent<HTMLElement>) => {
      // Skip if user prefers reduced motion
      if (respectMotionPreference && prefersReducedMotion()) {
        return;
      }

      const button = event.currentTarget;

      // Remove any existing ripples
      const existingRipples = button.querySelectorAll('.ripple-effect');
      existingRipples.forEach((ripple) => ripple.remove());

      // Create ripple element
      const circle = document.createElement('span');
      const diameter = Math.max(button.clientWidth, button.clientHeight);
      const radius = diameter / 2;

      // Position ripple at click location
      const rect = button.getBoundingClientRect();
      const x = event.clientX - rect.left - radius;
      const y = event.clientY - rect.top - radius;

      // Apply styles
      Object.assign(circle.style, {
        width: `${diameter}px`,
        height: `${diameter}px`,
        left: `${x}px`,
        top: `${y}px`,
        position: 'absolute',
        borderRadius: '50%',
        background: color,
        transform: 'scale(0)',
        animation: `ripple-animation ${duration}ms ease-out`,
        pointerEvents: 'none',
        zIndex: '0',
      });

      circle.classList.add('ripple-effect');

      // Add to DOM
      button.appendChild(circle);

      // Remove after animation completes
      setTimeout(() => {
        circle.remove();
      }, duration);
    },
    [color, duration, respectMotionPreference]
  );

  return createRipple;
}

/**
 * CSS keyframes for ripple animation (should be in global CSS)
 *
 * Add this to your global CSS file:
 *
 * ```css
 * @keyframes ripple-animation {
 *   to {
 *     transform: scale(4);
 *     opacity: 0;
 *   }
 * }
 * ```
 */

export default useRipple;
