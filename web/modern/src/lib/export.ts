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

export const DEFAULT_BATCH_CONCURRENCY = 5;

const normalizeConcurrency = (value: number): number => {
	if (!Number.isFinite(value)) {
		return DEFAULT_BATCH_CONCURRENCY;
	}

	return Math.max(1, Math.trunc(value));
};

/**
 * mapWithConcurrency processes items with a bounded amount of parallel work.
 * It preserves input order in the returned results while allowing up to `concurrency` active tasks.
 */
export async function mapWithConcurrency<T, R>(
	items: T[],
	mapper: (item: T, index: number) => Promise<R>,
	concurrency: number = DEFAULT_BATCH_CONCURRENCY
): Promise<R[]> {
	if (items.length === 0) {
		return [];
	}

	const results = new Array<R>(items.length);
	const workerCount = Math.min(normalizeConcurrency(concurrency), items.length);
	let nextIndex = 0;

	const worker = async () => {
		for (;;) {
			const currentIndex = nextIndex;
			nextIndex += 1;

			if (currentIndex >= items.length) {
				return;
			}

			results[currentIndex] = await mapper(items[currentIndex], currentIndex);
		}
	};

	await Promise.all(Array.from({ length: workerCount }, () => worker()));
	return results;
}

/**
 * fetchAllPaginatedResults retrieves every page for a paginated endpoint.
 * It accepts a requester, base path, base query parameters, and batch size, and returns the combined records.
 */
export async function fetchAllPaginatedResults<T>(
	requestPage: PaginatedListRequester<T>,
	path: string,
	baseParams: URLSearchParams,
	batchSize: number = 1000,
	concurrency: number = DEFAULT_BATCH_CONCURRENCY
): Promise<T[]> {
	const fetchPage = async (page: number): Promise<PaginatedListResponse<T>> => {
		const params = new URLSearchParams(baseParams);
		params.set('p', String(page));
		params.set('size', String(batchSize));

		const response = await requestPage(`${path}?${params.toString()}`);
		const payload = response.data;
		if (payload?.success === false) {
			throw new Error(payload.message || 'Failed to fetch paginated results.');
		}

		return payload || {};
	};

	const firstPage = await fetchPage(0);
	const firstPageRecords = Array.isArray(firstPage.data) ? firstPage.data : [];
	if (typeof firstPage.total !== 'number' || !Number.isFinite(firstPage.total)) {
		return firstPageRecords;
	}

	const totalPages = Math.ceil(firstPage.total / batchSize);
	if (totalPages <= 1) {
		return firstPageRecords;
	}

	const remainingPages = Array.from({ length: totalPages - 1 }, (_value, index) => index + 1);
	const pagePayloads = await mapWithConcurrency(
		remainingPages,
		async (page) => {
			const payload = await fetchPage(page);
			return Array.isArray(payload.data) ? payload.data : [];
		},
		concurrency
	);

	return [firstPageRecords, ...pagePayloads].flat();
}

const stringifyCsvValue = (value: unknown): string => {
	if (value === null || value === undefined) {
		return '';
	}

	if (typeof value === 'string') {
		return value;
	}

	if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') {
		return String(value);
	}

	return JSON.stringify(value);
};

/**
 * buildCsv serializes a row matrix into CSV text with proper escaping.
 * It quotes all fields so JSON payloads, commas, quotes, and newlines remain spreadsheet-safe.
 */
export function buildCsv(rows: unknown[][]): string {
	return rows
		.map((row) =>
			row
				.map((value) => `"${stringifyCsvValue(value).replace(/"/g, '""')}"`)
				.join(',')
		)
		.join('\n');
}
