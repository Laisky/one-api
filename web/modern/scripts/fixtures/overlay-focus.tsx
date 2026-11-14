import React, { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { MobileDrawer } from '@/components/ui/mobile-drawer';
import { NotificationsProvider, useNotifications } from '@/components/ui/notifications';
import { Dialog, DialogContent, DialogTitle, DialogDescription, DialogTrigger } from '@/components/ui/dialog';
import '@/i18n';

/** OverlayFocusFixture renders real overlapping drawers, modal dialogs, and long-lived toast actions. */
export function OverlayFocusFixture() {
  const [a, setA] = useState(false);
  const [b, setB] = useState(false);
  const [modal, setModal] = useState(false);
  const [nested, setNested] = useState(false);
  const [mounted, setMounted] = useState(true);
  const [backgroundActions, setBackgroundActions] = useState(0);
  const { notify } = useNotifications();
  Object.assign(window, { overlayFixture: {
    openA: () => setA(true), closeA: () => setA(false),
    openTogether: () => { setA(true); setB(true); },
    openB: () => setB(true), closeB: () => setB(false),
    openModal: () => setModal(true), closeModal: () => setModal(false),
    unmountDrawers: () => setMounted(false), mountDrawers: () => { setA(false); setB(false); setMounted(true); },
    notify: () => notify({ id: 'focus-toast', message: 'Overlay notification', durationMs: 600000 }),
    notifyError: () => notify({ id: 'focus-error', message: 'Overlay error', type: 'error', durationMs: 600000 }),
  } });
  return <>
    <button id="open-a" onClick={() => setA(true)}>Open drawer A</button>
    <Dialog open={modal} onOpenChange={setModal}>
      <DialogTrigger asChild><button id="open-modal">Open modal</button></DialogTrigger>
      <DialogContent>
        <DialogTitle>Test modal</DialogTitle>
        <DialogDescription>Modal focus boundary</DialogDescription>
        <button id="modal-action">Modal action</button>
        <button id="modal-open-b" onClick={() => setB(true)}>Open drawer from modal</button>
        <button id="modal-close" onClick={() => setModal(false)}>Close modal</button>
      </DialogContent>
    </Dialog>
    <button id="background" onClick={() => setBackgroundActions((value) => value + 1)}>Background action</button>
    <output id="background-actions">{backgroundActions}</output>
    {mounted && <MobileDrawer isOpen={a} onClose={() => setA(false)} title="Drawer A" closeOnBackdropClick={false}>
      <button id="open-b" onClick={() => setB(true)}>Open drawer B</button>
      <Dialog open={nested} onOpenChange={setNested}>
        <DialogTrigger asChild><button id="nested-modal">Open nested modal</button></DialogTrigger>
        <DialogContent>
          <DialogTitle>Nested modal</DialogTitle>
          <DialogDescription>Nested modal focus boundary</DialogDescription>
          <button id="nested-action">Nested action</button>
          <button id="nested-close" onClick={() => setNested(false)}>Close nested modal</button>
        </DialogContent>
      </Dialog>
      <button id="a-last" onClick={() => setA(false)}>Close drawer A</button>
    </MobileDrawer>}
    {mounted && <MobileDrawer isOpen={b} onClose={() => setB(false)} title="Drawer B" closeOnBackdropClick={false}>
      <button id="b-first">B action</button>
      <button id="b-last" onClick={() => setB(false)}>Close drawer B</button>
    </MobileDrawer>}
  </>;
}

createRoot(document.getElementById('root')!).render(
  <NotificationsProvider><OverlayFocusFixture /></NotificationsProvider>
);
