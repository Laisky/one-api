import PropTypes from 'prop-types';
import { Box, Button, FormControl, FormHelperText, InputLabel, TextField, Typography } from '@mui/material';
import LabelWithTooltip from './LabelWithTooltip';

/**
 * ChannelAdvancedFields renders the model mapping editor, the system prompt, the model configs
 * editor and, for AWS channels, the inference profile ARN map. It owns no state: every value and
 * handler comes from EditModal and its Formik form.
 *
 * @param {object} props The component props.
 * @param {object} props.values The Formik form values.
 * @param {object} props.touched The Formik touched flags.
 * @param {object} props.errors The Formik validation errors.
 * @param {Function} props.handleBlur The Formik blur handler.
 * @param {Function} props.handleChange The Formik change handler.
 * @param {Function} props.setFieldValue The Formik setter for one field.
 * @param {object} props.inputLabel The field labels for the selected channel type.
 * @param {object} props.inputPrompt The field prompts for the selected channel type.
 * @param {{model_configs: string}} props.defaultPricing The default model configs loaded for the channel type.
 * @param {Function} props.formatJSON EditModal's helper that pretty-prints JSON text, returning invalid text unchanged.
 * @param {Function} props.isValidJSON EditModal's helper that reports whether text is empty or valid JSON.
 * @param {object} props.theme The MUI theme EditModal renders with.
 * @returns {JSX.Element} The advanced model settings fields.
 */
const ChannelAdvancedFields = ({
  values,
  touched,
  errors,
  handleBlur,
  handleChange,
  setFieldValue,
  inputLabel,
  inputPrompt,
  defaultPricing,
  formatJSON,
  isValidJSON,
  theme
}) => (
  <>
    <FormControl fullWidth error={Boolean(touched.model_mapping && errors.model_mapping)} sx={{ ...theme.typography.otherInput }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
        <LabelWithTooltip
          htmlFor="channel-model_mapping-label"
          label={inputLabel.model_mapping}
          helpText="将传入的模型请求重定向到不同的模型。例如，将'gpt-4-0314'映射到'gpt-4'以处理已弃用的模型名称。JSON格式：{&quot;请求模型&quot;: &quot;实际模型&quot;}"
          sx={{
            position: 'relative',
            transform: 'none',
            fontSize: '0.875rem',
            fontWeight: 500,
            color: theme.palette.text.primary
          }}
        />
        <Box>
          <Button
            size="small"
            variant="outlined"
            onClick={() => {
              const formattedValue = formatJSON(values.model_mapping);
              setFieldValue('model_mapping', formattedValue);
            }}
            disabled={!values.model_mapping || values.model_mapping.trim() === ''}
          >
            格式化JSON
          </Button>
        </Box>
      </Box>
      <TextField
        multiline
        id="channel-model_mapping-label"
        value={values.model_mapping}
        name="model_mapping"
        onBlur={handleBlur}
        onChange={handleChange}
        aria-describedby="helper-text-channel-model_mapping-label"
        minRows={5}
        placeholder={inputPrompt.model_mapping}
        sx={{
          '& .MuiInputBase-input': {
            fontFamily: 'JetBrains Mono, Consolas, Monaco, "Courier New", monospace',
            fontSize: '13px',
            lineHeight: '1.4',
            backgroundColor: '#f8f9fa',
          },
          '& .MuiOutlinedInput-root': {
            '& fieldset': {
              borderColor: isValidJSON(values.model_mapping) ? theme.palette.grey[300] : theme.palette.error.main,
            },
          }
        }}
      />
      {touched.model_mapping && errors.model_mapping ? (
        <FormHelperText error id="helper-tex-channel-model_mapping-label">
          {errors.model_mapping}
        </FormHelperText>
      ) : (
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <FormHelperText id="helper-tex-channel-model_mapping-label">
            {inputPrompt.model_mapping}
          </FormHelperText>
          {values.model_mapping && values.model_mapping.trim() !== '' && (
            <Typography
              variant="caption"
              sx={{
                color: isValidJSON(values.model_mapping) ? theme.palette.success.main : theme.palette.error.main,
                fontWeight: 'bold',
                fontSize: '11px'
              }}
            >
              {isValidJSON(values.model_mapping) ? '✓ 有效JSON' : '✗ 无效JSON'}
            </Typography>
          )}
        </Box>
      )}
    </FormControl>
    <FormControl fullWidth error={Boolean(touched.system_prompt && errors.system_prompt)} sx={{ ...theme.typography.otherInput }}>
      <LabelWithTooltip
        htmlFor="channel-system_prompt-label"
        label={inputLabel.system_prompt}
        helpText="为通过此渠道的所有请求强制设置特定的系统提示词。适用于创建专门的AI助手或强制执行特定的行为模式。"
        sx={{
          position: 'relative',
          transform: 'none',
          fontSize: '0.875rem',
          fontWeight: 500,
          color: theme.palette.text.primary,
          mb: 1
        }}
      />
      <TextField
        multiline
        id="channel-system_prompt-label"
        value={values.system_prompt}
        name="system_prompt"
        onBlur={handleBlur}
        onChange={handleChange}
        aria-describedby="helper-text-channel-system_prompt-label"
        minRows={5}
        placeholder={inputPrompt.system_prompt}
      />
      {touched.system_prompt && errors.system_prompt ? (
        <FormHelperText error id="helper-tex-channel-system_prompt-label">
          {errors.system_prompt}
        </FormHelperText>
      ) : (
        <FormHelperText id="helper-tex-channel-system_prompt-label"> {inputPrompt.system_prompt} </FormHelperText>
      )}
    </FormControl>

    {/* Channel-specific pricing fields - now handled through model_configs */}

    <FormControl fullWidth error={Boolean(touched.model_configs && errors.model_configs)} sx={{ ...theme.typography.otherInput }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
        <LabelWithTooltip
          htmlFor="channel-model_configs-label"
          label="模型配置"
          helpText="为每个模型配置定价和限制。'ratio'设置输入token成本，'completion_ratio'设置输出token成本倍数，'max_tokens'设置请求限制。覆盖默认定价。"
          sx={{
            position: 'relative',
            transform: 'none',
            fontSize: '0.875rem',
            fontWeight: 500,
            color: theme.palette.text.primary
          }}
        />
        <Box>
          <Button
            size="small"
            variant="outlined"
            onClick={() => {
              const formattedValue = formatJSON(defaultPricing.model_configs);
              setFieldValue('model_configs', formattedValue);
            }}
            sx={{ mr: 1 }}
          >
            加载默认值
          </Button>
          <Button
            size="small"
            variant="outlined"
            onClick={() => {
              const formattedValue = formatJSON(values.model_configs);
              setFieldValue('model_configs', formattedValue);
            }}
            disabled={!values.model_configs || values.model_configs.trim() === ''}
          >
            格式化JSON
          </Button>
        </Box>
      </Box>
      <TextField
        multiline
        id="channel-model_configs-label"
        value={values.model_configs}
        name="model_configs"
        onBlur={handleBlur}
        onChange={handleChange}
        aria-describedby="helper-text-channel-model_configs-label"
        minRows={8}
        placeholder='统一的模型配置包括定价和属性。JSON格式，键为模型名称，值包含ratio、completion_ratio和max_tokens字段，例如：{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 2.0, "max_tokens": 65536}}'
        sx={{
          '& .MuiInputBase-input': {
            fontFamily: 'JetBrains Mono, Consolas, Monaco, "Courier New", monospace',
            fontSize: '13px',
            lineHeight: '1.4',
            backgroundColor: '#f8f9fa',
          },
          '& .MuiOutlinedInput-root': {
            '& fieldset': {
              borderColor: isValidJSON(values.model_configs) ? theme.palette.grey[300] : theme.palette.error.main,
            },
          }
        }}
      />
      {touched.model_configs && errors.model_configs ? (
        <FormHelperText error id="helper-tex-channel-model_configs-label">
          {errors.model_configs}
        </FormHelperText>
      ) : (
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <FormHelperText id="helper-tex-channel-model_configs-label">
            JSON 格式：统一的模型配置包括定价和属性。键为模型名称，值包含ratio、completion_ratio和max_tokens字段。
          </FormHelperText>
          {values.model_configs && values.model_configs.trim() !== '' && (
            <Typography
              variant="caption"
              sx={{
                color: isValidJSON(values.model_configs) ? theme.palette.success.main : theme.palette.error.main,
                fontWeight: 'bold',
                fontSize: '11px'
              }}
            >
              {isValidJSON(values.model_configs) ? '✓ 有效JSON' : '✗ 无效JSON'}
            </Typography>
          )}
        </Box>
      )}
    </FormControl>

    {/* AWS-specific inference profile ARN mapping */}
    {values.type === 33 && (
      <FormControl fullWidth error={Boolean(touched.inference_profile_arn_map && errors.inference_profile_arn_map)} sx={{ ...theme.typography.otherInput }}>
        <InputLabel
          htmlFor="channel-inference_profile_arn_map-label"
          sx={{
            position: 'relative',
            transform: 'none',
            fontSize: '0.875rem',
            fontWeight: 500,
            color: theme.palette.text.primary
          }}
        >
          {inputLabel.inference_profile_arn_map}
        </InputLabel>
        <TextField
          multiline
          id="channel-inference_profile_arn_map-label"
          value={values.inference_profile_arn_map}
          name="inference_profile_arn_map"
          onBlur={handleBlur}
          onChange={handleChange}
          aria-describedby="helper-text-channel-inference_profile_arn_map-label"
          minRows={5}
          placeholder={inputPrompt.inference_profile_arn_map}
        />
        {touched.inference_profile_arn_map && errors.inference_profile_arn_map ? (
          <FormHelperText error id="helper-tex-channel-inference_profile_arn_map-label">
            {errors.inference_profile_arn_map}
          </FormHelperText>
        ) : (
          <FormHelperText id="helper-tex-channel-inference_profile_arn_map-label">
            JSON 格式：{`{"模型名称": "arn:aws:bedrock:region:account:inference-profile/profile-id"}`}。将模型名称映射到 AWS Bedrock 推理配置文件 ARN。留空则使用默认模型 ID。
          </FormHelperText>
        )}
      </FormControl>
    )}
  </>
);

export default ChannelAdvancedFields;

ChannelAdvancedFields.propTypes = {
  values: PropTypes.object,
  touched: PropTypes.object,
  errors: PropTypes.object,
  handleBlur: PropTypes.func,
  handleChange: PropTypes.func,
  setFieldValue: PropTypes.func,
  inputLabel: PropTypes.object,
  inputPrompt: PropTypes.object,
  defaultPricing: PropTypes.object,
  formatJSON: PropTypes.func,
  isValidJSON: PropTypes.func,
  theme: PropTypes.object
};
