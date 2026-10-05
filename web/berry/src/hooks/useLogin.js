import { API } from 'utils/api';
import { useDispatch } from 'react-redux';
import { LOGIN } from 'store/actions';
import { useNavigate } from 'react-router';
import { showSuccess } from 'utils/common';

const normalizeUser = (user) => {
  if (!user) return user;
  const uuid = user.uuid || user.user_uuid;
  return uuid ? { ...user, uuid, id: uuid } : user;
};

/**
 * isTotpRequired reports whether a login response (password, OAuth, or WeChat)
 * asks the user for a TOTP code before the session is created.
 *
 * @param {string|undefined} message is the `message` field of the login response.
 * @param {object|null|undefined} data is the `data` field of the login response.
 * @returns {boolean} true when the server answered with the `totp_required` challenge.
 */
export const isTotpRequired = (message, data) => message === 'totp_required' || data?.totp_required === true;

const useLogin = () => {
  const dispatch = useDispatch();
  const navigate = useNavigate();
  const login = async (username, password, totpCode = null) => {
    try {
      const loginData = { username, password };
      if (totpCode) {
        loginData.totp_code = totpCode;
      }

      const res = await API.post(`/api/user/login`, loginData);
      const { success, message, data } = res.data;
      if (success) {
        const user = normalizeUser(data);
        localStorage.setItem('user', JSON.stringify(user));
        dispatch({ type: LOGIN, payload: user });
        navigate('/panel');
      }
      return { success, message, data };
    } catch (err) {
      // 请求失败，设置错误信息
      return { success: false, message: '' };
    }
  };

  const githubLogin = async (code, state) => {
    try {
      const res = await API.get(`/api/oauth/github?code=${code}&state=${state}`);
      const { success, message, data } = res.data;
      if (success) {
        if (message === 'bind') {
          showSuccess('绑定成功！');
          navigate('/panel');
        } else {
          const user = normalizeUser(data);
          dispatch({ type: LOGIN, payload: user });
          localStorage.setItem('user', JSON.stringify(user));
          showSuccess('登录成功！');
          navigate('/panel');
        }
      }
      return { success, message, data };
    } catch (err) {
      // 请求失败，设置错误信息
      return { success: false, message: '' };
    }
  };

  const larkLogin = async (code, state) => {
    try {
      const res = await API.get(`/api/oauth/lark?code=${code}&state=${state}`);
      const { success, message, data } = res.data;
      if (success) {
        if (message === 'bind') {
          showSuccess('绑定成功！');
          navigate('/panel');
        } else {
          const user = normalizeUser(data);
          dispatch({ type: LOGIN, payload: user });
          localStorage.setItem('user', JSON.stringify(user));
          showSuccess('登录成功！');
          navigate('/panel');
        }
      }
      return { success, message, data };
    } catch (err) {
      // 请求失败，设置错误信息
      return { success: false, message: '' };
    }
  };

  const oidcLogin = async (code, state) => {
    try {
      const res = await API.get(`/api/oauth/oidc?code=${code}&state=${state}`);
      const { success, message, data } = res.data;
      if (success) {
        if (message === 'bind') {
          showSuccess('绑定成功！');
          navigate('/panel');
        } else {
          const user = normalizeUser(data);
          dispatch({ type: LOGIN, payload: user });
          localStorage.setItem('user', JSON.stringify(user));
          showSuccess('登录成功！');
          navigate('/panel');
        }
      }
      return { success, message, data };
    } catch (err) {
      // 请求失败，设置错误信息
      return { success: false, message: '' };
    }
  }

  const wechatLogin = async (code) => {
    try {
      const res = await API.post(`/api/oauth/wechat?code=${encodeURIComponent(code)}`);
      const { success, message, data } = res.data;
      if (success) {
        const user = normalizeUser(data);
        dispatch({ type: LOGIN, payload: user });
        localStorage.setItem('user', JSON.stringify(user));
        showSuccess('登录成功！');
        navigate('/panel');
      }
      return { success, message, data };
    } catch (err) {
      // 请求失败，设置错误信息
      return { success: false, message: '' };
    }
  };

  /**
   * oauthTotpLogin completes an OAuth or WeChat login that the server paused
   * with the `totp_required` challenge by posting the TOTP code to
   * /api/oauth/totp; on success it runs the regular login-success path.
   *
   * @param {string} totpCode is the 6-digit TOTP code entered by the user.
   * @returns {Promise<{success: boolean, message: string, data: any}>} the server
   * verdict; `data.totp_expired` is true when the pending sign-in expired.
   */
  const oauthTotpLogin = async (totpCode) => {
    try {
      const res = await API.post('/api/oauth/totp', { totp_code: totpCode });
      const { success, message, data } = res.data;
      if (success) {
        const user = normalizeUser(data);
        dispatch({ type: LOGIN, payload: user });
        localStorage.setItem('user', JSON.stringify(user));
        showSuccess('登录成功！');
        navigate('/panel');
      }
      return { success, message, data };
    } catch (err) {
      // The API interceptor already reported the failure (e.g. HTTP 429).
      return { success: false, message: '' };
    }
  };

  const logout = async () => {
    await API.post('/api/user/logout');
    localStorage.removeItem('user');
    dispatch({ type: LOGIN, payload: null });
    navigate('/');
  };

  return { login, logout, githubLogin, wechatLogin, larkLogin, oidcLogin, oauthTotpLogin };
};

export default useLogin;
