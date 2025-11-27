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

export const known_uri = {
    auth_login: () => "/api/auth/login?",
    auth_me: () => "/api/auth/me",
    auth_logout: () => "/api/auth/logout",
    enum_vendors: () => "/api/enums/vendors",
    enum_formFactors: () => "/api/enums/form-factors",
    enum_managementTypes: () => "/api/enums/management-types",
    enum_powerStates: () => "/api/enums/power-states",
    enum_bootModes: () => "/api/enums/boot-modes",
    enum_powerActions: () => "/api/enums/power-actions",
    enum_architectures: () => "/api/enums/architectures",
    enum_distroTypes: () => "/api/enums/architectures",
    enum_preconfigureTypes: () => "/api/enums/preconfigure-types",
    enum_bookingPermissionLevels: () => "/api/enums/booking-permission-levels",
    enum_bookingStatuses: () => "/api/enums/booking-statuses",
    hosts_hosts: () => "/api/hosts",
    hosts_hostByManagementIP: (management_ip) => `${known_uri.hosts_hosts()}/${management_ip}`,
    hosts_hostPowerAction: (management_ip, power_action) => `${known_uri.hosts_hostByManagementIP(management_ip)}/power/${power_action}`,
    iso_images: () => "/api/iso-images",
};

export async function apiGet(URI, params) {
    let url = URL + URI;

    let response = await (fetch(
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

    let response = await (fetch(
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

    let response = await (fetch(
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
