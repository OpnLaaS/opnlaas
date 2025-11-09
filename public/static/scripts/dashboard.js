import { reverseObject } from "./lib/util.js";
import { URL } from "./lib/constants.js";

const list = document.getElementById("host-list");
const template = document.getElementById("host-item-template");
const emptyState = document.getElementById("empty");

function toggleItem(button) {
    const section = button.closest("section");
    const collapsible = section.querySelector(".transition-all");
    const arrow = button.querySelector("svg");
    const isCollapsed = collapsible.classList.contains("max-h-0");

    if (isCollapsed) {
        collapsible.classList.remove("max-h-0", "opacity-0");
        collapsible.classList.add("max-h-[1200px]", "opacity-100");
        arrow.style.transform = "rotate(180deg)";
    } else {
        collapsible.classList.add("max-h-0", "opacity-0");
        collapsible.classList.remove("max-h-[1200px]", "opacity-100");
        arrow.style.transform = "";
    }
}

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

async function getEnums(name) {
    const res = await fetch(`${URL}/api/enums/${name}`);
    if (!res.ok) throw new Error(`Failed to load enum: ${name}`);
    const obj = await res.json();
    return reverseObject(obj);
}

document.addEventListener("DOMContentLoaded", async () => {
    try {
        const res = await fetch(`${URL}/api/hosts`);
        if (!res.ok) throw new Error("Failed to load hosts");
        const data = await res.json();

        if (!Array.isArray(data) || data.length === 0) {
            emptyState?.classList.remove("hidden");
            return;
        }

        const vendorNames = await getEnums("vendors");
        const formFactors = await getEnums("form-factors");
        const mgmtTypes = await getEnums("management-types");
        const powerStates = await getEnums("power-states");

        list.innerHTML = "";
        data.forEach((host) => {
            const frag = template.content.cloneNode(true);

            // header
            frag.querySelector('[data-field="name"]').textContent = host.model;
            frag.querySelector('[data-field="form_factor"]').textContent = resolveEnum(formFactors, host.form_factor);
            frag.querySelector('[data-field="power"]').textContent = resolveEnum(powerStates, host.last_known_power_state);

            // chips (system facts)
            frag.querySelector('[data-field="ip"]').textContent = host.management_ip;
            frag.querySelector('[data-field="mgmt-type"]').textContent = resolveEnum(mgmtTypes, host.management_type);
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