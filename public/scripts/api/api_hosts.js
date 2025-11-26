import { apiGet, apiPostJSON, apiDelete, known_uri } from "./util.js";

export async function getHostsAll() {
    return await apiGet(known_uri.hosts_hosts());
}

export async function getHostsByManagementIp(management_ip) {
    return await apiGet(known_uri.hosts_hostByManagementIP(management_ip));
}

export async function postHostCreate(management_ip, management_type) {
    
    return await apiPostJSON(
        known_uri.hosts_hosts(), await JSON.stringify({
            "management_ip" : management_ip,
            "management_type": management_type,
        })  
    );
}

export async function deleteHostByManagementIp(management_ip) {
    return await apiDelete(known_uri.hosts_hostByManagementIP(management_ip));
}

export async function postHostPowerControl(management_ip, power_action) {
    return await apiPostJSON(known_uri.hosts_hostPowerAction(management_ip, power_action));
}
