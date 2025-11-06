import { apiGet, apiPost, apiDelete } from "./util.js";

const known_uri = {
    hosts: () => "/api/hosts",
	hostByManagementIP: (management_ip) =>  known_uri.hosts() + management_ip,
	hostPowerAction: (management_ip, power_action) => `${known_uri.hostByManagementIP(management_ip)}/power/${power_action}`
};


export async function getAllHosts() {
    return await apiGet(known_uri.hosts());
}