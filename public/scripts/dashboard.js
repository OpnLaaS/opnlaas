import { reverseObject } from "./lib/util.js";
import * as API from "./api/api.js";

// MM/DD/YYYY 24HR:MM
const dateTimeFormat = new Intl.DateTimeFormat("en-US", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
});

const list = document.getElementById("host-list");
const template = document.getElementById("host-item-template");
const emptyState = document.getElementById("empty");
const forms = {
    host: document.getElementById("newHostForm"),
    iso: document.getElementById("uploadISOPopup"),
};

const formTriggers = {
    host: document.getElementById("addHostBtn"),
    iso: document.getElementById("uploadISOBtn"),
};

const cancelButtons = {
    host: document.getElementById("cancelHostBtn"),
    iso: document.getElementById("cancelISOBtn"),
};

const hostFormElement = document.getElementById("hostForm");
const isoForm = document.getElementById("uploadISOForm");

function toggleItem(button) {
    const section = button.closest("section");
    const collapsible = section.querySelector(".transition-all:not(.power-menu)");
    const arrow = button.querySelector("svg");
    const isCollapsed = collapsible.classList.contains("max-h-0");

    if (isCollapsed) {
        collapsible.classList.remove("max-h-0", "opacity-0");
        collapsible.classList.add("collapsible-open", "opacity-100");
        arrow.style.transform = "rotate(180deg)";
    } else {
        collapsible.classList.add("max-h-0", "opacity-0");
        collapsible.classList.remove("collapsible-open", "opacity-100");
        arrow.style.transform = "";
    }
}

function togglePowerMenu(button) {
    const wrapper = button.closest("div.relative");
    const menu = wrapper.querySelector(".power-menu");
    const isClosed = menu.classList.contains("max-h-0");

    closeAllMenus();
    if (isClosed) {
        resetPowerMenu(menu);
        menu.classList.remove("max-h-0", "opacity-0");
        menu.classList.add("max-h-96", "opacity-100");
    }
}

function closeAllMenus() {
    document.querySelectorAll(".power-menu").forEach(menu => {
        resetPowerMenu(menu);
        menu.classList.add("max-h-0", "opacity-0");
        menu.classList.remove("max-h-96", "opacity-100");
    });
}

document.addEventListener("click", function (event) {
    const isPowerMenu = event.target.closest(".power-menu") || event.target.closest(".power-button");
    const isMgmtMenu = event.target.closest("#mgmtTypeMenu") || event.target.closest("#mgmtMenuBtn");

    if (isPowerMenu || isMgmtMenu) {
        return;
    }

    closeAllMenus();
    if (typeof closeMgmtMenu === "function") closeMgmtMenu();
});

// Pretty-print capacity in GB/TB
function prettyCapacityGB(gb) {
    if (gb == null || isNaN(gb)) return "Unknown";
    const n = Number(gb);
    if (n >= 1024) return (n / 1024).toFixed(1).replace(/\.0$/, "") + " TB";
    return n + " GB";
}

function totalCapacityGB(devs) {
    return (devs || []).reduce((sum, d) => sum + (Number(d.capacity_gb) || 0), 0);
}

function renderStorageLine(dev) {
    if (!dev || typeof dev !== "object") return "Unknown device";
    const parts = [];
    if ("capacity_gb" in dev) parts.push(prettyCapacityGB(dev.capacity_gb));
    if (dev.media_type) parts.push(String(dev.media_type).toUpperCase());
    if (dev.interface) parts.push(dev.interface);
    if (dev.model) parts.push(dev.model);
    return parts.filter(Boolean).join(" • ");
}

document.addEventListener("DOMContentLoaded", async () => {
    await renderHosts();
});

// expose to template

function resolveEnum(maybeMap, value) {
    if (maybeMap && typeof maybeMap === "object" && (value in maybeMap)) return maybeMap[value];
    return (value ?? "—");
}

function cleanSku(manufacturer, sku) {
    if (!sku) return sku;
    const man = (manufacturer || "").toLowerCase().trim();
    const s = String(sku).trim();
    if (man && s.toLowerCase().startsWith(man)) {
        return s.slice(man.length).trim().replace(/^[-,\s]+/, "");
    }
    return s;
}


window.toggleItem = toggleItem;
window.togglePowerMenu = togglePowerMenu;
window.closeAllMenus = closeAllMenus;

function applyPowerBadge(badgeEl, stateLabel) {
    if (!badgeEl) return;
    const normalized = String(stateLabel || "").toLowerCase();
    badgeEl.classList.remove("power-on", "power-off", "power-unknown");
    if (normalized.includes("on")) {
        badgeEl.classList.add("power-on");
    } else if (normalized.includes("off")) {
        badgeEl.classList.add("power-off");
    } else {
        badgeEl.classList.add("power-unknown");
    }
}

async function renderHosts() {
    try {
        const hostData = await API.getHostsAll();
        const [vendorsRes, formFactorsRes, mgmtTypesRes, powerStatesRes] = await Promise.all([
            API.getVendors(),
            API.getFormFactors(),
            API.getManagementTypes(),
            API.getPowerStates(),
        ]);

        const vendorNames = reverseObject(vendorsRes.body || {});
        const formFactors = reverseObject(formFactorsRes.body || {});
        const mgmtTypes = reverseObject(mgmtTypesRes.body || {});
        const powerStates = reverseObject(powerStatesRes.body || {});

        list.innerHTML = "";
        const hosts = Array.isArray(hostData.body) ? hostData.body : [];

        if (!hosts.length) {
            emptyState?.classList.remove("hidden");
            return;
        }

        emptyState?.classList.add("hidden");
        hosts.forEach((host) => {
            const frag = template.content.cloneNode(true);

            // header
            frag.querySelector('[data-field="name"]').textContent = host.model;
            frag.querySelector('[data-field="form_factor"]').textContent = resolveEnum(formFactors, host.form_factor);
            const powerLabel = resolveEnum(powerStates, host.last_known_power_state);
            const powerNode = frag.querySelector('[data-field="power"]');
            powerNode.textContent = powerLabel;
            powerNode.classList.add("power-state");
            applyPowerBadge(powerNode.closest("[data-role='power-badge']"), powerLabel);
            const powerTime = host.last_known_power_state_time ? new Date(host.last_known_power_state_time) : null;
            if (powerTime) {
                const formattedTime = dateTimeFormat.format(powerTime);
                ["power-updated", "power-updated-inline"].forEach((selector) => {
                    const node = frag.querySelector(`[data-field="${selector}"]`);
                    if (node) node.textContent = `As of ${formattedTime}`;
                });
            }

            // chips (system facts)
            frag.querySelector('[data-field="ip"]').textContent = host.management_ip;
            frag.querySelector('[data-field="mgmt_type"]').textContent = resolveEnum(mgmtTypes, host.management_type);
            frag.querySelector('[data-field="vendor"]').textContent = resolveEnum(vendorNames, host.vendor);

            // memory
            const mem = host.specs?.memory || {};
            frag.querySelector('[data-field="num_dimms"]').textContent = mem.num_dimms ?? "—";
            frag.querySelector('[data-field="size_gb"]').textContent = mem.size_gb ?? "—";
            frag.querySelector('[data-field="speed_mhz"]').textContent = mem.speed_mhz ?? "—";

            // processor
            const proc = host.specs?.processor || {};
            frag.querySelector('[data-field="manufacturer"]').textContent = proc.manufacturer ?? "—";
            frag.querySelector('[data-field="sku"]').textContent = cleanSku(proc.manufacturer ?? "", proc.sku ?? "—");
            frag.querySelector('[data-field="cores"]').textContent = proc.cores ?? "—";
            frag.querySelector('[data-field="count"]').textContent = proc.count ?? "—";
            frag.querySelector('[data-field="threads"]').textContent = proc.threads ?? "—";
            frag.querySelector('[data-field="processor_speed_mhz"]').textContent = `${proc.base_speed_mhz ?? "—"} / ${proc.max_speed_mhz ?? "—"}`;

            // storage
            const storageUl = frag.querySelector('[data-field="storage_list"]');
            storageUl.innerHTML = "";
            const storage = Array.isArray(host.specs?.storage) ? host.specs.storage : [];
            if (storage.length) {
                storage.forEach((dev) => {
                    const li = document.createElement("li");
                    li.textContent = renderStorageLine(dev);
                    storageUl.appendChild(li);
                });
            } else {
                const li = document.createElement("li");
                li.textContent = "No storage info";
                storageUl.appendChild(li);
            }
            const totalGB = totalCapacityGB(storage);
            frag.querySelector('[data-field="storage_total"]').textContent = prettyCapacityGB(totalGB);
            frag.querySelector('[data-field="storage_summary"]').textContent = `${storage.length} device${storage.length === 1 ? "" : "s"}`;

            list.appendChild(frag);
        });
    } catch (err) {
        console.error(err);
    }
}

let hostFormError = null;

if (hostFormElement) {
    hostFormError = document.createElement("p");
    hostFormError.className = "text-red-600 text-sm mb-1";
    hostFormError.style.display = "none";
    hostFormElement.prepend(hostFormError);
}

function setHostFormError(message) {
    if (!hostFormError) return;
    if (message) {
        hostFormError.textContent = message;
        hostFormError.style.display = "block";
    } else {
        hostFormError.textContent = "";
        hostFormError.style.display = "none";
    }
}

function setFormVisibility(formKey, shouldShow) {
    const form = forms[formKey];
    if (!form) return;

    form.classList.toggle("hidden", !shouldShow);
}

function resetHostForm() {
    if (hostFormElement) {
        hostFormElement.reset();

        const labelSpan = document.getElementById("mgmtSelectedLabel");
        const hiddenInput = document.getElementById("managementSelect");
        if (labelSpan) {
            labelSpan.textContent = "Select Management Type";
            labelSpan.classList.add("text-gray-400");
            labelSpan.classList.remove("text-font-primary");
        }
        if (hiddenInput) hiddenInput.value = "";
    }
    const spinner = document.getElementById("host-spinner");
    spinner?.classList.add("hidden");
    setHostFormError("");

    if (typeof closeMgmtMenu === "function") closeMgmtMenu();
}

function resetISOForm() {
    isoForm?.reset();
}

function closeForm(formKey) {
    setFormVisibility(formKey, false);
    if (formKey === "host") {
        resetHostForm();
    }
    if (formKey === "iso") {
        resetISOForm();
    }
}

function toggleAdminForm(formKey) {
    const targetForm = forms[formKey];
    const shouldOpen = targetForm ? targetForm.classList.contains("hidden") : false;

    Object.keys(forms).forEach((key) => {
        setFormVisibility(key, shouldOpen && key === formKey);
        if (!(shouldOpen && key === formKey)) {
            closeForm(key);
        }
    });
}

Object.entries(formTriggers).forEach(([key, trigger]) => {
    if (trigger) {
        trigger.addEventListener("click", () => toggleAdminForm(key));
    }
});

Object.entries(cancelButtons).forEach(([key, btn]) => {
    if (btn) {
        btn.addEventListener("click", () => closeForm(key));
    }
});

function getDeviceIP(element) {
    if (!element) return null;
    const section = element.closest("section");
    if (!section) return null;
    return section.querySelector('[data-field="ip"]');
}

function resetPowerMenu(menu) {
    if (!menu) return;
    const errorBox = menu.querySelector('[data-role="power-error"]');
    if (errorBox) {
        errorBox.textContent = "";
        errorBox.classList.add("hidden");
    }
    menu.querySelectorAll('button[aria-busy="true"]').forEach(btn => setPowerButtonLoading(btn, false));
}

function setPowerButtonLoading(button, isLoading, txt) {
    // const { label, spinner } = ensurePowerButtonStructure(button);

    // if (isLoading) {
    //     spinner.classList.remove("invisible");
    //     spinner.classList.add("is-active");
    //     button.setAttribute("aria-busy", "true");
    // } else {
    //     spinner.classList.add("invisible");
    //     spinner.classList.remove("is-active");
    //     button.removeAttribute("aria-busy");
    // }

    if (isLoading) {
        button.dataset.originalLabel = button.textContent;
        button.textContent = txt || "Processing...";
        button.disabled = true;
    } else {
        if (button.dataset.originalLabel) {
            button.textContent = button.dataset.originalLabel;
            delete button.dataset.originalLabel;
        }

        button.disabled = false;
    }
}

async function powerControl(button) {
    const btnText = (button.dataset.originalLabel || button.textContent || "").trim();
    const ipNode = getDeviceIP(button);
    if (!ipNode) return;
    const deviceAddress = ipNode.textContent;
    const menu = button.closest(".power-menu");
    const menuButtons = menu ? Array.from(menu.querySelectorAll("button")) : [];
    const errorBox = menu?.querySelector('[data-role="power-error"]');

    if (errorBox) {
        errorBox.textContent = "";
        errorBox.classList.add("hidden");
    }

    try {
        menuButtons.forEach((btn) => btn.disabled = true);

        setPowerButtonLoading(button, true, "Processing...");

        const powerActions = (await API.getPowerActions()).body;
        const powerAction = powerActions[btnText];
        const response = await API.postHostPowerControl(deviceAddress, powerAction);

        const message = response?.body?.message;
        const isOK = response.status_code === 200;
        if (!isOK) {
            const fallback = message || "Failed to change power state.";
            if (errorBox) {
                errorBox.textContent = fallback;
                errorBox.classList.remove("hidden");
            } else {
                alert(fallback);
            }
            return;
        } else {
            // body.power_state is new
            const newPowerState = resolveEnum(reverseObject((await API.getPowerStates()).body), response.body.power_state);
            const hostSection = button.closest("section");
            const powerNode = hostSection?.querySelector('[data-field="power"]');
            if (powerNode) {
                powerNode.textContent = newPowerState;
                applyPowerBadge(powerNode.closest("[data-role='power-badge']"), newPowerState);
            }
            const formattedTime = dateTimeFormat.format(new Date());
            ["power-updated", "power-updated-inline"].forEach((selector) => {
                const node = hostSection?.querySelector(`[data-field="${selector}"]`);
                if (node) node.textContent = `As of ${formattedTime}`;
            });
        }

        closeAllMenus();
    } catch (err) {
        console.error(err);
        if (errorBox) {
            errorBox.textContent = "Failed to change power state.";
            errorBox.classList.remove("hidden");
        } else {
            alert("Failed to change power state.");
        }
    } finally {
        menuButtons.forEach((btn) => {
            btn.disabled = false;
        });
        setPowerButtonLoading(button, false);
    }
}

window.powerControl = powerControl;

async function unenrollHost(button) {
    const ipNode = getDeviceIP(button);
    if (!ipNode) return;
    const deviceAddress = ipNode.textContent;

    const confirmRemoval = window.confirm(`Remove host ${deviceAddress} from inventory?`);
    if (!confirmRemoval) return;

    const defaultText = button.textContent;
    button.disabled = true;
    button.textContent = "Removing...";

    try {
        const response = await API.deleteHostByManagementIp(deviceAddress);
        if (response.status_code === 200) {
            const section = button.closest("section");
            section?.remove();
            if (!list.children.length) {
                emptyState?.classList.remove("hidden");
            }
        } else {
            alert(response?.body?.message || "Failed to remove host.");
            button.disabled = false;
            button.textContent = defaultText;
        }
    } catch (err) {
        console.error(err);
        alert("Failed to remove host.");
        button.disabled = false;
        button.textContent = defaultText;
    } finally {
        closeAllMenus();
    }
}
window.unenrollHost = unenrollHost;

if (hostFormElement) {
    hostFormElement.addEventListener("submit", async (e) => {
        e.preventDefault();
        const address = document.getElementById("addressInput").value;
        const managementType = document.getElementById("managementSelect").value;
        setHostFormError("");

        if (!validateIP(address)) {
            setHostFormError("Please enter a valid IPv4 address.");
            return;
        }

        const mgmtTypes = (await API.getManagementTypes()).body;
        const m = mgmtTypes[managementType];

        // Wait for host creation request then reload page
        document.getElementById("host-spinner").classList.toggle("hidden");
        const response = await API.postHostCreate(address, m);
        if (response.status_code !== 200) {
            document.getElementById("host-spinner").classList.toggle("hidden");
            console.log(response.body);
            const message = response?.body?.message || "Failed to add host. Please try again.";
            setHostFormError(message);
            return;
        }

        window.location.reload();
    });
}

function validateIP(address) {
    return /^(?!0)(?!.*\.$)((1?\d?\d|25[0-5]|2[0-4]\d)(\.|$)){4}$/.test(address);
}

if (isoForm) {
    isoForm.addEventListener("submit", uploadISO);
}

async function uploadISO(e) {
    e.preventDefault();
    if (!isoForm) return;
    const input = isoForm.querySelector('[name="iso-input"]');
    const file = input?.files?.[0];
    const fd = new FormData();

    if (!file) {
        return;
    }

    fd.append("iso_image", file, file.name);
    const response = await API.postIsoImage(fd);
    if (response.status_code === 200) {
        resetISOForm();
        closeForm("iso");
    } else {
        alert(response?.body?.message || "Failed to upload ISO.");
    }
}


function toggleMgmtMenu(btn) {
    const menu = document.getElementById("mgmtTypeMenu");
    const arrow = btn.querySelector("svg");

    closeAllMenus();

    const isClosed = menu.classList.contains("max-h-0");
    if (isClosed) {
        menu.classList.remove("max-h-0", "opacity-0");
        menu.classList.add("max-h-40", "opacity-100"); 
        arrow.style.transform = "rotate(180deg)";
    } else {
        closeMgmtMenu();
    }
}

function closeMgmtMenu() {
    const menu = document.getElementById("mgmtTypeMenu");
    const btn = document.getElementById("mgmtMenuBtn");

    if (menu && btn) {
        menu.classList.add("max-h-0", "opacity-0");
        menu.classList.remove("max-h-40", "opacity-100");

        const arrow = btn.querySelector("svg");
        if (arrow) arrow.style.transform = "";
    }
}

function selectMgmtType(value) {
    const hiddenInput = document.getElementById("managementSelect");
    const labelSpan = document.getElementById("mgmtSelectedLabel");

    if (hiddenInput && labelSpan) {
        hiddenInput.value = value;
        labelSpan.textContent = value;

        labelSpan.classList.remove("text-gray-400");
        labelSpan.classList.add("text-font-primary");
    }

    closeMgmtMenu();
}

window.toggleMgmtMenu = toggleMgmtMenu;
window.selectMgmtType = selectMgmtType;
