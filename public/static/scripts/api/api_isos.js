import { apiGet, apiPostGeneric, known_uri } from "./util.js";

export async function getIsoImages() {
    return await apiGet(known_uri.iso_images());
}

export async function postIsoImage(isoImageFileData) {
    return await apiPostGeneric(known_uri.iso_images(), isoImageFileData);
}