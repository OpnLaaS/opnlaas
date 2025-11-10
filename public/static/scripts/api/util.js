import { URL } from "../lib/constants.js"

export async function apiGet(URI, params) {
    let url = URL + URI;

    let response =  await (fetch (
        url + new URLSearchParams(params).toString(), {
            credentials: 'include',
        }
    ));

    try {
        var body = await response.json();
    } catch {
        var body = {};
    }

    return {
        status_code: response.status,
        body: body
    };
}

export async function apiPostJSON(URI, params) {
    let url = URL + URI;

    let response =  await (fetch(
        url, {
            credentials: 'include',
            method: "POST",
            headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/json'
            },
            body: params,
        }
    ));

    try {
        var body = await response.json();
    } catch {
        var body = {};
    }

    return {
        status_code: response.status,
        body: body
    };
}

// To use when the type of the sent content is defined (ie FormData)
export async function apiPostGeneric(URI, params) {
    let url = URL + URI;

    let response =  await (fetch(
        url, {
            credentials: 'include',
            method: "POST",
            body: params,
        }
    ));

    try {
        var body = await response.json();
    } catch {
        var body = {};
    }

    return {
        status_code: response.status,
        body: body
    };
}

export async function apiDelete(URI, params) {
    let url = URL + URI;

    let response = await (fetch(
        url, {
            credentials: 'include',
            method: "DELETE",
            headers: {
            'Accept': 'application/json',
            'Content-Type': 'application/json'
            },
            body: params
        }
    ));

    try {
        var body = await response.json();
    } catch {
        var body = {};
    }

    return {
        status_code: response.status,
        body: body
    };
}
