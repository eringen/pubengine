/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: "class",
  content: [
    "./views/**/*.templ",
    "./main.go",
    "./public/**/*.js",
  ],
  theme: {
    extend: {
      colors: {
        ink: "#0f0f0f",
        accent: "#ffcb47",
      },
    },
  },
  plugins: [require("@tailwindcss/typography")],
};
