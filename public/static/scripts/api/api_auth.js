import { apiPostGeneric, apiPostJSON, known_uri } from "./util.js";

export async function postLogin(username, password) {

    const formData = new FormData();
    formData.append("username", username);
    formData.append("password", password);

    return await apiPostGeneric(
        known_uri.auth_login() + new URLSearchParams("no_redirect=1"), 
        formData,
    );
}

export async function postLogout() {
    return await apiPostJSON(known_uri.auth_logout());
}