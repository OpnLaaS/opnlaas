import { apiDelete, apiGet, apiPostJSON, known_uri } from "./util.js";

export async function getBookingsMine() {
    return await apiGet(known_uri.bookings_mine());
}

export async function getBookingByID(bookingId) {
    return await apiGet(known_uri.bookings_bookingByID(bookingId));
}

export async function getBookingProvisioningStatus(bookingId) {
    return await apiGet(known_uri.bookings_provisioningByID(bookingId));
}

export async function cancelBookingProvisioning(bookingId) {
    return await apiPostJSON(
        known_uri.bookings_provisioningCancelByID(bookingId),
        JSON.stringify({}),
    );
}

export async function deleteBookingByID(bookingId) {
    return await apiDelete(known_uri.bookings_bookingByID(bookingId));
}

export async function deployBooking(payload) {
    return await apiPostJSON(
        known_uri.bookings_deploy(),
        JSON.stringify(payload),
    );
}

export async function getBookingNetworkPrefill() {
    return await apiGet(known_uri.bookings_networkPrefill());
}

export async function getBookingCart() {
    return await apiGet(known_uri.bookings_cart());
}

export async function setBookingCartNetwork(payload) {
    return await apiPostJSON(
        known_uri.bookings_cartNetwork(),
        JSON.stringify(payload),
    );
}

export async function getAvailableCartHosts() {
    return await apiGet(known_uri.bookings_availableCartHosts());
}

export async function addHostToCart(managementIpOrPayload, isoSelection) {
    const payload = typeof managementIpOrPayload === "object" && managementIpOrPayload !== null
        ? managementIpOrPayload
        : {
            management_ip: managementIpOrPayload,
            iso_selection: isoSelection || "",
        };

    return await apiPostJSON(
        known_uri.bookings_cartHosts(),
        JSON.stringify(payload),
    );
}

export async function removeHostFromCart(managementIp) {
    return await apiDelete(known_uri.bookings_cartHostByIP(managementIp));
}
