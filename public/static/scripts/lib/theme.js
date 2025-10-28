function setTheme(darkMode = true) {
    document.documentElement.classList[darkMode ? "add" : "remove"]("dark");
    localStorage.setItem("theme", darkMode ? "dark" : "light");

    const themeIcon = document.getElementById("theme-icon");
    themeIcon.textContent = darkMode ? "☀️" : "🌙";
}

export default function initTheming() {
    if (localStorage.getItem("theme")) {
        setTheme(localStorage.getItem("theme") === "dark");
    } else {
        setTheme(window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches);
    }

    const toggle = document.getElementById("theme-toggle");
    toggle.addEventListener("click", () => setTheme(!document.documentElement.classList.contains("dark")));

    const yearEl = document.getElementById("year");
    if (yearEl) {
        yearEl.textContent = new Date().getFullYear().toString();
    }
}