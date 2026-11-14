import React from 'react';
import { createRoot } from 'react-dom/client';
import { useSystemStatus } from '@/hooks/useSystemStatus';
import { useAuthStore } from '@/lib/stores/auth';

/** StatusFixture renders real status lifecycle consumers with refresh and logout controls. */
function StatusFixture() {
  const { systemStatus, isSystemStatusLoading, refreshSystemStatus } = useSystemStatus();
  return <>
    <output id="name">{systemStatus.system_name || ''}</output>
    <output id="loading">{String(isSystemStatusLoading)}</output>
    <button onClick={() => { void refreshSystemStatus(); }}>Refresh</button>
    <button onClick={() => useAuthStore.getState().logout()}>Logout</button>
  </>;
}
createRoot(document.getElementById('root')!).render(<StatusFixture />);
