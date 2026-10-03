/** unregister retires only the matching CRA worker without waiting for readiness. */
export async function unregister() {
  if (!('serviceWorker' in navigator)) return;
  const script = new URL(`${process.env.PUBLIC_URL || ''}/service-worker.js`, window.location.origin);
  if (script.origin !== window.location.origin) return;
  const registrations = await navigator.serviceWorker.getRegistrations();
  const owned = registrations.filter((registration) =>
    [registration.active, registration.waiting, registration.installing]
      .some((worker) => worker && worker.scriptURL === script.href)
  );
  await Promise.all(owned.map((registration) => registration.unregister()));
}
