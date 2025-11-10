import { apiGet } from "./util.js";

const known_uri = {
    vendors: () => "/api/enums/vendors",
    formFactors: () => "/api/enums/form-factors",
    managementTypes: () => "/api/enums/management-types",
    powerStates: () => "/api/enums/power-states",
    bootModes: () => "/api/enums/boot-modes",
    powerActions: () => "/api/enums/power-actions",
    architectures: () => "/api/enums/architectures",
    distroTypes: () => "/api/enums/architectures",
    preconfigureTypes: () => "/api/enums/preconfigure-types", 
};


export async function getVendors() {
    return await apiGet(known_uri.vendors());
}

export async function getFormFactors() {
    return await apiGet(known_uri.formFactors());
}

export async function getManagementTypes() {
    return await apiGet(known_uri.managementTypes());
}

export async function getPowerStates() {
    return await apiGet(known_uri.powerStates());
}

export async function getBootModes() {
    return await apiGet(known_uri.bootModes());
}

export async function getPowerActions() {
    return await apiGet(known_uri.powerActions());
}

export async function getArchitectures() {
    return await apiGet(known_uri.architectures());
}

export async function getDistroTypes() {
    return await apiGet(known_uri.distroTypes());
}

export async function getPreconfigureTypes() {
    return await apiGet(known_uri.preconfigureTypes());
}
