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
      return { success, message };
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
      return { success, message };
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
      return { success, message };
    } catch (err) {
      // 请求失败，设置错误信息
      return { success: false, message: '' };
    }
  }

  const wechatLogin = async (code) => {
    try {
      const res = await API.get(`/api/oauth/wechat?code=${code}`);
      const { success, message, data } = res.data;
      if (success) {
        const user = normalizeUser(data);
        dispatch({ type: LOGIN, payload: user });
        localStorage.setItem('user', JSON.stringify(user));
        showSuccess('登录成功！');
        navigate('/panel');
      }
      return { success, message };
    } catch (err) {
      // 请求失败，设置错误信息
      return { success: false, message: '' };
    }
  };

  const logout = async () => {
    await API.get('/api/user/logout');
    localStorage.removeItem('user');
    dispatch({ type: LOGIN, payload: null });
    navigate('/');
  };

  return { login, logout, githubLogin, wechatLogin, larkLogin,oidcLogin };
};

export default useLogin;
