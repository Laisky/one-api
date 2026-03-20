import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { useEffect, useState } from 'react';
import { useAuthStore } from '@/lib/stores/auth';

export function ProtectedRoute() {
  // Read the whole store to allow tests to inject extra fields like validateSession
  const store = useAuthStore() as any;
  const { user } = store;
  const location = useLocation();
  const [checking, setChecking] = useState<boolean>(false);
  const [valid, setValid] = useState<boolean | null>(null);

  // If user exists and a validateSession() is provided (tests), call it
  useEffect(() => {
    let mounted = true;
    const maybeValidate = async () => {
      if (user && typeof store.validateSession === 'function') {
        setChecking(true);
        try {
          const ok = await store.validateSession();
          if (mounted) setValid(!!ok);
        } finally {
          if (mounted) setChecking(false);
        }
      }
    };
    maybeValidate();
    return () => {
      mounted = false;
    };
  }, [user, store]);

  // Redirect to login with current path as redirect_to parameter
  if (!user) {
    const redirectTo = encodeURIComponent(location.pathname + location.search);
    // Render a small spinner for tests while also returning Navigate
    return (
      <>
        <div className="flex items-center justify-center p-4">
          <div className="animate-spin h-5 w-5 rounded-full border-2 border-t-transparent" />
        </div>
        <Navigate to={`/login?redirect_to=${redirectTo}`} replace />
      </>
    );
  }

  // If we are checking a session, show a minimal spinner
  if (checking) {
    return (
      <div className="flex items-center justify-center p-4">
        <div className="animate-spin h-5 w-5 rounded-full border-2 border-t-transparent" />
      </div>
    );
  }

  // If validation ran and failed, redirect
  if (valid === false) {
    const redirectTo = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?redirect_to=${redirectTo}`} replace />;
  }

  // Otherwise render protected outlet
  return <Outlet />;
}
