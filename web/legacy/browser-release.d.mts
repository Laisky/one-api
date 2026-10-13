/** browserRelease returns an immutable SHA256 identity of enumerated public frontend build inputs. */
export function browserRelease(theme: string, buildValues: Record<string, string>): string;
/** readReleaseInput reads a safe enumerated regular source/build file without following symlinks. */
export function readReleaseInput(name: string, inputRoot?: string): Uint8Array;
/** releaseInputGuard returns a Vite plugin enforcing completeness of actual first-party source and public inputs. */
export function releaseInputGuard(theme: string): { name: string; enforce: 'pre'; buildStart(): void; generateBundle(): void; load(id: string): null; transform(code: string, id: string): null };
/** browserBuildContract supplies an immutable release and a Vite plugin checking the identical source snapshot before artifact writes. */
export function browserBuildContract(theme: string, buildValues: Record<string, string>): { release: string; plugin: ReturnType<typeof releaseInputGuard> };
