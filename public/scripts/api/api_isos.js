import { apiGet, apiPostGeneric, apiDelete, known_uri } from "./util.js";

export async function getIsoImages() {
    return await apiGet(known_uri.iso_images());
}

export async function postIsoImage(isoImageFileData) {
    console.log(isoImageFileData)
    return await apiPostGeneric(known_uri.iso_images(), isoImageFileData);
}

export async function deleteIsoByName(name) {
    return await apiDelete(known_uri.iso_imagesByName(name));
}