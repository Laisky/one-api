// WechatModal.js
import { showError as reportUIError } from '../../../utils/common';
import PropTypes from 'prop-types';
import React, { useState } from 'react';
import { Dialog, DialogTitle, DialogContent, TextField, Button, Typography, Grid } from '@mui/material';
import { Formik, Form, Field } from 'formik';
import { showError } from 'utils/common';
import * as Yup from 'yup';
import { isTotpRequired } from 'hooks/useLogin';
import OAuthTotpForm from './OAuthTotpForm';

const validationSchema = Yup.object().shape({
  code: Yup.string().required('验证码不能为空')
});

const WechatModal = ({ open, handleClose, wechatLogin, qrCode }) => {
  const [totpRequired, setTotpRequired] = useState(false);

  /**
   * closeModal hides the dialog and drops any pending TOTP step so the next
   * opening starts from the WeChat verification code again.
   */
  const closeModal = () => {
    setTotpRequired(false);
    handleClose();
  };

  /**
   * handleSubmit exchanges the WeChat verification code for a session, or
   * switches to the TOTP step when the account has two-factor enabled.
   *
   * @param {object} values are the Formik form values holding the WeChat code.
   * @returns {Promise<void>} resolves once the login result has been handled.
   */
  const handleSubmit = async (values) => {
    const { success, message, data } = await wechatLogin(values.code);
    if (success) {
      closeModal();
    } else if (isTotpRequired(message, data)) {
      // The WeChat code is consumed by now; finish the sign-in with TOTP.
      setTotpRequired(true);
    } else if (message) {
      showError(message);
    }
  };

  if (totpRequired) {
    return (
      <Dialog open={open} onClose={closeModal}>
        <DialogTitle>双因子认证 (TOTP)</DialogTitle>
        <DialogContent>
          <OAuthTotpForm onBack={closeModal} />
        </DialogContent>
      </Dialog>
    );
  }

  return (
    <Dialog open={open} onClose={closeModal}>
      <DialogTitle>微信验证码登录</DialogTitle>
      <DialogContent>
        <Grid container direction="column" alignItems="center">
          <img src={qrCode} alt="二维码" style={{ maxWidth: '300px', maxHeight: '300px', width: 'auto', height: 'auto' }} />
          <Typography
            variant="body2"
            color="text.secondary"
            style={{ marginTop: '10px', textAlign: 'center', wordWrap: 'break-word', maxWidth: '300px' }}
          >
            请使用微信扫描二维码关注公众号，输入「验证码」获取验证码（三分钟内有效）
          </Typography>
          <Formik
            initialValues={{ code: '' }}
            validationSchema={validationSchema}
            onSubmit={(...uiArgs) => handleSubmit(...uiArgs).catch(reportUIError)}
          >
            {({ errors, touched }) => (
              <Form style={{ width: '100%' }}>
                <Grid item xs={12}>
                  <Field
                    as={TextField}
                    name="code"
                    label="验证码"
                    error={touched.code && Boolean(errors.code)}
                    helperText={touched.code && errors.code}
                    fullWidth
                  />
                </Grid>
                <Grid item xs={12}>
                  <Button type="submit" fullWidth>
                    提交
                  </Button>
                </Grid>
              </Form>
            )}
          </Formik>
        </Grid>
      </DialogContent>
    </Dialog>
  );
};

export default WechatModal;

WechatModal.propTypes = {
  open: PropTypes.bool,
  handleClose: PropTypes.func,
  wechatLogin: PropTypes.func,
  qrCode: PropTypes.string
};
