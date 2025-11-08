import { reverseObject } from "./lib/util.js";
import { URL } from "./lib/constants.js";

// Pretty-print capacity in GB/TB
function prettyCapacityGB(gb) {
    if (gb == null || isNaN(gb)) return "Unknown";
    const n = Number(gb);
    if (n >= 1024) return (n / 1024).toFixed(1).replace(/\.0$/, "") + " TB";
    return n + " GB";
}

// Build a line for a single storage device
function renderStorageLine(dev) {
    if (!dev || typeof dev !== "object") return "Unknown device";
    const parts = [];
    // capacity
    if ("capacity_gb" in dev) parts.push(prettyCapacityGB(dev.capacity_gb));
    // media/interface/model
    if (dev.media_type) parts.push(String(dev.media_type).toUpperCase());
    if (dev.interface) parts.push(dev.interface);
    if (dev.model) parts.push(dev.model);
    return parts.filter(Boolean).join(" • ");
}


const tbody = document.getElementById("hosts");
const template = document.getElementById("host-row-template");

function toggleRow(button) {
    const tr = button.closest("tr");
    const nextRow = tr.nextElementSibling;
    const container = nextRow.querySelector("div");
    const arrow = button.querySelector("svg");
    const isCollapsed = container.classList.contains("max-h-0");

    if (isCollapsed) {
        container.classList.remove("max-h-0", "opacity-0");
        container.classList.add("h-max", "opacity-100");
        arrow.classList.add("rotate-180");
    } else {
        container.classList.add("max-h-0", "opacity-0");
        container.classList.remove("max-h-100", "opacity-100");
        arrow.classList.remove("rotate-180");
    }
}
window.toggleRow = toggleRow;

async function getEnums(endpoint) {
    return reverseObject(await (await fetch(`${URL}/api/enums/${endpoint}`)).json());
}

document.addEventListener("DOMContentLoaded", async () => {
    // i know evan said try/catch is bad but whatever
    try {
        const res = await fetch(`${URL}/api/hosts`);
        const data = await res.json();
        // get enums
        const vendorNames = await getEnums("vendors");
        const formFactors = await getEnums("form-factors");
        const mgmtTypes = await getEnums("management-types");
        const powerStates = await getEnums("power-states");

        data.forEach((host) => {
            const clone = template.content.cloneNode(true);
            clone.querySelector('[data-field="form_factor"]').textContent = formFactors[host.form_factor];
            clone.querySelector('[data-field="power"]').textContent = powerStates[host.last_known_power_state];
            clone.querySelector('[data-field="ip"]').textContent = host.management_ip;
            clone.querySelector('[data-field="mgmt-type"]').textContent = mgmtTypes[host.management_type];
            clone.querySelector('[data-field="name"]').textContent = host.model;
            // memory
            clone.querySelector('[data-field="num_dimms"]').textContent = host.specs.memory.num_dimms;
            clone.querySelector('[data-field="size_gb"]').textContent = host.specs.memory.size_gb;
            clone.querySelector('[data-field="speed_mhz"]').textContent = host.specs.memory.speed_mhz;
            // processor
            clone.querySelector('[data-field="manufacturer"]').textContent = host.specs.processor.manufacturer;
            clone.querySelector('[data-field="cores"]').textContent = host.specs.processor.cores;
            clone.querySelector('[data-field="count"]').textContent = host.specs.processor.count;
            clone.querySelector('[data-field="sku"]').textContent = host.specs.processor.sku;
            clone.querySelector('[data-field="threads"]').textContent = host.specs.processor.threads;
            clone.querySelector('[data-field="processor_speed_mhz"]').textContent = `${host.specs.processor.base_speed_mhz} / ${host.specs.processor.max_speed_mhz}`;
            // storage
            const storageUl = clone.querySelector('[data-field="storage_list"]');
            if (Array.isArray(host.specs?.storage) && storageUl) {
                storageUl.innerHTML = "";
                host.specs.storage.forEach((dev) => {
                    const li = document.createElement("li");
                    li.textContent = renderStorageLine(dev);
                    storageUl.appendChild(li);
                });
            }
            clone.querySelector('[data-field="vendor"]').textContent = vendorNames[host.vendor];
            // clone.querySelector('[data-field=""]').textContent = host.
            tbody.appendChild(clone);
        });
    } catch (err) {
        console.error(err);
    }
});