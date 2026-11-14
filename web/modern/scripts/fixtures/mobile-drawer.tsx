// Offline browser fixture for the real MobileDrawer; no backend or credentials.
import React, { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { MobileDrawer } from '@/components/ui/mobile-drawer';
import '@/i18n';

/** DrawerFixture renders tall background content and a short or scrolling modal drawer. */
export function DrawerFixture() {
  const [open, setOpen] = useState(false);
  const [mounted, setMounted] = useState(true);
  const [long, setLong] = useState(false);
  Object.assign(window, {
    drawerFixture: {
      open: () => setOpen(true),
      close: () => setOpen(false),
      unmount: () => setMounted(false),
      mount: () => setMounted(true),
      long: () => setLong(true),
    },
  });
  return <>
    <button id="opener" onClick={() => setOpen(true)}>Open drawer</button>
    {Array.from({ length: 100 }, (_, index) => <p key={index}>Background row {index}</p>)}
    {mounted && <MobileDrawer isOpen={open} onClose={() => setOpen(false)} title="Test drawer">
      <a href="#drawer-end">Drawer link</a>
      {long && Array.from({ length: 100 }, (_, index) => <p key={index}>Drawer row {index}</p>)}
      <button id="drawer-end" onClick={() => setOpen(false)}>Done</button>
    </MobileDrawer>}
  </>;
}
createRoot(document.getElementById('root')!).render(<DrawerFixture />);
