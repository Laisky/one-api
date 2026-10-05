import { showError as reportUIError } from '../../../utils/common';
import PropTypes from 'prop-types';
import React from 'react';
import { Box, Button, FormHelperText, TextField, Typography } from '@mui/material';
import { Formik, Form, Field } from 'formik';
import * as Yup from 'yup';
import { showError } from 'utils/common';
import useLogin from 'hooks/useLogin';
import AnimateButton from 'ui-component/extended/AnimateButton';

const totpCodePattern = /^\d{6}$/;

const validationSchema = Yup.object().shape({
  totp_code: Yup.string().matches(totpCodePattern, 'TOTP code must be 6 digits').required('TOTP code is required')
});

/**
 * OAuthTotpForm completes an OAuth or WeChat login that the server paused with
 * the `totp_required` challenge. It asks for the 6-digit TOTP code and submits
 * it through useLogin().oauthTotpLogin, which runs the regular login-success
 * path. A wrong code keeps the form open for another attempt; an expired
 * challenge is reported and onBack is called.
 *
 * @param {object} props are the component props.
 * @param {Function} props.onBack is called when the user leaves the challenge or after it expired.
 * @returns {JSX.Element} the rendered TOTP form.
 */
const OAuthTotpForm = ({ onBack }) => {
  const { oauthTotpLogin } = useLogin();

  /**
   * submit sends the entered TOTP code and maps the server verdict onto the form.
   *
   * @param {object} values are the Formik form values.
   * @param {object} helpers are the Formik helpers used to surface a rejected code.
   * @returns {Promise<void>} resolves once the verdict has been handled.
   */
  const submit = async (values, { setErrors }) => {
    const { success, message, data } = await oauthTotpLogin(values.totp_code.trim());
    if (success) {
      return;
    }
    if (data?.totp_expired) {
      showError(message);
      onBack();
      return;
    }
    if (message) {
      setErrors({ submit: message });
    }
  };

  return (
    <Formik
      initialValues={{ totp_code: '', submit: null }}
      validationSchema={validationSchema}
      onSubmit={(...uiArgs) => submit(...uiArgs).catch(reportUIError)}
    >
      {({ errors, touched, isSubmitting, values }) => (
        <Box component={Form} noValidate sx={{ width: '100%' }}>
          <Typography variant="body2" color="text.secondary" align="center" sx={{ mb: 2 }}>
            请输入您的TOTP验证码
          </Typography>
          <Field
            as={TextField}
            name="totp_code"
            label="TOTP验证码"
            fullWidth
            autoFocus
            inputProps={{ maxLength: 6, autoComplete: 'one-time-code', inputMode: 'numeric' }}
            error={touched.totp_code && Boolean(errors.totp_code)}
            helperText={touched.totp_code && errors.totp_code}
          />
          {errors.submit && (
            <Box sx={{ mt: 2 }}>
              <FormHelperText error>{errors.submit}</FormHelperText>
            </Box>
          )}
          <Box sx={{ mt: 2 }}>
            <AnimateButton>
              <Button
                disableElevation
                disabled={isSubmitting || !totpCodePattern.test(values.totp_code.trim())}
                fullWidth
                size="large"
                type="submit"
                variant="contained"
                color="primary"
              >
                验证TOTP
              </Button>
            </AnimateButton>
          </Box>
          <Box sx={{ mt: 1 }}>
            <Button disableElevation disabled={isSubmitting} fullWidth size="large" variant="outlined" color="secondary" onClick={onBack}>
              返回登录
            </Button>
          </Box>
        </Box>
      )}
    </Formik>
  );
};

export default OAuthTotpForm;

OAuthTotpForm.propTypes = {
  onBack: PropTypes.func
};
