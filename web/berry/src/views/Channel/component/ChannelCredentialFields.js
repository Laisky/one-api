import PropTypes from 'prop-types';
import { FormControl, InputLabel, OutlinedInput, TextField, FormHelperText, Container, Switch } from '@mui/material';

/**
 * ChannelCredentialFields renders the channel key input (single or batch), the batch-add switch
 * for new channels, and the per-type config fields. It owns no state: every value and handler
 * comes from EditModal and its Formik form.
 *
 * @param {object} props The component props.
 * @param {object} props.inputLabel The field labels for the selected channel type.
 * @param {object} props.inputPrompt The field prompts for the selected channel type.
 * @param {object} props.values The Formik form values.
 * @param {object} props.touched The Formik touched flags.
 * @param {object} props.errors The Formik validation errors.
 * @param {Function} props.handleBlur The Formik blur handler.
 * @param {Function} props.handleChange The Formik change handler.
 * @param {boolean} props.batchAdd Whether the key input accepts one key per line.
 * @param {Function} props.setBatchAdd EditModal's setter for batchAdd.
 * @param {number|string} [props.channelId] The edited channel's id; the batch switch is hidden when it is set.
 * @param {object} props.theme The MUI theme EditModal renders with.
 * @returns {JSX.Element} The credential and config fields.
 */
const ChannelCredentialFields = ({
  inputLabel,
  inputPrompt,
  values,
  touched,
  errors,
  handleBlur,
  handleChange,
  batchAdd,
  setBatchAdd,
  channelId,
  theme
}) => (
  <>
    {inputLabel.key && (
      <>
        <FormControl fullWidth error={Boolean(touched.key && errors.key)} sx={{ ...theme.typography.otherInput }}>
          {!batchAdd ? (
            <>
              <InputLabel htmlFor="channel-key-label">{inputLabel.key}</InputLabel>
              <OutlinedInput
                id="channel-key-label"
                label={inputLabel.key}
                type="text"
                value={values.key}
                name="key"
                onBlur={handleBlur}
                onChange={handleChange}
                inputProps={{}}
                aria-describedby="helper-text-channel-key-label"
              />
            </>
          ) : (
            <TextField
              multiline
              id="channel-key-label"
              label={inputLabel.key}
              value={values.key}
              name="key"
              onBlur={handleBlur}
              onChange={handleChange}
              aria-describedby="helper-text-channel-key-label"
              minRows={5}
              placeholder={inputPrompt.key + '，一行一个密钥'}
            />
          )}

          {touched.key && errors.key ? (
            <FormHelperText error id="helper-tex-channel-key-label">
              {errors.key}
            </FormHelperText>
          ) : (
            <FormHelperText id="helper-tex-channel-key-label"> {inputPrompt.key} </FormHelperText>
          )}
        </FormControl>
        {!channelId && (
          <Container
            sx={{
              textAlign: 'right'
            }}
          >
            <Switch checked={batchAdd} onChange={(e) => setBatchAdd(e.target.checked)} />
            批量添加
          </Container>
        )}
      </>
    )}

    {inputLabel.config &&
      Object.keys(inputLabel.config).map((configName) => {
        return (
          <FormControl key={'config.' + configName} fullWidth sx={{ ...theme.typography.otherInput }}>
            <TextField
              multiline
              key={'config.' + configName}
              name={'config.' + configName}
              value={values.config?.[configName] || ''}
              label={configName}
              placeholder={inputPrompt.config[configName]}
              onChange={handleChange}
            />
            <FormHelperText id={`helper-tex-config.${configName}-label`}> {inputPrompt.config[configName]} </FormHelperText>
          </FormControl>
        );
      })}
  </>
);

export default ChannelCredentialFields;

ChannelCredentialFields.propTypes = {
  inputLabel: PropTypes.object,
  inputPrompt: PropTypes.object,
  values: PropTypes.object,
  touched: PropTypes.object,
  errors: PropTypes.object,
  handleBlur: PropTypes.func,
  handleChange: PropTypes.func,
  batchAdd: PropTypes.bool,
  setBatchAdd: PropTypes.func,
  channelId: PropTypes.oneOfType([PropTypes.number, PropTypes.string]),
  theme: PropTypes.object
};
