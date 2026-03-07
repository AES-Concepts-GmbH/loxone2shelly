/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        'shelly-blue': '#4A90D9',
        'shelly-dark': '#1a1f2e',
      }
    }
  },
  plugins: []
}
