import { URL } from "../lib/constants.js";

/**
 * This should not be used outside of the api code, 
 *  if an endpoint is added to the backed write a corresponding wrapper.
 * 
 * All functions follow the same IO structure:
 * @param {*} URI - The URI endpoint to hit
 * @param {*} params - Given params in the form of an object* (most of the time)
 * @returns {
 *  status_code: - HTTP Status code returned
 *  body: the response body (JSON), can be null 
 * }
 */

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

// To use when the type of the sent content is pre-defined and NOT JSON 
//  (ie FormData for authentication)
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
