import { showError as reportUIError } from '../helpers/utils';
import React, { useContext, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button, Form } from '@douyinfe/semi-ui';
import Text from '@douyinfe/semi-ui/lib/es/typography/text';
import { API, normalizeUser, showError, showSuccess } from '../helpers';
import { UserContext } from '../context/User';
import './OAuthTotpForm.css';

/**
 * OAuthTotpForm completes an OAuth or WeChat login that the server paused with
 * the `totp_required` challenge. It asks for the 6-digit TOTP code, submits it
 * to POST /api/oauth/totp, and on success runs the regular login-success path
 * (user context, persisted user, redirect to the home page). A wrong code keeps
 * the form open for another attempt; an expired challenge calls onBack.
 *
 * Parameters:
 *   - onBack: function, called when the user leaves the challenge or after the
 *     server reports that the pending sign-in expired.
 *
 * Return value: the rendered TOTP form.
 */
const OAuthTotpForm = ({ onBack }) => {
  const [, userDispatch] = useContext(UserContext);
  const [totpCode, setTotpCode] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  /**
   * submitTotp sends the entered code to the server and handles the outcome.
   * It returns a promise that rejects only on transport failures, which the
   * UI event boundary reports.
   */
  const submitTotp = async () => {
    setLoading(true);
    try {
      const res = await API.post('/api/oauth/totp', { totp_code: totpCode });
      const { success, message, data } = res.data;
      if (success) {
        const user = normalizeUser(data);
        userDispatch({ type: 'login', payload: user });
        localStorage.setItem('user', JSON.stringify(user));
        showSuccess('登录成功！');
        navigate('/');
        return;
      }
      showError(message);
      if (data?.totp_expired) {
        onBack();
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="oauth-totp-form">
      <Text className="oauth-totp-form__hint">请输入您的TOTP验证码</Text>
      <Form>
        <Form.Input
          field={'totp_code'}
          label={'TOTP验证码'}
          placeholder="请输入6位验证码"
          name="totp_code"
          maxLength={6}
          autoComplete="one-time-code"
          onChange={(value) => setTotpCode(value.trim())}
        />
        <Button theme="solid" type={'primary'} size="large" block htmlType={'submit'} loading={loading}
                onClick={(...uiArgs) => submitTotp(...uiArgs).catch(reportUIError)}
                disabled={loading || !/^\d{6}$/.test(totpCode)}>
          验证TOTP
        </Button>
        <Button className="oauth-totp-form__cancel" theme="solid" type={'tertiary'} size="large" block
                disabled={loading} onClick={onBack}>
          返回登录
        </Button>
      </Form>
    </div>
  );
};

export default OAuthTotpForm;
