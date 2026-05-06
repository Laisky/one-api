/**
 * PaginatedListResponse describes the backend pagination envelope for list endpoints.
 * It returns the success flag, optional message, page data, and total matching records.
 */
export interface PaginatedListResponse<T> {
	success?: boolean;
	message?: string;
	data?: T[];
	total?: number;
}

/**
 * PaginatedListRequester fetches a single paginated response for the provided URL.
 * It returns an object whose data field matches the backend pagination envelope.
 */
export type PaginatedListRequester<T> = (url: string) => Promise<{ data?: PaginatedListResponse<T> }>;

/**
 * fetchAllPaginatedResults retrieves every page for a paginated endpoint.
 * It accepts a requester, base path, base query parameters, and batch size, and returns the combined records.
 */
export async function fetchAllPaginatedResults<T>(
	requestPage: PaginatedListRequester<T>,
	path: string,
	baseParams: URLSearchParams,
	batchSize: number = 1000
): Promise<T[]> {
	const allRecords: T[] = [];
	let totalRecords = Number.POSITIVE_INFINITY;

	for (let page = 0; allRecords.length < totalRecords; page += 1) {
		const params = new URLSearchParams(baseParams);
		params.set('p', String(page));
		params.set('size', String(batchSize));

		const response = await requestPage(`${path}?${params.toString()}`);
		const payload = response.data;
		if (payload?.success === false) {
			throw new Error(payload.message || 'Failed to fetch paginated results.');
		}

		const pageRecords = Array.isArray(payload?.data) ? payload.data : [];
		if (typeof payload?.total === 'number' && Number.isFinite(payload.total)) {
			totalRecords = payload.total;
		} else if (totalRecords === Number.POSITIVE_INFINITY) {
			totalRecords = pageRecords.length;
		}

		allRecords.push(...pageRecords);

		if (pageRecords.length === 0) {
			break;
		}
	}

	return allRecords;
}
