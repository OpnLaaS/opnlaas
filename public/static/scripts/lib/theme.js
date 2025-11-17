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
    var currentPath = location.pathname;
    console.log("Current location:", currentPath);
    
    if (currentPath.includes("/dashboard")) {
        const navButton = document.getElementById("dashboard");
        if (navButton) {
            navButton.classList.add("bg-background-muted");
        }
    
    } else if (currentPath === "/") {
        const navButton = document.getElementById("home");
        if (navButton) {
            navButton.classList.add("bg-background-muted");
        }
        
    } else if (currentPath.includes("/login")) {
        const navButton = document.getElementById("login");
        if (navButton) {
            navButton.classList.add("bg-background-muted");
        }
    }
}