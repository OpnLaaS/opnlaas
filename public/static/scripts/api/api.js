/**
 * API Functions to be used when talking to the backed 
 * Generally if an api requires an enum value it is best to grab the enum from a separate API call then
 *      use returnValue[enumValue] to prevent any unwanted behavior as the endpoints expect the raw values
 * 
 * api/util.js should not be required by outside code,
 *      instead write wrapper in api folder so behavior is consistent 
 */

export * from './api_auth.js';
export * from './api_enums.js';
export * from './api_hosts.js';
export * from './api_isos.js';