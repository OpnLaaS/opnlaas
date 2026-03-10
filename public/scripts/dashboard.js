import * as API from "./api/api.js";
import { dateTimeFormat, reverseObject } from "./lib/util.js";
import { dismissToasts, showErrorToast, showInfoToast, showSuccessToast, showWarningToast } from "./lib/toast.js";
import MarkdownIt from "markdown-it";
import hljs from "highlight.js/lib/core";
import bash from "highlight.js/lib/languages/bash";
import json from "highlight.js/lib/languages/json";
import yaml from "highlight.js/lib/languages/yaml";
import plaintext from "highlight.js/lib/languages/plaintext";

const state = {
    bookingStatusNames: {},
    bookingPermissionLevelNames: {},
    vendorNames: {},
    formFactorNames: {},
    powerActionNames: {},
    powerStateNames: {},
    isos: [],
    availableHosts: [],
    cart: { hosts: {}, network_cidr: "", gateway_ipv4: "", dns_servers: [] },
    bookings: [],
    provisioningByBooking: new Map(),
    bookingNetworkPrefill: null,
    bookingDurationMaxDays: 32,
    bookingDurationDefaultDays: 32,
    bookingServerDNSName: "laas.cyber.lab",
    wizardStage: 1,
    hostConfigTargetIP: "",
    selectedProvisioningBookingID: 0,
    provisioningPollTimer: null,
    provisioningForceScrollToBottom: false,
    provisioningLevelFilters: new Set(["info", "warn", "error"]),
    watchedProvisioningBookingIDs: new Set(),
    watchedProvisioningLastStatus: new Map(),
    provisioningWatchTimer: null,
};

const refs = {
    refreshBookingsBtn: document.getElementById("refreshBookingsBtn"),
    openDeployWizardBtn: document.getElementById("openDeployWizardBtn"),
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
    wizardBookingDurationValue: document.getElementById("wizardBookingDurationValue"),
    wizardBookingDurationHint: document.getElementById("wizardBookingDurationHint"),
    wizardBookingDNSName: document.getElementById("wizardBookingDNSName"),
    wizardBookingCIDR: document.getElementById("wizardBookingCIDR"),
    wizardBookingCIDRHint: document.getElementById("wizardBookingCIDRHint"),
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
    hostCfgBootMode: document.getElementById("hostCfgBootMode"),
    hostCfgHostname: document.getElementById("hostCfgHostname"),
    hostCfgHostnamePreview: document.getElementById("hostCfgHostnamePreview"),
    hostCfgTimezone: document.getElementById("hostCfgTimezone"),
    hostCfgLocale: document.getElementById("hostCfgLocale"),
    hostCfgKeyboard: document.getElementById("hostCfgKeyboard"),
    hostCfgMirror: document.getElementById("hostCfgMirror"),
    hostCfgPackages: document.getElementById("hostCfgPackages"),
    hostCfgLateScript: document.getElementById("hostCfgLateScript"),
    hostCfgSaveBtn: document.getElementById("hostCfgSaveBtn"),
    provisioningLogModal: document.getElementById("provisioningLogModal"),
    provisioningBookingLabel: document.getElementById("provisioningBookingLabel"),
    provisioningCancelBtn: document.getElementById("provisioningCancelBtn"),
    provisioningRefreshBtn: document.getElementById("provisioningRefreshBtn"),
    provisioningSummary: document.getElementById("provisioningSummary"),
    provisioningProgressLabel: document.getElementById("provisioningProgressLabel"),
    provisioningProgressBar: document.getElementById("provisioningProgressBar"),
    provisioningHostsEmpty: document.getElementById("provisioningHostsEmpty"),
    provisioningHostsList: document.getElementById("provisioningHostsList"),
    provisioningHostRowTemplate: document.getElementById("provisioningHostRowTemplate"),
    provisioningEventsEmpty: document.getElementById("provisioningEventsEmpty"),
    provisioningEventsList: document.getElementById("provisioningEventsList"),
    provisioningEventRowTemplate: document.getElementById("provisioningEventRowTemplate"),
    provisioningLevelButtons: Array.from(document.querySelectorAll('[data-action="toggle-provisioning-level"]')),
};

hljs.registerLanguage("bash", bash);
hljs.registerLanguage("shell", bash);
hljs.registerLanguage("json", json);
hljs.registerLanguage("yaml", yaml);
hljs.registerLanguage("plaintext", plaintext);

const provisioningMarkdown = new MarkdownIt({
    html: false,
    linkify: false,
    breaks: true,
    highlight(source, language) {
        const lang = `${language || ""}`.trim().toLowerCase();
        const escaped = provisioningMarkdown.utils.escapeHtml(source);
        if (!source) {
            return "";
        }

        try {
            if (lang && hljs.getLanguage(lang)) {
                return `<pre><code class="hljs language-${lang}">${hljs.highlight(source, { language: lang, ignoreIllegals: true }).value}</code></pre>`;
            }
            return `<pre><code class="hljs">${hljs.highlightAuto(source, ["bash", "json", "yaml", "plaintext"]).value}</code></pre>`;
        } catch (_) {
            return `<pre><code class="hljs">${escaped}</code></pre>`;
        }
    },
});

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

function showSuccess(message) {
    dismissToasts("success");
    showSuccessToast(message);
}

function showWarning(message) {
    dismissToasts("warning");
    showWarningToast(message);
}

function sleep(ms) {
    return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function normalizeOperationStatus(status) {
    const normalized = `${status || ""}`.trim().toLowerCase();
    if (normalized === "success" || normalized === "warning" || normalized === "error") return normalized;
    if (normalized === "failed") return "error";
    if (normalized === "warn") return "warning";
    return normalized || "running";
}

function operationStatusTerminal(status) {
    const normalized = normalizeOperationStatus(status);
    return normalized === "success" || normalized === "warning" || normalized === "error";
}

function provisioningStatusTerminal(status) {
    const normalized = `${status || ""}`.trim().toLowerCase();
    return normalized === "completed"
        || normalized === "partial_failed"
        || normalized === "failed"
        || normalized === "canceled"
        || normalized === "destroyed";
}

function watchProvisioningCompletion(bookingID, initialStatus = "") {
    const id = Number(bookingID || 0);
    if (!id) return;

    const normalized = `${initialStatus || ""}`.trim().toLowerCase();
    if (provisioningStatusTerminal(normalized)) return;

    state.watchedProvisioningBookingIDs.add(id);
    if (normalized) {
        state.watchedProvisioningLastStatus.set(id, normalized);
    }
}

function stopProvisioningCompletionWatcher() {
    if (state.provisioningWatchTimer) {
        window.clearInterval(state.provisioningWatchTimer);
        state.provisioningWatchTimer = null;
    }
}

function provisioningCompletionMessage(bookingID, status) {
    const booking = state.bookings.find((entry) => Number(entry?.booking?.id || 0) === Number(bookingID || 0))?.booking;
    const bookingLabel = booking?.name ? `Booking #${bookingID} ${booking.name}` : `Booking #${bookingID}`;
    const normalized = `${status || ""}`.trim().toLowerCase();

    if (normalized === "completed") return { tone: "success", message: `${bookingLabel} provisioning completed.` };
    if (normalized === "partial_failed") return { tone: "warning", message: `${bookingLabel} provisioning completed with host-level failures.` };
    if (normalized === "failed") return { tone: "error", message: `${bookingLabel} provisioning failed.` };
    if (normalized === "canceled") return { tone: "warning", message: `${bookingLabel} provisioning was canceled.` };
    if (normalized === "destroyed") return { tone: "warning", message: `${bookingLabel} was destroyed.` };
    return { tone: "info", message: `${bookingLabel} provisioning status: ${normalized || "unknown"}.` };
}

async function pollProvisioningWatchList() {
    const watchIDs = Array.from(state.watchedProvisioningBookingIDs);
    for (const bookingID of watchIDs) {
        const response = await API.getBookingProvisioningStatus(bookingID);
        if (response.status_code === 404) {
            state.watchedProvisioningBookingIDs.delete(bookingID);
            state.watchedProvisioningLastStatus.delete(bookingID);
            continue;
        }
        if (response.status_code !== 200) {
            continue;
        }

        const snapshot = response.body || {};
        state.provisioningByBooking.set(bookingID, snapshot);

        const currentStatus = `${snapshot?.status || ""}`.trim().toLowerCase();
        const previousStatus = `${state.watchedProvisioningLastStatus.get(bookingID) || ""}`.trim().toLowerCase();
        if (currentStatus) {
            state.watchedProvisioningLastStatus.set(bookingID, currentStatus);
        }

        if (!provisioningStatusTerminal(currentStatus)) {
            continue;
        }

        const shouldToast = !provisioningStatusTerminal(previousStatus) || previousStatus !== currentStatus;
        if (shouldToast) {
            const completion = provisioningCompletionMessage(bookingID, currentStatus);
            if (completion.tone === "success") {
                showSuccess(completion.message);
            } else if (completion.tone === "warning") {
                showWarning(completion.message);
            } else if (completion.tone === "error") {
                showError(completion.message);
            } else {
                showInfo(completion.message);
            }
        }

        state.watchedProvisioningBookingIDs.delete(bookingID);
        state.watchedProvisioningLastStatus.delete(bookingID);
        await refreshBookingByID(bookingID);
    }
}

function startProvisioningCompletionWatcher() {
    if (state.provisioningWatchTimer) return;
    state.provisioningWatchTimer = window.setInterval(async () => {
        await pollProvisioningWatchList();
    }, 5000);
}

async function waitForOperationTerminal(operationID, timeoutMs = 180000, intervalMs = 1500) {
    const startedAt = Date.now();
    while ((Date.now() - startedAt) < timeoutMs) {
        const response = await API.getOperationByID(operationID);
        if (response.status_code === 404) {
            await sleep(intervalMs);
            continue;
        }
        if (response.status_code !== 200) {
            throw new Error(messageFromResponse(response, `Failed to load operation ${operationID}`));
        }

        const operation = response.body || {};
        if (operationStatusTerminal(operation.status)) {
            return operation;
        }
        await sleep(intervalMs);
    }

    throw new Error("Timed out waiting for operation completion.");
}

async function monitorDashboardHostPowerOperation(operationID, bookingID, managementIP) {
    try {
        const operation = await waitForOperationTerminal(operationID);
        const status = normalizeOperationStatus(operation?.status);
        const message = operation?.message || `Power action finished for ${managementIP}`;

        if (status === "success") {
            showSuccess(message);
        } else if (status === "warning") {
            showWarning(message);
        } else {
            showError(message);
        }
    } catch (err) {
        showError(err?.message || `Failed to track power action for ${managementIP}.`);
    } finally {
        await refreshBookingByID(bookingID);
    }
}

function formatTime(value) {
    if (!value) return "N/A";
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return "N/A";
    return dateTimeFormat.format(parsed);
}

function normalizeProvisioningLevel(level) {
    const normalized = `${level || ""}`.trim().toLowerCase();
    if (normalized === "warn" || normalized === "warning") return "warn";
    if (normalized === "error") return "error";
    return "info";
}

function updateProvisioningFilterButtons() {
    for (const button of refs.provisioningLevelButtons) {
        if (!(button instanceof HTMLButtonElement)) continue;
        const level = normalizeProvisioningLevel(button.dataset.level);
        const active = state.provisioningLevelFilters.has(level);
        button.setAttribute("aria-pressed", active ? "true" : "false");
        button.classList.toggle("is-active", active);
        button.classList.toggle("text-font-secondary", !active);
    }
}

function toggleProvisioningFilterLevel(level) {
    const normalized = normalizeProvisioningLevel(level);
    if (state.provisioningLevelFilters.has(normalized) && state.provisioningLevelFilters.size > 1) {
        state.provisioningLevelFilters.delete(normalized);
    } else {
        state.provisioningLevelFilters.add(normalized);
    }
}

function provisioningStagePercent(status) {
    const normalized = `${status || ""}`.trim().toLowerCase();
    const map = {
        queued: 4,
        reserved: 8,
        running: 18,
        configuring_pxe: 28,
        setting_boot: 38,
        restarting: 48,
        waiting_power: 60,
        awaiting_install: 76,
        installing: 88,
        completed: 100,
        failed: 100,
        partial_failed: 100,
        canceled: 100,
        destroyed: 100,
    };
    return map[normalized] ?? 12;
}

function summarizeProvisioningProgress(provisioning) {
    if (!provisioning) {
        return { percent: 0, text: "Progress unavailable" };
    }

    const hosts = Array.isArray(provisioning.hosts) ? provisioning.hosts : [];
    const total = hosts.length;
    const completed = hosts.filter((host) => `${host?.status || ""}`.trim().toLowerCase() === "completed").length;
    const failed = hosts.filter((host) => `${host?.status || ""}`.trim().toLowerCase() === "failed").length;

    let stage = `${provisioning.status || "queued"}`;
    let percent = provisioningStagePercent(stage);

    let latestHost = null;
    for (const host of hosts) {
        const currentTime = new Date(host?.updated_at || 0).getTime();
        const latestTime = new Date(latestHost?.updated_at || 0).getTime();
        if (!latestHost || currentTime > latestTime) {
            latestHost = host;
        }
    }

    if (latestHost?.status) {
        stage = `${latestHost.status}`;
        percent = Math.max(percent, provisioningStagePercent(stage));
    }

    const hostSummary = total > 0 ? `${completed}/${total} done` : "no hosts";
    const failSummary = failed > 0 ? ` | ${failed} failed` : "";
    return {
        percent: Math.max(0, Math.min(100, Math.round(percent))),
        text: `${hostSummary}${failSummary} | stage: ${stage}`,
    };
}

function bookingStatusLabel(value) {
    const reverse = reverseObject(state.bookingStatusNames || {});
    return reverse[value] || `Status ${value}`;
}

function bookingPermissionLabel(value) {
    const reverse = reverseObject(state.bookingPermissionLevelNames || {});
    return reverse[value] || `Permission ${value}`;
}

function vendorLabel(value) {
    const reverse = reverseObject(state.vendorNames || {});
    return reverse[value] || `${value ?? "Vendor?"}`;
}

function formFactorLabel(value) {
    const reverse = reverseObject(state.formFactorNames || {});
    return reverse[value] || `${value ?? "Form?"}`;
}

function powerStateLabel(value) {
    const reverse = reverseObject(state.powerStateNames || {});
    return reverse[value] || "Unknown";
}

function provisioningStatusLabel(status) {
    const normalized = `${status || ""}`.trim().toLowerCase();
    const map = {
        queued: "queued",
        running: "running",
        awaiting_install: "awaiting install",
        completed: "done",
        failed: "failed",
        partial_failed: "partial failure",
        canceled: "canceled",
        destroyed: "destroyed",
    };
    return map[normalized] || (normalized || "unknown");
}

function provisioningStatusCancelable(status) {
    const normalized = `${status || ""}`.trim().toLowerCase();
    return normalized === "queued" || normalized === "running" || normalized === "awaiting_install";
}

function bookingStateSummary(bookingView) {
    const booking = bookingView?.booking || {};
    const bookingState = bookingStatusLabel(booking.status);
    const provisioning = bookingView?.provisioning;
    if (!provisioning?.status) {
        return bookingState;
    }
    return `${bookingState} / Provisioning ${provisioningStatusLabel(provisioning.status)}`;
}

function parseCSV(raw) {
    return (raw || "")
        .split(",")
        .map((part) => part.trim())
        .filter((part) => part.length > 0);
}

const HOSTNAME_WORDS = [
    "amber", "atlas", "beacon", "binary", "bolt", "comet", "copper", "core", "delta", "echo",
    "ember", "falcon", "flare", "flux", "forge", "frost", "gale", "glint", "graph", "haven",
    "helix", "horizon", "ion", "jade", "jet", "jolt", "lattice", "lumen", "lynx", "matrix",
    "merit", "mint", "mosaic", "nexus", "nova", "onyx", "orbit", "origin", "oxide", "patch",
    "phoenix", "pixel", "plasma", "pulse", "quartz", "radar", "rivet", "rocket", "sage", "saturn",
    "signal", "slate", "solstice", "spark", "spire", "stride", "summit", "switch", "talon", "topaz",
    "torch", "tracer", "vector", "verge", "vertex", "viper", "vista", "vivid", "warp", "zephyr",
];

function randomHostnameWord() {
    if (!HOSTNAME_WORDS.length) {
        return "node";
    }
    const idx = Math.floor(Math.random() * HOSTNAME_WORDS.length);
    return HOSTNAME_WORDS[idx];
}

function sanitizeHostnameLabel(value) {
    const lowered = `${value || ""}`.trim().toLowerCase();
    if (!lowered) return "";

    const withoutApostrophes = lowered.replace(/['’]/g, "");
    const normalized = withoutApostrophes
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-+|-+$/g, "");

    return normalized.slice(0, 48).replace(/^-+|-+$/g, "");
}

function dnsLabelFromBookingName(name) {
    const normalized = `${name || ""}`.trim().toLowerCase()
        .replace(/['’]/g, "")
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-+|-+$/g, "");
    const fallback = normalized || "booking";
    return fallback.slice(0, 48).replace(/^-+|-+$/g, "") || "booking";
}

function bookingDescriptionLength() {
    return `${refs.wizardBookingDescription?.value || ""}`.trim().length;
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

function applyDurationDefaults() {
    const maxDays = Math.max(1, Number(state.bookingDurationMaxDays || 32));
    const defaultDays = Math.min(maxDays, Math.max(1, Number(state.bookingDurationDefaultDays || 32)));

    refs.wizardBookingDuration.max = `${maxDays}`;
    if (!refs.wizardBookingDuration.value) {
        refs.wizardBookingDuration.value = `${defaultDays}`;
    }

    const parsed = Number(refs.wizardBookingDuration.value || defaultDays);
    const clamped = Math.min(maxDays, Math.max(1, Number.isFinite(parsed) ? parsed : defaultDays));
    refs.wizardBookingDuration.value = `${clamped}`;

    if (refs.wizardBookingDurationValue) {
        refs.wizardBookingDurationValue.textContent = `${clamped}`;
    }
    if (refs.wizardBookingDurationHint) {
        refs.wizardBookingDurationHint.textContent = `1-${maxDays} days`;
    }
}

function updateWizardGeneratedDNS() {
    if (!refs.wizardBookingDNSName) return;
    refs.wizardBookingDNSName.value = dnsLabelFromBookingName(refs.wizardBookingName.value);
    updateHostCfgHostnamePreview();
}

function updateHostCfgHostnamePreview() {
    if (!refs.hostCfgHostnamePreview) return;
    const label = sanitizeHostnameLabel(refs.hostCfgHostname?.value || "");
    const bookingDNS = `${refs.wizardBookingDNSName?.value || dnsLabelFromBookingName(refs.wizardBookingName?.value || "") || "booking"}`.trim() || "booking";
    const serverDNS = `${state.bookingServerDNSName || "laas.cyber.lab"}`.trim().replace(/\.+$/g, "") || "laas.cyber.lab";
    const shownLabel = label || "<label>";
    refs.hostCfgHostnamePreview.textContent = `${shownLabel}.${bookingDNS}.${serverDNS}`;
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
    const [bookingStatusesRes, bookingPermissionsRes, vendorsRes, formFactorsRes, powerActionsRes, powerStatesRes] = await Promise.all([
        API.getBookingStatuses(),
        API.getBookingPermissionLevels(),
        API.getVendors(),
        API.getFormFactors(),
        API.getPowerActions(),
        API.getPowerStates(),
    ]);
    state.bookingStatusNames = bookingStatusesRes?.status_code === 200 ? bookingStatusesRes.body : {};
    state.bookingPermissionLevelNames = bookingPermissionsRes?.status_code === 200 ? bookingPermissionsRes.body : {};
    state.vendorNames = vendorsRes?.status_code === 200 ? vendorsRes.body : {};
    state.formFactorNames = formFactorsRes?.status_code === 200 ? formFactorsRes.body : {};
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
    state.bookingDurationMaxDays = Number(response.body?.max_duration_days || 32);
    state.bookingDurationDefaultDays = Number(response.body?.default_duration_days || 32);
    state.bookingServerDNSName = `${response.body?.server_dns_name || "laas.cyber.lab"}`.trim().replace(/\.+$/g, "") || "laas.cyber.lab";
    applyDurationDefaults();
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
        const operationID = `${response?.body?.operation?.id || ""}`.trim();
        if (operationID) {
            showInfo(messageFromResponse(response, `Power action queued for ${managementIP}`));
            void monitorDashboardHostPowerOperation(operationID, bookingID, managementIP);
            return;
        }

        showSuccess(messageFromResponse(response, `Power action completed for ${managementIP}`));
        await refreshBookingByID(bookingID);
    } finally {
        buttonEl.disabled = false;
        buttonEl.textContent = previous;
    }
}

async function cancelBookingProvisioning(bookingID, buttonEl = null) {
    const previous = buttonEl?.textContent || "";
    if (buttonEl) {
        buttonEl.disabled = true;
        buttonEl.textContent = "Canceling...";
    }

    hideError();
    hideInfo();
    try {
        const response = await API.cancelBookingProvisioning(bookingID);
        if (response.status_code !== 200) {
            showError(messageFromResponse(response, `Failed to cancel provisioning for booking ${bookingID}`));
            return;
        }

        const provisioning = response.body?.provisioning;
        if (provisioning?.booking_id) {
            state.provisioningByBooking.set(provisioning.booking_id, provisioning);
            watchProvisioningCompletion(provisioning.booking_id, provisioning.status || "queued");
        }
        showInfo(`Provisioning canceled for booking ${bookingID}`);
        await refreshBookingByID(bookingID);
        if (Number(state.selectedProvisioningBookingID || 0) === Number(bookingID)) {
            await refreshProvisioningForSelectedBooking();
        }
    } finally {
        if (buttonEl) {
            buttonEl.disabled = false;
            buttonEl.textContent = previous;
        }
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
        frag.querySelector('[data-field="meta"]').textContent = `#${booking.id} | ${bookingStateSummary(bookingView)} | ${bookingPermissionLabel(bookingView.permission_level)}`;
        frag.querySelector('[data-field="window"]').textContent = `Start: ${formatTime(booking.start_time)} | End: ${formatTime(booking.end_time)}`;
        frag.querySelector('[data-field="given-user"]').textContent = bookingView.credentials?.given_user_username || "N/A";
        frag.querySelector('[data-field="given-pass"]').textContent = bookingView.credentials?.given_user_password || "N/A";
        frag.querySelector('[data-field="managed-user"]').textContent = bookingView.credentials?.managed_user_username || "N/A";
        frag.querySelector('[data-field="managed-pass"]').textContent = bookingView.credentials?.managed_user_password || "N/A";

        const logsBtn = frag.querySelector('[data-action="open-provisioning-log"]');
        logsBtn?.addEventListener("click", async () => {
            await openProvisioningLogModal(booking.id);
        });

        const cancelProvisioningBtn = frag.querySelector('[data-action="cancel-provisioning"]');
        const canCancelProvisioning = !!bookingView?.provisioning?.cancelable || provisioningStatusCancelable(bookingView?.provisioning?.status);
        cancelProvisioningBtn.classList.toggle("hidden", !canCancelProvisioning);
        if (canCancelProvisioning) {
            cancelProvisioningBtn.addEventListener("click", async () => {
                if (!window.confirm(`Cancel provisioning for booking ${booking.id}?`)) {
                    return;
                }
                await cancelBookingProvisioning(booking.id, cancelProvisioningBtn);
            });
        }

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
    const bootMode = host.boot_mode || "UEFI";
    const hostname = host.hostname || "auto";
    const timezone = host.template_data?.["template.timezone"] || "default";
    const locale = host.template_data?.["template.locale"] || "default";
    return `ISO: ${iso} | Boot: ${bootMode} | Host: ${hostname} | IP: auto (sequential) | TZ: ${timezone} | Locale: ${locale} | Packages: ${templatePackagesSummary(host.template_data)}`;
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
        frag.querySelector('[data-field="details"]').textContent = `${vendorLabel(host.vendor)} | ${host.model || "Model?"} | ${formFactorLabel(host.form_factor)}`;
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
    const duration = refs.wizardBookingDuration.value || `${state.bookingDurationDefaultDays || 32}`;
    refs.wizardReviewMeta.innerHTML = `
        <div><span class="text-font-secondary">Name:</span> ${name || "(missing)"}</div>
        <div><span class="text-font-secondary">DNS:</span> ${refs.wizardBookingDNSName.value || "booking"}</div>
        <div><span class="text-font-secondary">Description:</span> ${description}</div>
        <div><span class="text-font-secondary">Duration:</span> ${duration} day(s)</div>
        <div><span class="text-font-secondary">Subnet:</span> ${refs.wizardBookingCIDR.value.trim() || "N/A"}</div>
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
    refs.hostCfgBootMode.value = "UEFI";
    if (refs.hostCfgHostname) {
        refs.hostCfgHostname.value = randomHostnameWord();
    }
    refs.hostCfgTimezone.value = "";
    refs.hostCfgLocale.value = "";
    refs.hostCfgKeyboard.value = "";
    refs.hostCfgMirror.value = "";
    refs.hostCfgPackages.value = "";
    refs.hostCfgLateScript.value = "";
    updateHostCfgHostnamePreview();
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
    refs.hostCfgBootMode.value = host?.boot_mode || "UEFI";
    if (refs.hostCfgHostname) {
        const rawHostname = `${host?.hostname || ""}`.trim();
        const hostLabel = sanitizeHostnameLabel(rawHostname.split(".")[0] || rawHostname);
        refs.hostCfgHostname.value = hostLabel || randomHostnameWord();
    }
    updateHostCfgHostnamePreview();

    if (host?.template_data) {
        refs.hostCfgTimezone.value = host.template_data["template.timezone"] || "";
        refs.hostCfgLocale.value = host.template_data["template.locale"] || "";
        refs.hostCfgKeyboard.value = host.template_data["template.keyboard_layout"] || "";
        refs.hostCfgMirror.value = host.template_data["template.mirror"] || "";
        refs.hostCfgPackages.value = host.template_data["template.packages"] || "";
        refs.hostCfgLateScript.value = host.template_data["template.late_script"] || "";
    }

    setModalOpen(refs.hostConfigModal, true);
}

function closeHostConfigModal() {
    state.hostConfigTargetIP = "";
    setModalOpen(refs.hostConfigModal, false);
}

async function saveHostConfig() {
    const ip = state.hostConfigTargetIP;
    if (!ip) return;

    const normalizedHostname = sanitizeHostnameLabel(refs.hostCfgHostname?.value || "");
    const hostnameLabel = normalizedHostname || randomHostnameWord();
    if (refs.hostCfgHostname) {
        refs.hostCfgHostname.value = hostnameLabel;
    }
    updateHostCfgHostnamePreview();

    const payload = {
        management_ip: ip,
        iso_selection: refs.hostCfgISO.value || "",
        boot_mode: refs.hostCfgBootMode.value || "UEFI",
        hostname: hostnameLabel,
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
    refs.wizardBookingDuration.value = `${state.bookingDurationDefaultDays || 32}`;
    applyDurationDefaults();
    updateWizardGeneratedDNS();
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
        if (bookingDescriptionLength() < 128) {
            showError("Description must be at least 128 characters before continuing.");
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
    if (bookingDescriptionLength() < 128) {
        showError("Description must be at least 128 characters.");
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
        duration_days: Number(refs.wizardBookingDuration.value || `${state.bookingDurationDefaultDays || 32}`),
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
        refs.wizardBookingDuration.value = `${state.bookingDurationDefaultDays || 32}`;
        applyDurationDefaults();
        updateWizardGeneratedDNS();
        refs.wizardBookingCIDR.value = "";

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
    const activeFilterList = ["info", "warn", "error"].filter((level) => state.provisioningLevelFilters.has(level)).join(",");
    const progress = summarizeProvisioningProgress(provisioning);
    refs.provisioningSummary.textContent = provisioning
        ? `${provisioning.status} | Updated ${formatTime(provisioning.updated_at)} | filters: ${activeFilterList}`
        : "No active provisioning state for this booking.";
    if (refs.provisioningProgressLabel) {
        refs.provisioningProgressLabel.textContent = progress.text;
    }
    if (refs.provisioningProgressBar) {
        refs.provisioningProgressBar.style.width = `${progress.percent}%`;
        refs.provisioningProgressBar.dataset.state = `${provisioning?.status || ""}`.trim().toLowerCase();
    }
    if (refs.provisioningCancelBtn) {
        const canCancel = provisioningStatusCancelable(provisioning?.status);
        refs.provisioningCancelBtn.classList.toggle("hidden", !canCancel);
        refs.provisioningCancelBtn.disabled = !canCancel;
    }
    updateProvisioningFilterButtons();

    refs.provisioningHostsList.textContent = "";
    const hosts = provisioning?.hosts || [];
    refs.provisioningHostsEmpty.classList.toggle("hidden", hosts.length > 0);
    for (const host of hosts) {
        const frag = refs.provisioningHostRowTemplate.content.cloneNode(true);
        const model = `${host.model || ""}`.trim();
        const hostname = `${host.hostname || ""}`.trim();
        const hostTarget = model && hostname
            ? `${model} -> ${hostname}`
            : (hostname || model || `Host ${host.management_ip || ""}`.trim());
        frag.querySelector('[data-field="target"]').textContent = hostTarget || "Host";
        frag.querySelector('[data-field="status"]').textContent = `${host.status} | ${host.message}`;
        refs.provisioningHostsList.appendChild(frag);
    }

    const shouldStickToBottom = state.provisioningForceScrollToBottom || (() => {
        const el = refs.provisioningEventsList;
        if (!el) return true;
        const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
        return distance <= 40;
    })();

    refs.provisioningEventsList.textContent = "";
    const events = provisioning?.events || [];
    const filteredEvents = events.filter((event) => state.provisioningLevelFilters.has(normalizeProvisioningLevel(event?.level)));
    refs.provisioningEventsEmpty.textContent = events.length === 0
        ? "No events yet."
        : "No events match current level filters.";
    refs.provisioningEventsEmpty.classList.toggle("hidden", filteredEvents.length > 0);

    for (const event of filteredEvents) {
        const frag = refs.provisioningEventRowTemplate.content.cloneNode(true);
        const level = normalizeProvisioningLevel(event.level);
        const row = frag.querySelector('[data-field="row"]');
        if (row) row.dataset.level = level;
        frag.querySelector('[data-field="heading"]').textContent = `${formatTime(event.at)} [${level.toUpperCase()}]`;
        const messageEl = frag.querySelector('[data-field="message"]');
        if (messageEl) {
            messageEl.innerHTML = provisioningMarkdown.render(event.message || "");
        }
        refs.provisioningEventsList.appendChild(frag);
    }

    if (shouldStickToBottom) {
        refs.provisioningEventsList.scrollTop = refs.provisioningEventsList.scrollHeight;
        state.provisioningForceScrollToBottom = false;
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

    const selectedBookingID = Number(bookingID || 0);
    if (!selectedBookingID) {
        showError("Provisioning logs must be opened from a specific booking.");
        return;
    }

    const bookingView = state.bookings.find((entry) => entry?.booking?.id === selectedBookingID);
    const booking = bookingView?.booking;
    if (!booking) {
        showError(`Booking ${selectedBookingID} is no longer available.`);
        return;
    }

    watchProvisioningCompletion(selectedBookingID, bookingView?.provisioning?.status || "");

    state.selectedProvisioningBookingID = selectedBookingID;
    state.provisioningForceScrollToBottom = true;
    if (refs.provisioningBookingLabel) {
        refs.provisioningBookingLabel.textContent = `Booking #${booking.id} ${booking.name || ""}`.trim();
    }
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

    refs.wizardRefreshHostsBtn?.addEventListener("click", async () => {
        await refreshWizardData();
    });

    refs.wizardBookingName?.addEventListener("input", () => {
        updateWizardGeneratedDNS();
        renderWizardReview();
    });

    refs.wizardBookingDuration?.addEventListener("input", () => {
        applyDurationDefaults();
        renderWizardReview();
    });

    refs.wizardBookingDescription?.addEventListener("input", () => {
        renderWizardReview();
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

    refs.hostCfgHostname?.addEventListener("input", () => {
        updateHostCfgHostnamePreview();
    });

    refs.provisioningRefreshBtn?.addEventListener("click", async () => {
        await refreshProvisioningForSelectedBooking();
    });
    refs.provisioningCancelBtn?.addEventListener("click", async () => {
        const bookingID = Number(state.selectedProvisioningBookingID || 0);
        if (!bookingID) return;
        if (!window.confirm(`Cancel provisioning for booking ${bookingID}?`)) {
            return;
        }
        await cancelBookingProvisioning(bookingID, refs.provisioningCancelBtn);
    });

    for (const button of refs.provisioningLevelButtons) {
        button.addEventListener("click", () => {
            toggleProvisioningFilterLevel(button.dataset.level);
            renderProvisioningModal();
        });
    }

    wireGlobalCloseActions();
}

async function init() {
    try {
        await loadEnums();
        await loadISOImages();
        applyDurationDefaults();
        updateWizardGeneratedDNS();
        await refreshAll();
        startProvisioningCompletionWatcher();
        updateProvisioningFilterButtons();
        updateWizardStageUI();
        wireEvents();
        window.addEventListener("beforeunload", () => {
            stopProvisioningCompletionWatcher();
            stopProvisioningPolling();
        });
    } catch (err) {
        console.error(err);
        showError(err?.message || "Failed to initialize dashboard.");
    }
}

init();
