import { apiGet, apiPostJSON } from "./util.js";

const known_uri = {
    iso_images: () => "/api/iso-images",
};


export async function getIsoImages() {
    return await apiGet(known_uri.iso_images());
}

export async function postIsoImage(isoImageFileData) {
    return await apiPostJSON(known_uri.iso_images(), isoImageFileData)
}