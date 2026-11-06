import { useCallback, useEffect, useState } from 'react';
import { api } from '@/lib/api';
import { DEFAULT_GROUP, isValidGroupName } from '@/lib/group-name';

/** GroupCatalog is the assignable billing-group list plus its load state and a manual retry. */
export interface GroupCatalog {
  /** groups lists the names that may be newly assigned, DEFAULT_GROUP first; it is empty while loading or after a failure. */
  groups: string[];
  /** loading is true while a catalog request is in flight. */
  loading: boolean;
  /** error is true when the last request failed or returned an unusable response. */
  error: boolean;
  /** retry starts a new catalog request. */
  retry: () => void;
}

/** GroupCatalogLoadState is the request outcome that GroupCatalog exposes. */
type GroupCatalogLoadState = Omit<GroupCatalog, 'retry'>;

/**
 * normalizeGroupCatalog converts the `data` array of a successful `/api/group/`
 * response (the GroupRatio keys) into the names an editor may newly assign.
 * Names are kept exactly as returned, never trimmed, because a group name is an
 * identifier. Entries that are not strings or break the group-name rule are
 * dropped: the backend rejects assigning them, although legacy entities that
 * already carry such a name keep it. Duplicates are removed, DEFAULT_GROUP is
 * always listed first (it is assignable even when the catalog omits it), and
 * the remaining names are sorted so the order is stable across reloads.
 *
 * @param data - The `data` array returned by `/api/group/`.
 * @returns The ordered, assignable group names.
 */
export function normalizeGroupCatalog(data: readonly unknown[]): string[] {
  const names = new Set<string>();
  for (const entry of data) {
    if (typeof entry === 'string' && entry !== DEFAULT_GROUP && isValidGroupName(entry)) {
      names.add(entry);
    }
  }
  return [DEFAULT_GROUP, ...Array.from(names).sort((a, b) => a.localeCompare(b, 'en'))];
}

/**
 * appendAssignedGroups returns the catalog followed by the assigned groups that
 * are missing from it, without duplicates. An entity's stored group is
 * grandfathered by the backend even when it is not (or no longer) in the
 * catalog, so a single-value picker must still list it while the catalog is
 * loading, after it failed, or when the group was removed from GroupRatio;
 * otherwise the picker would render blank and look as if the group was cleared.
 *
 * @param catalog - The assignable groups from useGroupCatalog.
 * @param assigned - Groups already on the entity or in the form; empty and undefined entries are ignored.
 * @returns The ordered group names a picker should list.
 */
export function appendAssignedGroups(catalog: readonly string[], assigned: ReadonlyArray<string | undefined>): string[] {
  const choices = [...catalog];
  for (const group of assigned) {
    if (group && !choices.includes(group)) {
      choices.push(group);
    }
  }
  return choices;
}

/**
 * describeCatalogFailure turns a failed catalog request into a one-line string
 * for the browser console, so logs never contain objects.
 *
 * @param error - The value thrown while loading the catalog.
 * @returns A short description of the failure.
 */
function describeCatalogFailure(error: unknown): string {
  if (typeof error === 'object' && error !== null && 'response' in error) {
    const status = (error as { response?: { status?: unknown } }).response?.status;
    if (typeof status === 'number') {
      return `HTTP ${status}`;
    }
  }
  return error instanceof Error ? error.message : String(error);
}

/**
 * useGroupCatalog loads the configured billing groups from `/api/group/` for
 * the channel and user editors, which may only assign existing groups (groups
 * are created in Settings → Operation → Groups & Ratios). A network error, a
 * `success: false` response or a malformed payload is reported as an error with
 * an empty list rather than replaced by a guessed fallback, so callers can tell
 * the administrator and offer `retry`. A request that is superseded or whose
 * component unmounted never publishes its result.
 *
 * @returns The assignable groups, the loading and error flags, and a retry action.
 */
export function useGroupCatalog(): GroupCatalog {
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<GroupCatalogLoadState>({ groups: [], loading: true, error: false });

  const retry = useCallback(() => {
    setState((previous) => ({ groups: previous.groups, loading: true, error: false }));
    setAttempt((value) => value + 1);
  }, []);

  useEffect(() => {
    let cancelled = false;

    /** load fetches and validates one catalog response, publishing it only while still current. */
    const load = async () => {
      try {
        const response = await api.get('/api/group/');
        const body: unknown = response.data;
        if (typeof body !== 'object' || body === null) {
          throw new Error('empty response');
        }
        const { success, message, data } = body as { success?: unknown; message?: unknown; data?: unknown };
        if (success !== true) {
          throw new Error(typeof message === 'string' && message !== '' ? message : 'request was not successful');
        }
        if (!Array.isArray(data)) {
          throw new Error('response data is not a list');
        }
        if (cancelled) return;
        setState({ groups: normalizeGroupCatalog(data), loading: false, error: false });
      } catch (error) {
        if (cancelled) return;
        console.error(`Failed to load the group catalog: ${describeCatalogFailure(error)}`);
        setState({ groups: [], loading: false, error: true });
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [attempt]);

  return { ...state, retry };
}
