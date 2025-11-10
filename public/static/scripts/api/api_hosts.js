import { apiGet, apiPostJSON, apiDelete } from "./util.js";

const known_uri = {
    hosts: () => "/api/hosts",
	hostByManagementIP: (management_ip) =>  `${known_uri.hosts()}/${management_ip}`,
	hostPowerAction: (management_ip, power_action) => `${known_uri.hostByManagementIP(management_ip)}/power/${power_action}`
};


export async function getHostsAll() {
    return await apiGet(known_uri.hosts());
}

export async function getHostsByManagementIp(management_ip) {
    return await apiGet(known_uri.hostByManagementIP(management_ip));
}

export async function postHostCreate(management_ip, management_type) {
    
    return await apiPostJSON(
        known_uri.hosts(), await JSON.stringify({
            "management_ip" : management_ip,
            "management_type": management_type,
        })  
    );
}

export async function deleteHostByManagementIp(management_ip) {
    return await apiDelete(known_uri.hostByManagementIP(management_ip));
}

export async function postHostPowerControl(management_ip, power_action) {
    return await apiPostJSON(known_uri.hostPowerAction(management_ip, power_action));
}
