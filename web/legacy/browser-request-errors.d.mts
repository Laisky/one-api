/** RequestErrorCapture receives the original unexpected error and may explicitly decline capture. */
export type RequestErrorCapture = (error: unknown) => void | boolean;
/** registerRequestErrorCapture enables reporting only after SDK initialization and returns its owned cleanup. */
export function registerRequestErrorCapture(capture: RequestErrorCapture): () => void;
/** reportUnexpectedRequestError captures one unexpected error without changing request or UI control flow. */
export function reportUnexpectedRequestError(error: unknown): boolean;
