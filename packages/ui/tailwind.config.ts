import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: '#080d13',
        ring: '#f6b26b',
        glow: '#ffd7a3'
      },
      keyframes: {
        pulseRing: {
          '0%, 100%': { transform: 'scale(0.95)', opacity: '0.8' },
          '50%': { transform: 'scale(1.06)', opacity: '1' }
        }
      },
      animation: {
        pulseRing: 'pulseRing 1.8s ease-in-out infinite'
      }
    }
  },
  plugins: []
} satisfies Config
