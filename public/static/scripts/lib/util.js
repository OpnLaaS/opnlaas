export async function fetchText(url) {
    return await (await fetch(url, {
        cache: "no-cache"
    })).text();
}

export async function fetchJSON(url) {
    return await (await fetch(url, {
        cache: "no-cache"
    })).json();
}

export function isExternalURL(url) {
    try {
        const link = new URL(url, window.location.href);
        return link.origin !== window.location.origin;
    } catch {
        return false;
    }
}