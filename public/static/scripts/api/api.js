import { getManagementTypes, getPowerActions, getVendors } from './api_enums.js';
import { deleteHostByManagementIp, getHostsAll, getHostsByManagementIp, postHostCreate, postHostPowerControl } from './api_hosts.js';
import { getIsoImages, postIsoImage } from './api_isos.js';
import { postLogout, postLogin } from './api_auth.js';

export * from './api_auth.js';
export * from './api_enums.js';
export * from './api_hosts.js';
export * from './api_isos.js';
export * from './util.js';



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