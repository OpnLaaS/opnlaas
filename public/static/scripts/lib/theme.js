function setTheme(darkMode = true) {
    document.documentElement.classList[darkMode ? "add" : "remove"]("dark");
    localStorage.setItem("theme", darkMode ? "dark" : "light");
    
    const themeIcon = document.getElementById("theme-icon");
    if (themeIcon) { 
        themeIcon.textContent = darkMode ? "☀️" : "🌙";
    }

    const logoImage = document.getElementById("logo-image");
    if (logoImage) {
        logoImage.src = darkMode ? "/static/img/logo_dark.png" : "/static/img/logo_light.png";
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
        const el = document.getElementById("dashboard");
        if (el) { 
            el.classList.add("bg-gray-200", "dark:bg-white/20");
        }
    
    } else if (currentPath === "/") {
        const el = document.getElementById("home");
        if (el) { 
            el.classList.add("bg-gray-200", "dark:bg-white/20");
        }
        
    } else if (currentPath.includes("/login")) {
        const el = document.getElementById("login");
        if (el) { 
            el.classList.add("bg-gray-200", "dark:bg-white/20");
        }
    }
}