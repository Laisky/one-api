/** omitUnownedChannelFields returns a copy of a channel update payload without the fields the legacy forms do not own. */
export declare function omitUnownedChannelFields<T extends Record<string, unknown>>(payload: T): Omit<T, 'testing_model'>;
