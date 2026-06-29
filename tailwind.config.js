/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./index.html", "./src/**/*.{js,jsx}"],
  theme: {
    extend: {
      colors: {
        bg: "var(--bg)",
        bg2: "var(--bg2)",
        surface: "var(--surface)",
        "surface-raised": "var(--surface-raised)",
        line: "var(--line)",
        "line-strong": "var(--line-strong)",
        text: "var(--text)",
        muted: "var(--muted)",
        faint: "var(--faint)",
        primary: "var(--primary)",
        "primary-dim": "var(--primary-dim)",
        secondary: "var(--secondary)",
        success: "var(--success)",
        warning: "var(--warning)",
        danger: "var(--danger)",
        "not-comparable": "var(--not-comparable)",
        g0: "var(--g0)",
        g1: "var(--g1)",
        g2: "var(--g2)",
        g3: "var(--g3)",
        g4: "var(--g4)",
        g5: "var(--g5)",
      },
      fontFamily: {
        mono: [
          "ui-monospace",
          "SF Mono",
          "JetBrains Mono",
          "Cascadia Code",
          "Menlo",
          "Consolas",
          "monospace",
        ],
      },
    },
  },
  plugins: [],
};
