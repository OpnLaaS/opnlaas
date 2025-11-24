import { reverseObject } from "./lib/util.js";
import * as API from "./api/api.js";

const list = document.getElementById("host-list");
const template = document.getElementById("host-item-template");
const emptyState = document.getElementById("empty");

function toggleItem(button) {
    const section = button.closest("section");
    const collapsible = section.querySelector(".transition-all:not(.power-menu)");
    const arrow = button.querySelector("svg");
    const isCollapsed = collapsible.classList.contains("max-h-0");

    if (isCollapsed) {
        collapsible.classList.remove("max-h-0", "opacity-0");
        collapsible.classList.add("max-h-100", "opacity-100");
        arrow.style.transform = "rotate(180deg)";
    } else {
        collapsible.classList.add("max-h-0", "opacity-0");
        collapsible.classList.remove("max-h-100", "opacity-100");
        arrow.style.transform = "";
    }
}

function togglePowerMenu(button) {
    const wrapper = button.closest("div.relative");
    const menu = wrapper.querySelector(".power-menu");
    const isClosed = menu.classList.contains("max-h-0");

    closeAllMenus();
    if (isClosed) {
        menu.classList.remove("max-h-0", "opacity-0");
        menu.classList.add("max-h-70", "opacity-100");
    }
}

function closeAllMenus() {
    document.querySelectorAll(".power-menu").forEach(menu => {
        menu.classList.add("max-h-0", "opacity-0");
        menu.classList.remove("max-h-70", "opacity-100");
    });
}

document.addEventListener("click", function (event) {
    if (event.target.closest(".power-menu") || event.target.closest(".power-button")) {
        return;
    }

    closeAllMenus();
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
    try {
        const hostData = await API.getHostsAll();
        // get enums
        const vendorsRes = await API.getVendors();
        const vendorNames = reverseObject(vendorsRes.body || {});

        const formFactorsRes = await API.getFormFactors();
        const formFactors = reverseObject(formFactorsRes.body || {});

        const mgmtTypesRes = await API.getManagementTypes();
        const mgmtTypes = reverseObject(mgmtTypesRes.body || {});

        const powerStatesRes = await API.getPowerStates();
        const powerStates = reverseObject(powerStatesRes.body || {});

        list.innerHTML = "";
        hostData.body.forEach((host) => {
            const frag = template.content.cloneNode(true);

            // header
            frag.querySelector('[data-field="name"]').textContent = host.model;
            frag.querySelector('[data-field="form_factor"]').textContent = resolveEnum(formFactors, host.form_factor);
            frag.querySelector('[data-field="power"]').textContent = resolveEnum(powerStates, host.last_known_power_state);

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

const addHostBtn = document.getElementById("addHostBtn");
const newHostForm = document.getElementById("newHostForm");

function hideForm(form) {
    console.log(form);
    if (form.classList.contains("max-h-0")) {
        form.classList.remove("max-h-0", "opacity-0");
        form.classList.add("max-h-80", "opacity-100");
    } else {
        form.classList.remove("max-h-80", "opacity-100");
        form.classList.add("max-h-0", "opacity-0");
    }
}
window.hideForm = hideForm;

if (addHostBtn) {
    addHostBtn.addEventListener('click', hideForm);
}

function getDeviceIP(element) {
    if (!element) return null;
    const section = element.closest("section");
    if (!section) return null;
    return section.querySelector('[data-field="ip"]');
}

async function powerControl(button) {
    const btnText = button.textContent;
    const device_address = getDeviceIP(button).textContent;

    const powerActions = (await API.getPowerActions()).body;
    const power_action = powerActions[btnText];
    
    API.postHostPowerControl(device_address, power_action);
}
window.powerControl = powerControl;

const addHostForm = newHostForm;
addHostForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    const address = document.getElementById("addressInput").value;
    const managementType = document.getElementById("managementSelect").value;

    if (!validateIP(address)) {
        //TODO add error message on webpage
        return;
    }

    const mgmtTypes = (await API.getManagementTypes()).body;
    const m = mgmtTypes[managementType];

    // Wait for host creation request then reload page
    document.getElementById("host-spinner").classList.toggle("hidden");
    const response = await API.postHostCreate(address, m);
    if (response.status_code !== 200) {
        // TODO show error message on webpage
        return;
    }

    window.location.reload();
});

function validateIP(address) {
    return /^(?!0)(?!.*\.$)((1?\d?\d|25[0-5]|2[0-4]\d)(\.|$)){4}$/.test(address);
}

const isoForm = document.getElementById("uploadISOForm");

function uploadISO(e) {
    e.preventDefault();
    const input = isoForm.querySelector('[name="iso-input"]');
    const file = input?.files?.[0];
    const fd = new FormData();

    if (!file) {
        return;
    }

    fd.append("iso_image", file, file.name);
    API.postIsoImage(fd);
}

window.uploadISO = uploadISO;

document.getElementById("submitISOBtn").addEventListener('click', uploadISO);
