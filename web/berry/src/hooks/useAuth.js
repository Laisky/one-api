import { useEffect } from 'react';
import { isAdmin } from 'utils/common';
import { useNavigate } from 'react-router-dom';

/** useAuth redirects non-admin users after render within the current router. */
const useAuth = () => {
  const navigate = useNavigate();
  const userIsAdmin = isAdmin();
  useEffect(() => {
    if (!userIsAdmin) navigate('/panel/404');
  }, [navigate, userIsAdmin]);
};

export default useAuth;
