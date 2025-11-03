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
    container.classList.add("max-h-100", "opacity-100");
    arrow.classList.add("rotate-180");
  } else {
    container.classList.add("max-h-0", "opacity-0");
    container.classList.remove("max-h-100", "opacity-100");
    arrow.classList.remove("rotate-180");
  }
}
window.toggleRow = toggleRow;

function reverseObject(obj) {
    return Object.fromEntries(Object.entries(obj).map(([k, v]) => [v, k]))
}

async function getEnums(url, endpoint) {
    const res = await fetch(`${url}/api/enums/${endpoint}`)
    const data = await res.json();
    return reverseObject(data)
}

document.addEventListener("DOMContentLoaded", async () => {
  // i know evan said try/catch is bad but whatever
  try {
    // this might be somewhere else idk
    const url = "http://localhost:8080";
    const res = await fetch(`${url}/api/hosts`);
    const data = await res.json();
    // get enums
    const vendorNames = await getEnums(url, 'vendors')
    const form_factors = await getEnums(url, 'form-factors')
    const mgmt_types = await getEnums(url, 'management-types')
    const power_states = await getEnums(url, 'power-states')


    console.log(data);

    data.forEach(host => {
        const clone = template.content.cloneNode(true)
        clone.querySelector('[data-field="form_factor"]').textContent = form_factors[host.form_factor]
        clone.querySelector('[data-field="power"]').textContent = power_states[host.last_known_power_state]
        clone.querySelector('[data-field="ip"]').textContent = host.management_ip
        clone.querySelector('[data-field="mgmt-type"]').textContent = mgmt_types[host.management_type]
        clone.querySelector('[data-field="name"]').textContent = host.model
        // memory
        clone.querySelector('[data-field="num_dimms"]').textContent = host.specs.memory.num_dimms
        clone.querySelector('[data-field="size_gb"]').textContent = host.specs.memory.size_gb
        clone.querySelector('[data-field="speed_mhz"]').textContent = host.specs.memory.speed_mhz
        // processor
        clone.querySelector('[data-field="cores"]').textContent = host.specs.processor.cores
        clone.querySelector('[data-field="count"]').textContent = host.specs.processor.count
        clone.querySelector('[data-field="sku"]').textContent = host.specs.processor.sku
        clone.querySelector('[data-field="threads"]').textContent = host.specs.processor.threads
        // storage
        // TODO needs forEach or somethin, just the first one so far
        clone.querySelector('[data-field="capacity_gb"]').textContent = host.specs.storage[0].capacity_gb
        clone.querySelector('[data-field="media_type"]').textContent = host.specs.storage[0].media_type



        clone.querySelector('[data-field="vendor"]').textContent = vendorNames[host.vendor]
        // clone.querySelector('[data-field=""]').textContent = host.
        tbody.appendChild(clone)
    })
  } catch (err) {
    console.error(err);
  }
});