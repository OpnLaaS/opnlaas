import { initTheming, changeDashboard } from "./lib/theme.js";
import { getCurrentUser, postLogout } from "./api/api.js";

changeDashboard();
initTheming();

const userChip = document.getElementById("user-chip");
const userDropdown = document.getElementById("user-dropdown");
const navUserName = document.getElementById("navUserName");
const navUserNameMenu = document.getElementById("navUserNameMenu");
const navUserFullName = document.getElementById("navUserFullName");
const navUserEmail = document.getElementById("navUserEmail");
const navUserRole = document.getElementById("navUserRole");
const logoutBtnNav = document.getElementById("logoutBtnNav");

async function hydrateNavUser() {
    if (!userChip) return;
    try {
        const res = await getCurrentUser();
        if (!res || res.status_code !== 200) return;

        const profile = res.body || {};
        const name =  profile.username || profile.display_name || "User";
        const email = profile.email || profile.username || "";
        const role = profile.permissions
            ? `${profile.permissions.charAt(0).toUpperCase()}${profile.permissions.slice(1)}`
            : (profile.is_admin ? "Administrator" : "User");

        navUserName && (navUserName.textContent = name);
        navUserNameMenu && (navUserNameMenu.textContent = name);
        navUserFullName && (navUserFullName.textContent = profile.display_name || "");
        navUserEmail && (navUserEmail.textContent = email);
        navUserRole && (navUserRole.textContent = role);
    } catch (err) {
        console.error(err);
    }
}

function closeUserDropdown() {
    if (userDropdown) {
        userDropdown.classList.add("hidden");
    }
}

function toggleUserDropdown() {
    if (userDropdown) {
        userDropdown.classList.toggle("hidden");
    }
}

if (userChip) {
    userChip.addEventListener("click", (e) => {
        e.stopPropagation();
        toggleUserDropdown();
    });

    document.addEventListener("click", (e) => {
        if (!userDropdown) return;
        if (userDropdown.contains(e.target) || userChip.contains(e.target)) return;
        closeUserDropdown();
    });
}

if (logoutBtnNav) {
    logoutBtnNav.addEventListener("click", async () => {
        logoutBtnNav.disabled = true;
        logoutBtnNav.textContent = "Logging out...";
        try {
            await postLogout();
        } finally {
            window.location.href = "/login";
        }
    });
}

hydrateNavUser();
