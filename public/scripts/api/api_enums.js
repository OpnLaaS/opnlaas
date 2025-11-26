import { apiGet, known_uri } from "./util.js";

export async function getVendors() {
    return await apiGet(known_uri.enum_vendors());
}

export async function getFormFactors() {
    return await apiGet(known_uri.enum_formFactors());
}

export async function getManagementTypes() {
    return await apiGet(known_uri.enum_managementTypes());
}

export async function getPowerStates() {
    return await apiGet(known_uri.enum_powerStates());
}

export async function getBootModes() {
    return await apiGet(known_uri.enum_bootModes());
}

export async function getPowerActions() {
    return await apiGet(known_uri.enum_powerActions());
}

export async function getArchitectures() {
    return await apiGet(known_uri.enum_architectures());
}

export async function getDistroTypes() {
    return await apiGet(known_uri.enum_distroTypes());
}

export async function getPreconfigureTypes() {
    return await apiGet(known_uri.enum_preconfigureTypes());
}
