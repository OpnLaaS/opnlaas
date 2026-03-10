import { apiGet, known_uri } from "./util.js";

export async function getOperationByID(operation_id) {
    return await apiGet(known_uri.operations_operationByID(operation_id));
}
