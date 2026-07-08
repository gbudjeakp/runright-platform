/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        cream:  { DEFAULT: '#FBF0DC', alt: '#F3E2C2' },
        paper:  '#FFFDF7',
        ink:    { DEFAULT: '#2C1A0E', mid: '#6B4226', light: '#9A7B5A' },
        red:    { DEFAULT: '#C23B22', dark: '#9B2D17' },
        gold:   { DEFAULT: '#B8860B', light: '#D4A82A' },
        navy:   '#1B3361',
        border: { DEFAULT: '#D4B896', dark: '#B8946A' },
        dark: {
          base:        '#130C05',
          alt:         '#1E1209',
          paper:       '#231810',
          border:      '#3D2810',
          'border-hi': '#5A3A18',
          sidebar:     '#0A0603',
        },
      },
      fontFamily: {
        sans:  ['Lato', 'system-ui', 'sans-serif'],
        serif: ['"Playfair Display"', 'Georgia', 'serif'],
        deco:  ['"Bebas Neue"', 'Impact', 'sans-serif'],
        mono:  ['ui-monospace', 'Consolas', '"Courier New"', 'monospace'],
      },
      boxShadow: {
        rr:    '3px 3px 0 rgba(92,58,30,.18)',
        'rr-lg': '6px 6px 0 rgba(92,58,30,.14)',
      },
    },
  },
  plugins: [],
}

