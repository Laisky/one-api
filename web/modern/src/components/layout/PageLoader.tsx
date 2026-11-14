/**
 * PageLoader is the minimal fallback shown while a lazily loaded route chunk downloads.
 * It reserves vertical space so the surrounding layout (header, notices, footer) stays stable.
 *
 * @returns A centered, decorative spinner.
 */
export function PageLoader() {
  return (
    <div className="flex items-center justify-center min-h-[50vh]">
      <span className="h-8 w-8 animate-spin rounded-full border-4 border-primary/20 border-t-primary" aria-hidden="true" />
    </div>
  );
}
