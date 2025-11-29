function setTheme(darkMode = true) {
    document.documentElement.classList[darkMode ? "add" : "remove"]("dark");
    localStorage.setItem("theme", darkMode ? "dark" : "light");
    
    const themeIcon = document.getElementById("theme-icon");
    if (themeIcon) {
        themeIcon.textContent = darkMode ? "☀️" : "🌙";
    }

    const logoImage = document.getElementById("logo-image");
    if (logoImage) {
        logoImage.src = darkMode ? "/static/img/logo-dark.svg" : "/static/img/logo.svg";
    }
}

export function initTheming() {
    if (localStorage.getItem("theme")) {
        setTheme(localStorage.getItem("theme") === "dark");
    } else {
        setTheme(window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches);
    }

    const toggle = document.getElementById("theme-toggle");
    if (toggle) {
        toggle.addEventListener("click", () => setTheme(!document.documentElement.classList.contains("dark")));
    }

    const yearEl = document.getElementById("year");
    if (yearEl) {
        yearEl.textContent = new Date().getFullYear().toString();
    }
    changeDashboard();
}

export function changeDashboard() {
    const currentPath = location.pathname;
    const id = currentPath === "/" ? "home" : currentPath.split("/")[1];
    const navButton = document.getElementById(id);
    
    if (navButton) {
        navButton.classList.add("bg-background-muted");
    }
}