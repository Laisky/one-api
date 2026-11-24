import { showError as reportUIError } from '../../../utils/common';
import PropTypes from 'prop-types';
import { useState, useEffect, useRef } from 'react';
import { CHANNEL_OPTIONS } from 'constants/ChannelConstants';
import { useTheme } from '@mui/material/styles';
import { API } from 'utils/api';
import { showError, showSuccess, getChannelDefaultModels } from 'utils/common';
import { resolveOtherSave, snapshotOther, storedOtherOverride } from 'utils/channel-other';
import ChannelDebugPanel from '../../../components/ChannelDebugPanel';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Button,
  Divider,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  OutlinedInput,
  ButtonGroup,
  Container,
  Autocomplete,
  FormHelperText,
  Checkbox,
  Box,
  Typography
} from '@mui/material';

import { Formik } from 'formik';
import { defaultConfig, typeConfig } from '../type/Config'; //typeConfig
import { createFilterOptions } from '@mui/material/Autocomplete';
import CheckBoxOutlineBlankIcon from '@mui/icons-material/CheckBoxOutlineBlank';
import CheckBoxIcon from '@mui/icons-material/CheckBox';
import { omitUnownedChannelFields } from '../../../../../legacy/channel-update-payload.mjs';
import { validationSchema } from './editModalConfig';
import LabelWithTooltip from './LabelWithTooltip';
import ChannelCredentialFields from './ChannelCredentialFields';
import ChannelAdvancedFields from './ChannelAdvancedFields';

const icon = <CheckBoxOutlineBlankIcon fontSize="small" />;
const checkedIcon = <CheckBoxIcon fontSize="small" />;

const filter = createFilterOptions();

/**
 * EditModal renders the Berry dialog that creates a channel or edits a stored one.
 *
 * @param {object} props The component props.
 * @param {boolean} props.open Whether the dialog is shown.
 * @param {number|string} [props.channelId] The channel to edit; a new channel is created when it is empty.
 * @param {Function} props.onCancel Closes the dialog without saving.
 * @param {Function} props.onOk Called with true after a successful save.
 * @returns {JSX.Element} The channel editor dialog.
 */
const EditModal = ({ open, channelId, onCancel, onOk }) => {
  const theme = useTheme();
  const [loading, setLoading] = useState(false);
  const [initialInput, setInitialInput] = useState(defaultConfig.input);
  const [inputLabel, setInputLabel] = useState(defaultConfig.inputLabel); //
  const [inputPrompt, setInputPrompt] = useState(defaultConfig.prompt);
  const [groupOptions, setGroupOptions] = useState([]);
  const [modelOptions, setModelOptions] = useState([]);
  const [batchAdd, setBatchAdd] = useState(false);
  // otherSnapshot remembers the stored `other` value and typed override of the loaded channel.
  const otherSnapshot = useRef(null);
  const [defaultPricing, setDefaultPricing] = useState({
    model_ratio: '',
    completion_ratio: '',
    model_configs: '',
  });

  const formatJSON = (jsonString) => {
    if (!jsonString || jsonString.trim() === '') return '';
    try {
      const parsed = JSON.parse(jsonString);
      return JSON.stringify(parsed, null, 2);
    } catch (e) {
      return jsonString; // Return original if parsing fails
    }
  };

  const isValidJSON = (jsonString) => {
    if (!jsonString || jsonString.trim() === '') return true; // Empty is valid
    try {
      JSON.parse(jsonString);
      return true;
    } catch (e) {
      return false;
    }
  };

  const initChannel = (typeValue) => {
    if (typeConfig[typeValue]?.inputLabel) {
      setInputLabel({
        ...defaultConfig.inputLabel,
        ...typeConfig[typeValue].inputLabel
      });
    } else {
      setInputLabel(defaultConfig.inputLabel);
    }

    if (typeConfig[typeValue]?.prompt) {
      setInputPrompt({
        ...defaultConfig.prompt,
        ...typeConfig[typeValue].prompt
      });
    } else {
      setInputPrompt(defaultConfig.prompt);
    }

    return typeConfig[typeValue]?.input;
  };
  const handleTypeChange = (setFieldValue, typeValue, values) => {
    initChannel(typeValue);
    // The new type starts from a fresh config, so no stored override remains.
    otherSnapshot.current = null;

    setFieldValue('models', []);
    setFieldValue('model_configs', '');
    setFieldValue('model_mapping', '');
    setFieldValue('system_prompt', '');
    setFieldValue('inference_profile_arn_map', '');
    setFieldValue('base_url', '');
    setFieldValue('other', '');

    setFieldValue('config', { api_format: 'chat_completion' });
    // Load default pricing for the new channel type
    loadDefaultPricing(typeValue).catch(reportUIError);
  };

  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      setGroupOptions(res.data.data);
    } catch (error) {
      showError(error);
    }
  };

  const fetchModels = async () => {
    try {
      let res = await API.get(`/api/channel/models`);
      const { data } = res.data;
      data.forEach((item) => {
        if (!item.owned_by) {
          item.owned_by = '未知';
        }
      });
      // 先对data排序
      data.sort((a, b) => {
        const ownedByComparison = a.owned_by.localeCompare(b.owned_by);
        if (ownedByComparison === 0) {
          return a.id.localeCompare(b.id);
        }
        return ownedByComparison;
      });

      setModelOptions(
        data.map((model) => {
          return {
            id: model.id,
            group: model.owned_by
          };
        })
      );
    } catch (error) {
      showError(error);
    }
  };

  const loadDefaultPricing = async (channelType) => {
    try {
      const res = await API.get(`/api/channel/default-pricing?type=${channelType}`);
      if (res.data.success) {
        // Format model_configs if it exists
        let formattedModelConfigs = res.data.data.model_configs || '';
        if (formattedModelConfigs && formattedModelConfigs !== '') {
          try {
            const parsed = JSON.parse(formattedModelConfigs);
            formattedModelConfigs = JSON.stringify(parsed, null, 2);
          } catch (e) {
            // If parsing fails, use as-is
          }
        }

        setDefaultPricing({
          model_ratio: res.data.data.model_ratio || '',
          completion_ratio: res.data.data.completion_ratio || '',
          model_configs: formattedModelConfigs,
        });
      }
    } catch (error) {
      console.error('Failed to load default pricing:', error);
    }
  };

  const submit = async (values, { setErrors, setStatus, setSubmitting }) => {
    try {
      setSubmitting(true);
      // The request body is a copy: Formik's values stay as displayed, so a failed save leaves the
      // form, and the untouched-field comparison of the next save, unchanged.
      const body = { ...values };
      if (body.base_url && body.base_url.endsWith('/')) {
        body.base_url = body.base_url.slice(0, body.base_url.length - 1);
      }
      // An untouched field resends the stored values; an edit also replaces a stored override.
      const otherSave = resolveOtherSave(otherSnapshot.current, values.type, values.other, values.config);
      body.other = otherSave.other;
      // The legacy defaults apply only when no typed override supplies the value.
      const overridden = storedOtherOverride(values.type, otherSave.config) !== '';
      if (!overridden && body.type === 3 && body.other === '') {
        body.other = '2023-09-01-preview';
      }
      if (!overridden && body.type === 18 && body.other === '') {
        body.other = 'v2.1';
      }
      if (body.key === '') {
        if (values.config.ak && values.config.sk && values.config.region) {
          body.key = `${values.config.ak}|${values.config.sk}|${values.config.region}`;
        } else if (values.config.region && values.config.vertex_ai_project_id && values.config.vertex_ai_adc) {
          body.key = `${values.config.region}|${values.config.vertex_ai_project_id}|${values.config.vertex_ai_adc}`;
        }
      }

      let res;
      const modelsStr = values.models.map((model) => model.id).join(',');
      const configStr = JSON.stringify(otherSave.config);
      body.group = values.groups.join(',');

      // Handle pricing fields - convert empty strings to null for the API
      if (body.model_ratio === '') {
        body.model_ratio = null;
      }
      if (body.completion_ratio === '') {
        body.completion_ratio = null;
      }
      if (body.model_configs === '') {
        body.model_configs = null;
      }
      if (channelId) {
        res = await API.put(`/api/channel/`, omitUnownedChannelFields({
          ...body,
          uuid: channelId,
          models: modelsStr,
          config: configStr
        }));
      } else {
        res = await API.post(`/api/channel/`, { ...body, models: modelsStr, config: configStr });
      }
      const { success, message } = res.data;
      if (success) {
        if (channelId) {
          showSuccess('渠道更新成功！');
        } else {
          showSuccess('渠道创建成功！');
        }
        setSubmitting(false);
        setStatus({ success: true });
        onOk(true);
      } else {
        setStatus({ success: false });
        showError(message);
        setErrors({ submit: message });
      }
    } finally {
      setSubmitting(false);
    }
  };

  function initialModel(channelModel) {
    if (!channelModel) {
      return [];
    }

    // 如果 channelModel 是一个字符串
    if (typeof channelModel === 'string') {
      channelModel = channelModel.split(',');
    }
    let modelList = channelModel.map((model) => {
      const modelOption = modelOptions.find((option) => option.id === model);
      if (modelOption) {
        return modelOption;
      }
      return { id: model, group: '自定义：点击或回车输入' };
    });
    return modelList;
  }

  const loadChannel = async () => {
    setLoading(true);
    try {
      // Add cache busting parameter to ensure fresh data
      const cacheBuster = Date.now();
      let res = await API.get(`/api/channel/${channelId}?_cb=${cacheBuster}`);
      const { success, message, data } = res.data;
      if (success) {
        if (data.models === '') {
          data.models = [];
        } else {
          data.models = initialModel(data.models);
        }
        if (data.group === '') {
          data.groups = [];
        } else {
          data.groups = data.group.split(',');
        }
        if (data.model_mapping !== '') {
          data.model_mapping = JSON.stringify(JSON.parse(data.model_mapping), null, 2);
        }
        if (data.config !== '') {
          try {
            const parsedConfig = JSON.parse(data.config);
            data.config = {
              ...defaultConfig.input.config,
              ...parsedConfig,
              api_format: parsedConfig.api_format || 'chat_completion'
            };
          } catch (error) {
            console.error('Failed to parse channel config:', error);
            data.config = { ...defaultConfig.input.config };
          }
        } else {
          data.config = { ...defaultConfig.input.config };
        }
        // The field shows the value the relay uses: a stored typed override wins over `other`.
        const { displayed, snapshot } = snapshotOther(data.type, data.other, data.config);
        otherSnapshot.current = snapshot;
        data.other = displayed;
        // Format pricing fields for display
        if (data.model_ratio && data.model_ratio !== '') {
          try {
            data.model_ratio = JSON.stringify(JSON.parse(data.model_ratio), null, 2);
          } catch (e) {
            console.error('Failed to parse model_ratio:', e);
          }
        }
        if (data.completion_ratio && data.completion_ratio !== '') {
          try {
            data.completion_ratio = JSON.stringify(JSON.parse(data.completion_ratio), null, 2);
          } catch (e) {
            console.error('Failed to parse completion_ratio:', e);
          }
        }
        if (data.model_configs && data.model_configs !== '') {
          try {
            const parsedConfigs = JSON.parse(data.model_configs);
            // Pretty format with proper indentation
            data.model_configs = JSON.stringify(parsedConfigs, null, 2);
            console.log('Loaded model_configs for channel:', data.uuid, 'type:', data.type, 'models:', Object.keys(parsedConfigs));
          } catch (e) {
            console.error('Failed to parse model_configs:', e);
            // If parsing fails, keep original value but log the error
          }
        }
        data.base_url = data.base_url ?? '';
        data.is_edit = true;
        initChannel(data.type);
        setInitialInput(data);
        // Load default pricing for this channel type, but don't override existing model_configs
        loadDefaultPricing(data.type).catch(reportUIError);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchGroups().then().catch(reportUIError);
    fetchModels().then().catch(reportUIError);
  }, []);

  useEffect(() => {
    setBatchAdd(false);
    if (channelId) {
      loadChannel().then().catch(reportUIError);
    } else {
      initChannel(1);
      otherSnapshot.current = null;
      setInitialInput({ ...defaultConfig.input, is_edit: false });
      // Load default pricing for new channels
      loadDefaultPricing(1).catch(reportUIError);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId]);

  return (
    <Dialog open={open} onClose={onCancel} fullWidth maxWidth={'md'}>
      <DialogTitle
        sx={{
          margin: '0px',
          fontWeight: 700,
          lineHeight: '1.55556',
          padding: '24px',
          fontSize: '1.125rem',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center'
        }}
      >
        <span>{channelId ? '编辑渠道' : '新建渠道'}</span>
        {channelId && (
          <ChannelDebugPanel
            channelId={channelId}
            channelType={initialInput.type}
            channelName={initialInput.name}
          />
        )}
      </DialogTitle>
      <Divider />
      <DialogContent>
        {loading ? (
          <Box
            sx={{
              minHeight: '400px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              backgroundColor: 'background.paper',
              borderRadius: '8px',
              margin: '1rem 0'
            }}
          >
            <Box
              sx={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                gap: '1rem',
                color: 'text.secondary'
              }}
            >
              <div className="ui active inline loader"></div>
              <Typography>正在加载渠道信息...</Typography>
            </Box>
          </Box>
        ) : (
          <Formik initialValues={initialInput} enableReinitialize validationSchema={validationSchema} onSubmit={(...uiArgs) => submit(...uiArgs).catch(reportUIError)}>
            {({ errors, handleBlur, handleChange, handleSubmit, isSubmitting, touched, values, setFieldValue }) => (
              <form noValidate onSubmit={handleSubmit}>
                <FormControl fullWidth error={Boolean(touched.type && errors.type)} sx={{ ...theme.typography.otherInput }}>
                  <InputLabel htmlFor="channel-type-label">{inputLabel.type}</InputLabel>
                  <Select
                    id="channel-type-label"
                    label={inputLabel.type}
                    value={values.type}
                    name="type"
                    onBlur={handleBlur}
                    onChange={(e) => {
                      handleChange(e);
                      handleTypeChange(setFieldValue, e.target.value, values);
                    }}
                    MenuProps={{
                      PaperProps: {
                        style: {
                          maxHeight: 200
                        }
                      }
                    }}
                  >
                    {Object.values(CHANNEL_OPTIONS)
                      .sort((a, b) => {
                        return a.text.localeCompare(b.text);
                      })
                      .map((option) => {
                        return (
                          <MenuItem key={option.value} value={option.value}>
                            {option.text}
                          </MenuItem>
                        );
                      })}
                  </Select>
                  {touched.type && errors.type ? (
                    <FormHelperText error id="helper-tex-channel-type-label">
                      {errors.type}
                    </FormHelperText>
                  ) : (
                    <FormHelperText id="helper-tex-channel-type-label"> {inputPrompt.type} </FormHelperText>
                  )}
                </FormControl>

                <FormControl fullWidth error={Boolean(touched.name && errors.name)} sx={{ ...theme.typography.otherInput }}>
                  <InputLabel htmlFor="channel-name-label">{inputLabel.name}</InputLabel>
                  <OutlinedInput
                    id="channel-name-label"
                    label={inputLabel.name}
                    type="text"
                    value={values.name}
                    name="name"
                    onBlur={handleBlur}
                    onChange={handleChange}
                    inputProps={{ autoComplete: 'name' }}
                    aria-describedby="helper-text-channel-name-label"
                  />
                  {touched.name && errors.name ? (
                    <FormHelperText error id="helper-tex-channel-name-label">
                      {errors.name}
                    </FormHelperText>
                  ) : (
                    <FormHelperText id="helper-tex-channel-name-label"> {inputPrompt.name} </FormHelperText>
                  )}
                </FormControl>

                <FormControl fullWidth error={Boolean(touched.base_url && errors.base_url)} sx={{ ...theme.typography.otherInput }}>
                  <InputLabel htmlFor="channel-base_url-label">{inputLabel.base_url}</InputLabel>
                  <OutlinedInput
                    id="channel-base_url-label"
                    label={inputLabel.base_url}
                    type="text"
                    value={values.base_url}
                    name="base_url"
                    onBlur={handleBlur}
                    onChange={handleChange}
                    inputProps={{}}
                    aria-describedby="helper-text-channel-base_url-label"
                  />
                  {touched.base_url && errors.base_url ? (
                    <FormHelperText error id="helper-tex-channel-base_url-label">
                      {errors.base_url}
                    </FormHelperText>
                  ) : (
                    <FormHelperText id="helper-tex-channel-base_url-label"> {inputPrompt.base_url} </FormHelperText>
                  )}
                </FormControl>

                {values.type === 50 && (
                  <FormControl fullWidth sx={{ ...theme.typography.otherInput }}>
                    <InputLabel id="channel-api-format-label">上游 API 格式</InputLabel>
                    <Select
                      labelId="channel-api-format-label"
                      id="channel-api-format"
                      label="上游 API 格式"
                      value={values.config?.api_format ?? 'chat_completion'}
                      onChange={(event) => {
                        const nextConfig = {
                          ...values.config,
                          api_format: event.target.value
                        };
                        setFieldValue('config', nextConfig, true);
                      }}
                    >
                      <MenuItem value="chat_completion">ChatCompletion（默认）</MenuItem>
                      <MenuItem value="response">Response</MenuItem>
                    </Select>
                    <FormHelperText>ChatCompletion 适用于传统 OpenAI 兼容渠道；当上游只接受 Response API 时请选择 Response。</FormHelperText>
                  </FormControl>
                )}

                {inputPrompt.other && (
                  <FormControl fullWidth error={Boolean(touched.other && errors.other)} sx={{ ...theme.typography.otherInput }}>
                    <InputLabel htmlFor="channel-other-label">{inputLabel.other}</InputLabel>
                    <OutlinedInput
                      id="channel-other-label"
                      label={inputLabel.other}
                      type="text"
                      value={values.other}
                      name="other"
                      onBlur={handleBlur}
                      onChange={handleChange}
                      inputProps={{}}
                      aria-describedby="helper-text-channel-other-label"
                    />
                    {touched.other && errors.other ? (
                      <FormHelperText error id="helper-tex-channel-other-label">
                        {errors.other}
                      </FormHelperText>
                    ) : (
                      <FormHelperText id="helper-tex-channel-other-label"> {inputPrompt.other} </FormHelperText>
                    )}
                  </FormControl>
                )}

                <FormControl fullWidth sx={{ ...theme.typography.otherInput }}>
                  <Autocomplete
                    multiple
                    id="channel-groups-label"
                    options={groupOptions}
                    value={values.groups}
                    onChange={(e, value) => {
                      const event = {
                        target: {
                          name: 'groups',
                          value: value
                        }
                      };
                      handleChange(event);
                    }}
                    onBlur={handleBlur}
                    filterSelectedOptions
                    renderInput={(params) => <TextField {...params} name="groups" error={Boolean(errors.groups)} label={inputLabel.groups} />}
                    aria-describedby="helper-text-channel-groups-label"
                  />
                  {errors.groups ? (
                    <FormHelperText error id="helper-tex-channel-groups-label">
                      {errors.groups}
                    </FormHelperText>
                  ) : (
                    <FormHelperText id="helper-tex-channel-groups-label"> {inputPrompt.groups} </FormHelperText>
                  )}
                </FormControl>

                <FormControl fullWidth sx={{ ...theme.typography.otherInput }}>
                  <Autocomplete
                    multiple
                    freeSolo
                    id="channel-models-label"
                    options={modelOptions}
                    value={values.models}
                    onChange={(e, value) => {
                      const event = {
                        target: {
                          name: 'models',
                          value: value.map((item) => (typeof item === 'string' ? { id: item, group: '自定义：点击或回车输入' } : item))
                        }
                      };
                      handleChange(event);
                    }}
                    onBlur={handleBlur}
                    // filterSelectedOptions
                    disableCloseOnSelect
                    renderInput={(params) => <TextField {...params} name="models" error={Boolean(errors.models)} label={inputLabel.models} />}
                    groupBy={(option) => option.group}
                    getOptionLabel={(option) => {
                      if (typeof option === 'string') {
                        return option;
                      }
                      if (option.inputValue) {
                        return option.inputValue;
                      }
                      return option.id;
                    }}
                    filterOptions={(options, params) => {
                      const filtered = filter(options, params);
                      const { inputValue } = params;
                      const isExisting = options.some((option) => inputValue === option.id);
                      if (inputValue !== '' && !isExisting) {
                        filtered.push({
                          id: inputValue,
                          group: '自定义：点击或回车输入'
                        });
                      }
                      return filtered;
                    }}
                    renderOption={(props, option, { selected }) => (
                      <li {...props}>
                        <Checkbox icon={icon} checkedIcon={checkedIcon} style={{ marginRight: 8 }} checked={selected} />
                        {option.id}
                      </li>
                    )}
                  />
                  {errors.models ? (
                    <FormHelperText error id="helper-tex-channel-models-label">
                      {errors.models}
                    </FormHelperText>
                  ) : (
                    <FormHelperText id="helper-tex-channel-models-label"> {inputPrompt.models} </FormHelperText>
                  )}
                </FormControl>
                <Container
                  sx={{
                    textAlign: 'right'
                  }}
                >
                  <ButtonGroup variant="outlined" aria-label="small outlined primary button group">
                    <Button
                      onClick={() => {
                        // Related models are the zero-config recommendations for the selected type.
                        setFieldValue('models', initialModel(getChannelDefaultModels(values.type)));
                      }}
                    >
                      填入相关模型
                    </Button>
                    <Button
                      onClick={() => {
                        setFieldValue('models', modelOptions);
                      }}
                    >
                      填入所有模型
                    </Button>
                  </ButtonGroup>
                </Container>
                <ChannelCredentialFields
                  inputLabel={inputLabel}
                  inputPrompt={inputPrompt}
                  values={values}
                  touched={touched}
                  errors={errors}
                  handleBlur={handleBlur}
                  handleChange={handleChange}
                  batchAdd={batchAdd}
                  setBatchAdd={setBatchAdd}
                  channelId={channelId}
                  theme={theme}
                />

                <ChannelAdvancedFields
                  values={values}
                  touched={touched}
                  errors={errors}
                  handleBlur={handleBlur}
                  handleChange={handleChange}
                  setFieldValue={setFieldValue}
                  inputLabel={inputLabel}
                  inputPrompt={inputPrompt}
                  defaultPricing={defaultPricing}
                  formatJSON={formatJSON}
                  isValidJSON={isValidJSON}
                  theme={theme}
                />

                {/* Rate Limit Field */}
                <FormControl fullWidth error={Boolean(touched.ratelimit && errors.ratelimit)} sx={{ ...theme.typography.otherInput }}>
                  <LabelWithTooltip
                    htmlFor="channel-ratelimit-label"
                    label={inputLabel.ratelimit}
                    helpText="控制每个令牌在每个渠道3分钟内的最大请求次数。设置为0表示不限制。这有助于防止滥用和管理API使用量。"
                    sx={{
                      position: 'relative',
                      transform: 'none',
                      fontSize: '0.875rem',
                      fontWeight: 500,
                      color: theme.palette.text.primary,
                      mb: 1
                    }}
                  />
                  <OutlinedInput
                    id="channel-ratelimit-label"
                    type="number"
                    value={values.ratelimit}
                    name="ratelimit"
                    onBlur={handleBlur}
                    onChange={handleChange}
                    placeholder={inputPrompt.ratelimit}
                    inputProps={{ min: 0 }}
                    aria-describedby="helper-text-channel-ratelimit-label"
                  />
                  {touched.ratelimit && errors.ratelimit ? (
                    <FormHelperText error id="helper-text-channel-ratelimit-label">
                      {errors.ratelimit}
                    </FormHelperText>
                  ) : (
                    <FormHelperText id="helper-text-channel-ratelimit-label">
                      {inputPrompt.ratelimit}
                    </FormHelperText>
                  )}
                </FormControl>

                <DialogActions>
                  <Button onClick={onCancel}>取消</Button>
                  <Button disableElevation disabled={isSubmitting} type="submit" variant="contained" color="primary">
                    提交
                  </Button>
                </DialogActions>
              </form>
            )}
          </Formik>
        )}
      </DialogContent>
    </Dialog>
  );
};

export default EditModal;

EditModal.propTypes = {
  open: PropTypes.bool,
  channelId: PropTypes.oneOfType([PropTypes.number, PropTypes.string]),
  onCancel: PropTypes.func,
  onOk: PropTypes.func
};
