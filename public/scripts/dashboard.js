import * as API from "./api/api.js";
import { dateTimeFormat, reverseObject } from "./lib/util.js";
import { dismissToasts, showErrorToast, showInfoToast } from "./lib/toast.js";

const state = {
    bookingStatusNames: {},
    powerActionNames: {},
    powerStateNames: {},
    isos: [],
    availableHosts: [],
    cart: { hosts: {}, network_cidr: "", gateway_ipv4: "", dns_servers: [] },
    bookings: [],
    provisioningByBooking: new Map(),
    bookingNetworkPrefill: null,
    wizardStage: 1,
    hostConfigTargetIP: "",
    selectedProvisioningBookingID: 0,
    provisioningPollTimer: null,
};

const refs = {
    refreshBookingsBtn: document.getElementById("refreshBookingsBtn"),
    openDeployWizardBtn: document.getElementById("openDeployWizardBtn"),
    openProvisioningLogBtn: document.getElementById("openProvisioningLogBtn"),
    bookingsEmpty: document.getElementById("bookingsEmpty"),
    bookingsList: document.getElementById("bookingsList"),
    bookingCardTemplate: document.getElementById("bookingCardTemplate"),
    bookingHostRowTemplate: document.getElementById("bookingHostRowTemplate"),
    deployWizardModal: document.getElementById("deployWizardModal"),
    wizardStageChip1: document.getElementById("wizardStageChip1"),
    wizardStageChip2: document.getElementById("wizardStageChip2"),
    wizardStageChip3: document.getElementById("wizardStageChip3"),
    wizardStage1: document.getElementById("wizardStage1"),
    wizardStage2: document.getElementById("wizardStage2"),
    wizardStage3: document.getElementById("wizardStage3"),
    wizardBackBtn: document.getElementById("wizardBackBtn"),
    wizardNextBtn: document.getElementById("wizardNextBtn"),
    wizardSubmitBtn: document.getElementById("wizardSubmitBtn"),
    wizardSubmitSpinner: document.getElementById("wizardSubmitSpinner"),
    wizardRefreshHostsBtn: document.getElementById("wizardRefreshHostsBtn"),
    wizardBookingName: document.getElementById("wizardBookingName"),
    wizardBookingDescription: document.getElementById("wizardBookingDescription"),
    wizardBookingDuration: document.getElementById("wizardBookingDuration"),
    wizardBookingCIDR: document.getElementById("wizardBookingCIDR"),
    wizardBookingCIDRHint: document.getElementById("wizardBookingCIDRHint"),
    wizardBootMode: document.getElementById("wizardBootMode"),
    wizardSelectedCount: document.getElementById("wizardSelectedCount"),
    wizardAvailableHostsEmpty: document.getElementById("wizardAvailableHostsEmpty"),
    wizardAvailableHostsList: document.getElementById("wizardAvailableHostsList"),
    wizardHostCardTemplate: document.getElementById("wizardHostCardTemplate"),
    wizardSelectedHostsEmpty: document.getElementById("wizardSelectedHostsEmpty"),
    wizardSelectedHostsList: document.getElementById("wizardSelectedHostsList"),
    wizardSelectedHostRowTemplate: document.getElementById("wizardSelectedHostRowTemplate"),
    wizardReviewMeta: document.getElementById("wizardReviewMeta"),
    wizardReviewHostsEmpty: document.getElementById("wizardReviewHostsEmpty"),
    wizardReviewHostsList: document.getElementById("wizardReviewHostsList"),
    hostConfigModal: document.getElementById("hostConfigModal"),
    hostConfigTargetLabel: document.getElementById("hostConfigTargetLabel"),
    hostCfgISO: document.getElementById("hostCfgISO"),
    hostCfgTimezone: document.getElementById("hostCfgTimezone"),
    hostCfgLocale: document.getElementById("hostCfgLocale"),
    hostCfgKeyboard: document.getElementById("hostCfgKeyboard"),
    hostCfgMirror: document.getElementById("hostCfgMirror"),
    hostCfgPackages: document.getElementById("hostCfgPackages"),
    hostCfgAssignedIP: document.getElementById("hostCfgAssignedIP"),
    hostCfgAssignedIPHint: document.getElementById("hostCfgAssignedIPHint"),
    hostCfgLateScript: document.getElementById("hostCfgLateScript"),
    hostCfgSaveBtn: document.getElementById("hostCfgSaveBtn"),
    provisioningLogModal: document.getElementById("provisioningLogModal"),
    provisioningBookingSelect: document.getElementById("provisioningBookingSelect"),
    provisioningRefreshBtn: document.getElementById("provisioningRefreshBtn"),
    provisioningSummary: document.getElementById("provisioningSummary"),
    provisioningHostsEmpty: document.getElementById("provisioningHostsEmpty"),
    provisioningHostsList: document.getElementById("provisioningHostsList"),
    provisioningHostRowTemplate: document.getElementById("provisioningHostRowTemplate"),
    provisioningEventsEmpty: document.getElementById("provisioningEventsEmpty"),
    provisioningEventsList: document.getElementById("provisioningEventsList"),
    provisioningEventRowTemplate: document.getElementById("provisioningEventRowTemplate"),
};

function messageFromResponse(response, fallback) {
    return response?.body?.message || fallback;
}

function showError(message) {
    dismissToasts("error");
    showErrorToast(message);
}

function hideError() {
    dismissToasts("error");
}

function showInfo(message) {
    dismissToasts("info");
    showInfoToast(message);
}

function hideInfo() {
    dismissToasts("info");
}

function formatTime(value) {
    if (!value) return "N/A";
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return "N/A";
    return dateTimeFormat.format(parsed);
}

function bookingStatusLabel(value) {
    const reverse = reverseObject(state.bookingStatusNames || {});
    return reverse[value] || `Status ${value}`;
}

function powerStateLabel(value) {
    const reverse = reverseObject(state.powerStateNames || {});
    return reverse[value] || "Unknown";
}

function parseCSV(raw) {
    return (raw || "")
        .split(",")
        .map((part) => part.trim())
        .filter((part) => part.length > 0);
}

function ipToInt(ip) {
    const parts = `${ip || ""}`.trim().split(".");
    if (parts.length !== 4) return null;
    const octets = parts.map((p) => Number(p));
    if (octets.some((n) => !Number.isInteger(n) || n < 0 || n > 255)) return null;
    return ((octets[0] << 24) >>> 0) + ((octets[1] << 16) >>> 0) + ((octets[2] << 8) >>> 0) + (octets[3] >>> 0);
}

function intToIP(value) {
    return [
        (value >>> 24) & 255,
        (value >>> 16) & 255,
        (value >>> 8) & 255,
        value & 255,
    ].join(".");
}

function parseCIDR(cidr) {
    const [ip, prefixRaw] = `${cidr || ""}`.trim().split("/");
    const prefix = Number(prefixRaw);
    const ipInt = ipToInt(ip);
    if (ipInt === null || !Number.isInteger(prefix) || prefix < 1 || prefix > 30) return null;

    const hostBits = 32 - prefix;
    const blockSize = 2 ** hostBits;
    const networkInt = Math.floor(ipInt / blockSize) * blockSize;
    const broadcastInt = networkInt + blockSize - 1;
    return { prefix, networkInt, broadcastInt, blockSize };
}

function normalizeIP(ip) {
    const value = ipToInt(ip);
    if (value === null) return "";
    return intToIP(value);
}

function cidrContainsIP(cidrInfo, ip) {
    if (!cidrInfo) return false;
    const value = ipToInt(ip);
    if (value === null) return false;
    return value >= cidrInfo.networkInt && value <= cidrInfo.broadcastInt;
}

function templatePackagesSummary(templateData) {
    const packages = templateData?.["template.packages"] || "";
    return packages.trim() ? packages : "none";
}

function getCartHostMap() {
    return state.cart?.hosts || {};
}

function cartHostsArray() {
    return Object.values(getCartHostMap()).sort((a, b) => (a.management_ip || "").localeCompare(b.management_ip || ""));
}

function setModalOpen(modalEl, open) {
    if (!modalEl) return;
    modalEl.classList.toggle("hidden", !open);
}

function setWizardSubmitPending(active) {
    refs.wizardSubmitBtn.disabled = !!active;
    refs.wizardSubmitSpinner.classList.toggle("hidden", !active);
}

function setStageChipActive(chip, active) {
    if (!chip) return;
    chip.classList.toggle("bg-background-secondary", !!active);
    chip.classList.toggle("border-accent-primary", !!active);
    chip.classList.toggle("text-font-primary", !!active);
}

function updateWizardStageUI() {
    const stage = state.wizardStage;
    refs.wizardStage1.classList.toggle("hidden", stage !== 1);
    refs.wizardStage2.classList.toggle("hidden", stage !== 2);
    refs.wizardStage3.classList.toggle("hidden", stage !== 3);

    setStageChipActive(refs.wizardStageChip1, stage === 1);
    setStageChipActive(refs.wizardStageChip2, stage === 2);
    setStageChipActive(refs.wizardStageChip3, stage === 3);

    refs.wizardBackBtn.classList.toggle("invisible", stage === 1);
    refs.wizardNextBtn.classList.toggle("hidden", stage === 3);
    refs.wizardSubmitBtn.classList.toggle("hidden", stage !== 3);
}

function buildISOOptions(selectEl, selectedISO = "") {
    selectEl.textContent = "";
    const defaultOption = document.createElement("option");
    defaultOption.value = "";
    defaultOption.textContent = "Use PXE default ISO";
    selectEl.appendChild(defaultOption);

    for (const iso of state.isos) {
        const option = document.createElement("option");
        option.value = iso.name;
        option.textContent = iso.name;
        if (selectedISO && selectedISO === iso.name) {
            option.selected = true;
        }
        selectEl.appendChild(option);
    }
}

async function loadEnums() {
    const [bookingStatusesRes, powerActionsRes, powerStatesRes] = await Promise.all([
        API.getBookingStatuses(),
        API.getPowerActions(),
        API.getPowerStates(),
    ]);
    state.bookingStatusNames = bookingStatusesRes?.status_code === 200 ? bookingStatusesRes.body : {};
    state.powerActionNames = powerActionsRes?.status_code === 200 ? powerActionsRes.body : {};
    state.powerStateNames = powerStatesRes?.status_code === 200 ? powerStatesRes.body : {};
}

async function loadISOImages() {
    const response = await API.getIsoImages();
    if (response.status_code !== 200) {
        throw new Error(messageFromResponse(response, "Failed to load ISO images"));
    }
    state.isos = Array.isArray(response.body) ? response.body : [];
    state.isos.sort((a, b) => (a?.name || "").localeCompare(b?.name || ""));
}

async function loadBookingNetworkPrefill() {
    const response = await API.getBookingNetworkPrefill();
    if (response.status_code !== 200) {
        throw new Error(messageFromResponse(response, "Failed to load booking network defaults"));
    }
    state.bookingNetworkPrefill = response.body || null;
}

async function loadAvailableHosts() {
    const response = await API.getAvailableCartHosts();
    if (response.status_code !== 200) {
        throw new Error(messageFromResponse(response, "Failed to load available hosts"));
    }
    state.availableHosts = Array.isArray(response.body) ? response.body : [];
    state.availableHosts.sort((a, b) => (a.management_ip || "").localeCompare(b.management_ip || ""));
}

async function loadCart() {
    const response = await API.getBookingCart();
    if (response.status_code === 404) {
        state.cart = { hosts: {}, network_cidr: "", gateway_ipv4: "", dns_servers: [] };
        return;
    }
    if (response.status_code !== 200) {
        throw new Error(messageFromResponse(response, "Failed to load cart"));
    }
    state.cart = response.body || { hosts: {}, network_cidr: "", gateway_ipv4: "", dns_servers: [] };
}

async function loadBookings() {
    const response = await API.getBookingsMine();
    if (response.status_code !== 200) {
        throw new Error(messageFromResponse(response, "Failed to load bookings"));
    }
    state.bookings = Array.isArray(response.body) ? response.body : [];
}

function powerActionsOrdered() {
    return Object.entries(state.powerActionNames || {})
        .map(([label, value]) => ({ label, value }))
        .sort((a, b) => a.label.localeCompare(b.label));
}

async function runHostPowerAction(bookingID, managementIP, powerAction, buttonEl) {
    if (!powerAction) return;

    const previous = buttonEl.textContent;
    buttonEl.disabled = true;
    buttonEl.textContent = "Running...";
    hideError();
    hideInfo();

    try {
        const response = await API.postHostPowerControl(managementIP, powerAction);
        if (response.status_code !== 200 && response.status_code !== 202) {
            showError(messageFromResponse(response, `Power action failed for host ${managementIP}`));
            return;
        }
        showInfo(`Power action queued for ${managementIP}`);
        await refreshBookingByID(bookingID);
    } finally {
        buttonEl.disabled = false;
        buttonEl.textContent = previous;
    }
}

function renderBookingHostRows(card, bookingView) {
    const hostsList = card.querySelector('[data-field="hosts-list"]');
    const hostsEmpty = card.querySelector('[data-field="hosts-empty"]');
    hostsList.textContent = "";

    const hosts = Array.isArray(bookingView.hosts) ? bookingView.hosts : [];
    if (hosts.length === 0) {
        hostsEmpty.classList.remove("hidden");
        return;
    }
    hostsEmpty.classList.add("hidden");

    const powerActions = powerActionsOrdered();
    for (const host of hosts) {
        const frag = refs.bookingHostRowTemplate.content.cloneNode(true);
        frag.querySelector('[data-field="management_ip"]').textContent = host.management_ip;
        frag.querySelector('[data-field="state"]').textContent = `Power: ${powerStateLabel(host.last_known_power_state)} | Last update: ${formatTime(host.last_known_power_state_time)}`;
        frag.querySelector('[data-field="assigned_ip"]').textContent = `Assigned IP: ${host.assigned_ipv4 || "N/A"}${host.assigned_cidr ? ` (${host.assigned_cidr})` : ""}`;

        const actionSelect = frag.querySelector('[data-field="power-action"]');
        for (const item of powerActions) {
            const option = document.createElement("option");
            option.value = `${item.value}`;
            option.textContent = item.label;
            actionSelect.appendChild(option);
        }

        const runBtn = frag.querySelector('[data-action="run-power"]');
        runBtn.addEventListener("click", async () => {
            await runHostPowerAction(bookingView.booking.id, host.management_ip, actionSelect.value, runBtn);
        });

        hostsList.appendChild(frag);
    }
}

function renderBookings() {
    refs.bookingsList.textContent = "";
    if (!state.bookings.length) {
        refs.bookingsEmpty.classList.remove("hidden");
        return;
    }

    refs.bookingsEmpty.classList.add("hidden");
    for (const bookingView of state.bookings) {
        const booking = bookingView.booking || {};
        const frag = refs.bookingCardTemplate.content.cloneNode(true);
        const card = frag.querySelector("article");
        card.dataset.bookingId = `${booking.id}`;

        frag.querySelector('[data-field="name"]').textContent = booking.name || `Booking ${booking.id}`;
        frag.querySelector('[data-field="meta"]').textContent = `#${booking.id} | ${bookingStatusLabel(booking.status)} | Permission ${bookingView.permission_level}`;
        frag.querySelector('[data-field="window"]').textContent = `Start: ${formatTime(booking.start_time)} | End: ${formatTime(booking.end_time)}`;
        frag.querySelector('[data-field="given-user"]').textContent = bookingView.credentials?.given_user_username || "N/A";
        frag.querySelector('[data-field="given-pass"]').textContent = bookingView.credentials?.given_user_password || "N/A";
        frag.querySelector('[data-field="managed-user"]').textContent = bookingView.credentials?.managed_user_username || "N/A";
        frag.querySelector('[data-field="managed-pass"]').textContent = bookingView.credentials?.managed_user_password || "N/A";

        const destroyBtn = frag.querySelector('[data-action="destroy"]');
        destroyBtn.addEventListener("click", async () => {
            if (!window.confirm(`Destroy booking ${booking.id}?`)) {
                return;
            }

            destroyBtn.disabled = true;
            destroyBtn.textContent = "Destroying...";
            hideError();
            hideInfo();

            try {
                const response = await API.deleteBookingByID(booking.id);
                if (response.status_code !== 200) {
                    showError(messageFromResponse(response, `Failed to destroy booking ${booking.id}`));
                    return;
                }
                showInfo(`Booking ${booking.id} destroyed`);
                await refreshAll();
            } finally {
                destroyBtn.disabled = false;
                destroyBtn.textContent = "Destroy";
            }
        });

        renderBookingHostRows(card, bookingView);
        refs.bookingsList.appendChild(frag);
    }
}

function wizardHostSummary(host) {
    const iso = host.iso_selection || "PXE default";
    const timezone = host.template_data?.["template.timezone"] || "default";
    const locale = host.template_data?.["template.locale"] || "default";
    const assignedIP = host.assigned_ipv4 || "auto";
    return `ISO: ${iso} | IP: ${assignedIP} | TZ: ${timezone} | Locale: ${locale} | Packages: ${templatePackagesSummary(host.template_data)}`;
}

function renderWizardHosts() {
    const cartMap = getCartHostMap();
    const selectedHosts = cartHostsArray();
    refs.wizardSelectedCount.textContent = `${selectedHosts.length} selected`;

    refs.wizardAvailableHostsList.textContent = "";
    let availableCount = 0;
    for (const host of state.availableHosts) {
        if (cartMap[host.management_ip]) {
            continue;
        }
        availableCount += 1;
        const frag = refs.wizardHostCardTemplate.content.cloneNode(true);
        frag.querySelector('[data-field="management_ip"]').textContent = host.management_ip;
        frag.querySelector('[data-field="details"]').textContent = `${host.vendor ?? "Vendor?"} | ${host.model || "Model?"} | ${host.form_factor ?? "Form?"}`;
        frag.querySelector('[data-action="add-host"]').addEventListener("click", () => {
            openHostConfigModal(host.management_ip);
        });
        refs.wizardAvailableHostsList.appendChild(frag);
    }
    refs.wizardAvailableHostsEmpty.classList.toggle("hidden", availableCount > 0);

    refs.wizardSelectedHostsList.textContent = "";
    refs.wizardSelectedHostsEmpty.classList.toggle("hidden", selectedHosts.length > 0);
    for (const host of selectedHosts) {
        const frag = refs.wizardSelectedHostRowTemplate.content.cloneNode(true);
        frag.querySelector('[data-field="management_ip"]').textContent = host.management_ip;
        frag.querySelector('[data-field="summary"]').textContent = wizardHostSummary(host);

        frag.querySelector('[data-action="edit-host"]').addEventListener("click", () => {
            openHostConfigModal(host.management_ip);
        });
        frag.querySelector('[data-action="remove-host"]').addEventListener("click", async () => {
            const response = await API.removeHostFromCart(host.management_ip);
            if (response.status_code !== 200) {
                showError(messageFromResponse(response, `Failed to remove ${host.management_ip} from cart`));
                return;
            }
            await refreshWizardData();
        });
        refs.wizardSelectedHostsList.appendChild(frag);
    }
}

function renderWizardReview() {
    const name = refs.wizardBookingName.value.trim();
    const description = refs.wizardBookingDescription.value.trim() || "N/A";
    const duration = refs.wizardBookingDuration.value || "1";
    const bootMode = refs.wizardBootMode.value || "UEFI";
    refs.wizardReviewMeta.innerHTML = `
        <div><span class="text-font-secondary">Name:</span> ${name || "(missing)"}</div>
        <div><span class="text-font-secondary">Description:</span> ${description}</div>
        <div><span class="text-font-secondary">Duration:</span> ${duration} day(s)</div>
        <div><span class="text-font-secondary">Subnet:</span> ${refs.wizardBookingCIDR.value.trim() || "N/A"}</div>
        <div><span class="text-font-secondary">Boot Mode:</span> ${bootMode}</div>
    `;

    const selectedHosts = cartHostsArray();
    refs.wizardReviewHostsList.textContent = "";
    refs.wizardReviewHostsEmpty.classList.toggle("hidden", selectedHosts.length > 0);
    for (const host of selectedHosts) {
        const row = document.createElement("div");
        row.className = "rounded-lg border border-background-muted px-3 py-2 text-xs";
        row.textContent = `${host.management_ip} | ${wizardHostSummary(host)}`;
        refs.wizardReviewHostsList.appendChild(row);
    }
}

function renderBookingCIDRHint(message, isError = false) {
    if (!refs.wizardBookingCIDRHint) return;
    refs.wizardBookingCIDRHint.textContent = message || "";
    refs.wizardBookingCIDRHint.classList.toggle("text-red-300", !!isError);
    refs.wizardBookingCIDRHint.classList.toggle("text-font-secondary", !isError);
}

function currentCIDRInfo() {
    return parseCIDR(refs.wizardBookingCIDR?.value || "");
}

async function ensureCartNetworkAssigned(showErrorOnFailure = false) {
    const response = await API.setBookingCartNetwork({});
    if (response.status_code !== 200) {
        const message = messageFromResponse(response, "Failed to allocate booking subnet");
        renderBookingCIDRHint(message, true);
        if (showErrorOnFailure) showError(message);
        return false;
    }

    const canonicalCIDR = response.body?.network_cidr || "";
    if (!canonicalCIDR) {
        const message = "Booking subnet allocation returned no CIDR.";
        renderBookingCIDRHint(message, true);
        if (showErrorOnFailure) showError(message);
        return false;
    }

    refs.wizardBookingCIDR.value = canonicalCIDR;
    state.cart.network_cidr = canonicalCIDR;
    state.cart.gateway_ipv4 = response.body?.gateway_ipv4 || "";
    state.cart.dns_servers = Array.isArray(response.body?.dns_servers) ? response.body.dns_servers : [];
    const hostPrefix = Number(state.bookingNetworkPrefill?.host_network_prefix || 0);
    const hostPrefixText = hostPrefix > 0 ? ` | Host /${hostPrefix}` : "";
    renderBookingCIDRHint(`Gateway ${state.cart.gateway_ipv4 || "auto"}${hostPrefixText} | DNS ${(state.cart.dns_servers || []).join(", ") || "none"}`, false);
    renderWizardReview();
    return true;
}

function gatewayOffsetForCIDR(cidrInfo) {
    const gateway = normalizeIP(state.cart?.gateway_ipv4 || "");
    if (!gateway || !cidrInfo) return null;
    if (!cidrContainsIP(cidrInfo, gateway)) return null;

    const gatewayInt = ipToInt(gateway);
    if (gatewayInt === null) return null;
    return gatewayInt - cidrInfo.networkInt;
}

function assignedOffsetFromIP(cidrInfo, ip) {
    const normalized = normalizeIP(ip);
    if (!normalized || !cidrInfo) return null;

    const value = ipToInt(normalized);
    if (value === null) return null;
    return value - cidrInfo.networkInt;
}

function suggestHostAssignedOffset(managementIP) {
    const cidrInfo = currentCIDRInfo();
    if (!cidrInfo) return "";

    const used = new Set();
    for (const host of cartHostsArray()) {
        if (!host || host.management_ip === managementIP || !host.assigned_ipv4) continue;
        const offset = assignedOffsetFromIP(cidrInfo, host.assigned_ipv4);
        if (offset !== null) used.add(offset);
    }

    const gatewayOffset = gatewayOffsetForCIDR(cidrInfo);
    if (gatewayOffset !== null) used.add(gatewayOffset);

    const startOffset = Number(state.bookingNetworkPrefill?.host_start_offset || 10);
    for (let offset = Math.max(2, startOffset); offset < cidrInfo.blockSize - 1; offset += 1) {
        if (!used.has(offset)) return `${offset}`;
    }

    return "";
}

function validateAssignedHostBits(rawBits, managementIP) {
    const parsed = Number(rawBits);
    if (!Number.isInteger(parsed)) {
        return { ok: false, message: "Host bits must be a whole number.", bits: null };
    }

    const cidrInfo = currentCIDRInfo();
    if (!cidrInfo) {
        return { ok: false, message: "Booking subnet is not assigned yet.", bits: null };
    }

    if (parsed <= 0 || parsed >= cidrInfo.blockSize) {
        return { ok: false, message: `Host bits must be between 1 and ${cidrInfo.blockSize - 2}.`, bits: null };
    }
    if (parsed === 0 || parsed === cidrInfo.blockSize - 1) {
        return { ok: false, message: "Host bits cannot be network or broadcast.", bits: null };
    }

    const gatewayOffset = gatewayOffsetForCIDR(cidrInfo);
    if (gatewayOffset !== null && parsed === gatewayOffset) {
        return { ok: false, message: "Host bits conflict with booking gateway.", bits: null };
    }

    const candidateIP = intToIP(cidrInfo.networkInt + parsed);
    for (const host of cartHostsArray()) {
        if (!host || host.management_ip === managementIP || !host.assigned_ipv4) continue;
        if (normalizeIP(host.assigned_ipv4) === candidateIP) {
            return { ok: false, message: `Host bits ${parsed} already used by ${host.management_ip}.`, bits: null };
        }
    }

    return { ok: true, bits: parsed, ip: candidateIP, message: `Maps to ${candidateIP} in ${refs.wizardBookingCIDR.value.trim()}.` };
}

function updateHostAssignedIPHint() {
    const bits = refs.hostCfgAssignedIP.value.trim();
    if (!bits) {
        refs.hostCfgAssignedIPHint.textContent = "Leave empty to auto-assign next available host bits.";
        refs.hostCfgAssignedIPHint.classList.remove("text-red-300");
        refs.hostCfgSaveBtn.disabled = false;
        return { ok: true, bits: null, ip: "" };
    }

    const validation = validateAssignedHostBits(bits, state.hostConfigTargetIP);
    refs.hostCfgAssignedIPHint.textContent = validation.message || "";
    refs.hostCfgAssignedIPHint.classList.toggle("text-red-300", !validation.ok);
    refs.hostCfgSaveBtn.disabled = !validation.ok;
    return validation;
}

async function refreshWizardData() {
    await Promise.all([loadAvailableHosts(), loadCart()]);
    if (state.cart?.network_cidr) {
        refs.wizardBookingCIDR.value = state.cart.network_cidr;
    }
    if (state.cart?.network_cidr || state.cart?.gateway_ipv4) {
        const hostPrefix = Number(state.bookingNetworkPrefill?.host_network_prefix || 0);
        const hostPrefixText = hostPrefix > 0 ? ` | Host /${hostPrefix}` : "";
        renderBookingCIDRHint(`Gateway ${state.cart.gateway_ipv4 || "auto"}${hostPrefixText} | DNS ${(state.cart.dns_servers || []).join(", ") || "none"}`, false);
    }
    renderWizardHosts();
    renderWizardReview();
}

function clearHostConfigInputs() {
    refs.hostCfgTimezone.value = "";
    refs.hostCfgLocale.value = "";
    refs.hostCfgKeyboard.value = "";
    refs.hostCfgMirror.value = "";
    refs.hostCfgPackages.value = "";
    refs.hostCfgAssignedIP.value = "";
    refs.hostCfgAssignedIPHint.textContent = "";
    refs.hostCfgLateScript.value = "";
}

function templateDataFromHostConfigInputs() {
    const data = {};
    const timezone = refs.hostCfgTimezone.value.trim();
    const locale = refs.hostCfgLocale.value.trim();
    const keyboard = refs.hostCfgKeyboard.value.trim();
    const mirror = refs.hostCfgMirror.value.trim();
    const packages = parseCSV(refs.hostCfgPackages.value);
    const lateScript = refs.hostCfgLateScript.value.trim();

    if (timezone) data["template.timezone"] = timezone;
    if (locale) data["template.locale"] = locale;
    if (keyboard) data["template.keyboard_layout"] = keyboard;
    if (mirror) data["template.mirror"] = mirror;
    if (packages.length) data["template.packages"] = packages.join(",");
    if (lateScript) data["template.late_script"] = lateScript;

    return data;
}

function openHostConfigModal(managementIP) {
    const host = getCartHostMap()[managementIP];
    state.hostConfigTargetIP = managementIP;
    refs.hostConfigTargetLabel.textContent = managementIP;
    buildISOOptions(refs.hostCfgISO, host?.iso_selection || "");
    clearHostConfigInputs();

    if (host?.template_data) {
        refs.hostCfgTimezone.value = host.template_data["template.timezone"] || "";
        refs.hostCfgLocale.value = host.template_data["template.locale"] || "";
        refs.hostCfgKeyboard.value = host.template_data["template.keyboard_layout"] || "";
        refs.hostCfgMirror.value = host.template_data["template.mirror"] || "";
        refs.hostCfgPackages.value = host.template_data["template.packages"] || "";
        refs.hostCfgLateScript.value = host.template_data["template.late_script"] || "";
    }

    if (host?.assigned_ipv4) {
        const cidrInfo = currentCIDRInfo();
        const offset = assignedOffsetFromIP(cidrInfo, host.assigned_ipv4);
        refs.hostCfgAssignedIP.value = offset !== null ? `${offset}` : "";
    } else {
        refs.hostCfgAssignedIP.value = suggestHostAssignedOffset(managementIP);
    }
    updateHostAssignedIPHint();

    setModalOpen(refs.hostConfigModal, true);
}

function closeHostConfigModal() {
    state.hostConfigTargetIP = "";
    setModalOpen(refs.hostConfigModal, false);
}

async function saveHostConfig() {
    const ip = state.hostConfigTargetIP;
    if (!ip) return;

    const networkOk = await ensureCartNetworkAssigned(true);
    if (!networkOk) return;

    const assignedValidation = updateHostAssignedIPHint();
    if (!assignedValidation.ok) {
        showError(assignedValidation.message || "Assigned host bits are invalid.");
        return;
    }

    const payload = {
        management_ip: ip,
        iso_selection: refs.hostCfgISO.value || "",
        assigned_host_bits: assignedValidation.bits,
        template_data: templateDataFromHostConfigInputs(),
    };

    const response = await API.addHostToCart(payload);
    if (response.status_code !== 200) {
        showError(messageFromResponse(response, `Failed to save host ${ip} config`));
        return;
    }

    closeHostConfigModal();
    await refreshWizardData();
}

function closeDeployWizard() {
    setModalOpen(refs.deployWizardModal, false);
}

async function openDeployWizard() {
    hideError();
    hideInfo();
    state.wizardStage = 1;
    updateWizardStageUI();
    try {
        await loadBookingNetworkPrefill();
    } catch (err) {
        showError(err?.message || "Failed to load booking network defaults.");
        return;
    }
    const networkOk = await ensureCartNetworkAssigned(true);
    if (!networkOk) {
        return;
    }
    await refreshWizardData();

    if (!refs.wizardBookingCIDR.value.trim()) {
        refs.wizardBookingCIDR.value = state.bookingNetworkPrefill?.suggested_cidr || "";
    }
    if (state.bookingNetworkPrefill) {
        const prefill = state.bookingNetworkPrefill;
        const hostPrefixText = prefill.host_network_prefix ? ` | Host /${prefill.host_network_prefix}` : "";
        renderBookingCIDRHint(
            `Pool ${prefill.supernet_cidr} | /${prefill.booking_prefix} per booking | Assigned ${refs.wizardBookingCIDR.value.trim() || prefill.suggested_cidr}${hostPrefixText}`,
            false,
        );
    }
    setModalOpen(refs.deployWizardModal, true);
}

async function wizardNext() {
    if (state.wizardStage === 1) {
        const name = refs.wizardBookingName.value.trim();
        if (!name) {
            showError("Booking name is required before continuing.");
            return;
        }

        const networkOk = await ensureCartNetworkAssigned(true);
        if (!networkOk) {
            return;
        }
    }

    if (state.wizardStage === 2 && cartHostsArray().length === 0) {
        showError("Add at least one host before review.");
        return;
    }

    state.wizardStage = Math.min(3, state.wizardStage + 1);
    updateWizardStageUI();
    renderWizardReview();
}

function wizardBack() {
    state.wizardStage = Math.max(1, state.wizardStage - 1);
    updateWizardStageUI();
}

async function submitWizardDeployment() {
    hideError();
    hideInfo();

    const name = refs.wizardBookingName.value.trim();
    if (!name) {
        showError("Booking name is required.");
        return;
    }

    if (cartHostsArray().length === 0) {
        showError("Add at least one host before submitting.");
        return;
    }

    const networkOk = await ensureCartNetworkAssigned(true);
    if (!networkOk) {
        return;
    }

    const payload = {
        name,
        description: refs.wizardBookingDescription.value.trim(),
        duration_days: Number(refs.wizardBookingDuration.value || "1"),
        boot_mode: refs.wizardBootMode.value || "UEFI",
        force_restart: true,
    };

    setWizardSubmitPending(true);
    try {
        const response = await API.deployBooking(payload);
        if (response.status_code !== 202) {
            showError(messageFromResponse(response, "Failed to deploy booking"));
            return;
        }

        const provisioning = response.body?.provisioning;
        if (provisioning?.booking_id) {
            state.provisioningByBooking.set(provisioning.booking_id, provisioning);
        }

        closeDeployWizard();
        refs.wizardBookingName.value = "";
        refs.wizardBookingDescription.value = "";
        refs.wizardBookingDuration.value = "1";
        refs.wizardBookingCIDR.value = "";
        refs.wizardBootMode.value = "UEFI";

        showInfo("Deployment submitted. Opening provisioning log.");
        await refreshAll();
        if (provisioning?.booking_id) {
            await openProvisioningLogModal(provisioning.booking_id);
        }
    } finally {
        setWizardSubmitPending(false);
    }
}

function stopProvisioningPolling() {
    if (state.provisioningPollTimer) {
        window.clearInterval(state.provisioningPollTimer);
        state.provisioningPollTimer = null;
    }
}

function renderProvisioningModal() {
    const bookingID = state.selectedProvisioningBookingID;
    const provisioning = state.provisioningByBooking.get(bookingID);
    refs.provisioningSummary.textContent = provisioning
        ? `${provisioning.status} | Updated ${formatTime(provisioning.updated_at)}`
        : "No active provisioning state for this booking.";

    refs.provisioningHostsList.textContent = "";
    const hosts = provisioning?.hosts || [];
    refs.provisioningHostsEmpty.classList.toggle("hidden", hosts.length > 0);
    for (const host of hosts) {
        const frag = refs.provisioningHostRowTemplate.content.cloneNode(true);
        frag.querySelector('[data-field="management_ip"]').textContent = host.management_ip;
        frag.querySelector('[data-field="status"]').textContent = `${host.status} | ${host.message}`;
        refs.provisioningHostsList.appendChild(frag);
    }

    refs.provisioningEventsList.textContent = "";
    const events = provisioning?.events || [];
    refs.provisioningEventsEmpty.classList.toggle("hidden", events.length > 0);
    for (const event of events.slice().reverse()) {
        const frag = refs.provisioningEventRowTemplate.content.cloneNode(true);
        frag.querySelector('[data-field="heading"]').textContent = `[${event.level}] ${formatTime(event.at)}`;
        frag.querySelector('[data-field="message"]').textContent = event.message;
        refs.provisioningEventsList.appendChild(frag);
    }
}

function populateProvisioningBookingSelect() {
    refs.provisioningBookingSelect.textContent = "";
    for (const bookingView of state.bookings) {
        const booking = bookingView.booking;
        if (!booking) continue;
        const option = document.createElement("option");
        option.value = `${booking.id}`;
        option.textContent = `#${booking.id} ${booking.name}`;
        refs.provisioningBookingSelect.appendChild(option);
    }

    if (state.selectedProvisioningBookingID) {
        refs.provisioningBookingSelect.value = `${state.selectedProvisioningBookingID}`;
    }
}

async function refreshProvisioningForSelectedBooking() {
    const bookingID = Number(state.selectedProvisioningBookingID || 0);
    if (!bookingID) {
        renderProvisioningModal();
        return;
    }

    const response = await API.getBookingProvisioningStatus(bookingID);
    if (response.status_code === 200) {
        state.provisioningByBooking.set(bookingID, response.body);
    } else if (response.status_code === 404) {
        state.provisioningByBooking.delete(bookingID);
    } else {
        showError(messageFromResponse(response, "Failed to load provisioning log"));
    }
    renderProvisioningModal();
}

function closeProvisioningLogModal() {
    setModalOpen(refs.provisioningLogModal, false);
    stopProvisioningPolling();
}

async function openProvisioningLogModal(bookingID = 0) {
    hideError();
    hideInfo();

    if (!state.bookings.length) {
        showError("No bookings available to inspect provisioning logs.");
        return;
    }

    state.selectedProvisioningBookingID = bookingID || state.bookings[0]?.booking?.id || 0;
    populateProvisioningBookingSelect();
    setModalOpen(refs.provisioningLogModal, true);
    await refreshProvisioningForSelectedBooking();

    stopProvisioningPolling();
    state.provisioningPollTimer = window.setInterval(async () => {
        await refreshProvisioningForSelectedBooking();
    }, 4000);
}

async function refreshBookingByID(bookingID) {
    const response = await API.getBookingByID(bookingID);
    if (response.status_code !== 200) {
        return;
    }
    const idx = state.bookings.findIndex((entry) => entry?.booking?.id === bookingID);
    if (idx >= 0) {
        state.bookings[idx] = response.body;
    } else {
        state.bookings.unshift(response.body);
    }
    renderBookings();
}

async function refreshAll() {
    await Promise.all([loadBookings(), loadCart(), loadAvailableHosts()]);
    renderBookings();
    renderWizardHosts();
    renderWizardReview();
    populateProvisioningBookingSelect();
}

function wireGlobalCloseActions() {
    document.addEventListener("click", (event) => {
        const target = event.target;
        if (!(target instanceof HTMLElement)) return;

        const action = target.dataset.action;
        if (!action) return;

        if (action === "close-wizard") {
            closeDeployWizard();
        }
        if (action === "close-host-config") {
            closeHostConfigModal();
        }
        if (action === "close-provisioning-log") {
            closeProvisioningLogModal();
        }
    });
}

function wireEvents() {
    refs.refreshBookingsBtn?.addEventListener("click", async () => {
        hideError();
        hideInfo();
        await refreshAll();
    });

    refs.openDeployWizardBtn?.addEventListener("click", async () => {
        await openDeployWizard();
    });

    refs.openProvisioningLogBtn?.addEventListener("click", async () => {
        await openProvisioningLogModal();
    });

    refs.wizardRefreshHostsBtn?.addEventListener("click", async () => {
        await refreshWizardData();
    });

    refs.wizardBackBtn?.addEventListener("click", () => {
        wizardBack();
    });

    refs.wizardNextBtn?.addEventListener("click", async () => {
        await wizardNext();
    });

    refs.wizardSubmitBtn?.addEventListener("click", async () => {
        await submitWizardDeployment();
    });

    refs.hostCfgSaveBtn?.addEventListener("click", async () => {
        await saveHostConfig();
    });

    refs.hostCfgAssignedIP?.addEventListener("input", () => {
        updateHostAssignedIPHint();
    });

    refs.provisioningRefreshBtn?.addEventListener("click", async () => {
        await refreshProvisioningForSelectedBooking();
    });

    refs.provisioningBookingSelect?.addEventListener("change", async () => {
        state.selectedProvisioningBookingID = Number(refs.provisioningBookingSelect.value || "0");
        await refreshProvisioningForSelectedBooking();
    });

    wireGlobalCloseActions();
}

async function init() {
    try {
        await loadEnums();
        await loadISOImages();
        await refreshAll();
        updateWizardStageUI();
        wireEvents();
    } catch (err) {
        console.error(err);
        showError(err?.message || "Failed to initialize dashboard.");
    }
}

init();
