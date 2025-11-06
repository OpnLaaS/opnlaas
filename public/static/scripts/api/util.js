import { URL } from "../lib/constants.js"

export async function apiGet(URI, params) {
    let url = URL + URI;

    let response =  await (fetch (
        url + new URLSearchParams(params).toString()
    ));

    return {
        status_code: response.status,
        body: await response.json()     
    };
}

export async function apiPost(URI, params) {
    let url = URL + URI;

    let response =  await (fetch(
        url, {
            method: "POST",
            headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/json'
            },
            body: params
        }
    ));

    return {
        status_code: response.status,
        body: await response.json()
    };
}

export async function apiDelete(URI, params) {
    let url = URL + URI;

    let response = await (fetch(
        url, {
            method: "DELETE",
            headers: {
            'Accept': 'application/json',
            'Content-Type': 'application/json'
            },
            body: params
        }
    ));

    return {
        status_code: response.status,
        body: await response.json()
    };
}

// DELETE
// window.apiPost = apiPost;
// window.apiGet = apiGet;
// window.apiDelete = apiDelete;
