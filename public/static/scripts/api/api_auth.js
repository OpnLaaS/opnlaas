import { apiPostGeneric, apiPostJSON } from "./util.js";

const known_uri = {
    login: () => "/api/auth/login?",
    logout: () => "/api/auth/logout"
};

export async function postLogin(username, password) {

    const formData = new FormData();
    formData.append("username", username);
    formData.append("password", password);

    return await apiPostGeneric(
        known_uri.login() + new URLSearchParams("no_redirect=1"), 
        formData,
    );
}

export async function postLogout() {
    return await apiPostJSON(known_uri.logout());
}