/**
 * API Functions to be used when talking to the backed 
 * Generally if an api requires an enum value it is best to grab the enum from a separate API call then
 *      use returnValue[enumValue] to prevent any unwanted behavior as the endpoints expect the raw values
 * 
 * api/util.js should not be required by outside code,
 *      instead write wrapper in api folder so behavior is consistent 
 */


import { getManagementTypes, getPowerActions, getVendors } from './api_enums.js';
import { deleteHostByManagementIp, getHostsAll, getHostsByManagementIp, postHostCreate, postHostPowerControl } from './api_hosts.js';
import { getIsoImages, postIsoImage } from './api_isos.js';
import { postLogout, postLogin } from './api_auth.js';


export * from "./api_auth.js";
export * from "./api_enums.js";
export * from "./api_hosts.js";
export * from "./api_isos.js";


window.getHostsAll = getHostsAll;
window.getHostsByManagementIp = getHostsByManagementIp;
window.getVendors = getVendors;
window.getIsoImages = getIsoImages;
window.postHostCreate = postHostCreate;
window.postLogout = postLogout;
window.postLogin = postLogin;
window.postIsoImage = postIsoImage;
window.deleteHost = deleteHostByManagementIp;
window.postPowerAction = postHostPowerControl;
window.getPowerAction = getPowerActions;
window.getManagementTypes = getManagementTypes;