// Date time formatting
// MM/DD/YYYY 24HR:MM
export const dateTimeFormat = new Intl.DateTimeFormat("en-US", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
});

export function validateIP(address) {
    return /^(?!0)(?!.*\.$)((1?\d?\d|25[0-5]|2[0-4]\d)(\.|$)){4}$/.test(address);
}

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

export function reverseObject(obj) {
    return Object.fromEntries(Object.entries(obj).map(([k, v]) => [v, k]));
}