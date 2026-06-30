import PropTypes from 'prop-types';
import { useState, useEffect } from 'react';
import { CHANNEL_OPTIONS } from 'constants/ChannelConstants';
import { useTheme } from '@mui/material/styles';
import { API } from 'utils/api';
import { showError, showSuccess, getChannelModels } from 'utils/common';
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
  Switch,
  Checkbox,
  Box,
  Typography,
  Tooltip,
  IconButton
} from '@mui/material';
import { HelpOutline } from '@mui/icons-material';

import { Formik } from 'formik';
import * as Yup from 'yup';
import { defaultConfig, typeConfig } from '../type/Config'; //typeConfig
import { createFilterOptions } from '@mui/material/Autocomplete';
import CheckBoxOutlineBlankIcon from '@mui/icons-material/CheckBoxOutlineBlank';
import CheckBoxIcon from '@mui/icons-material/CheckBox';

const icon = <CheckBoxOutlineBlankIcon fontSize="small" />;
const checkedIcon = <CheckBoxIcon fontSize="small" />;

const filter = createFilterOptions();
const isClockHHMM = (value) => typeof value === 'string' && /^([01]\d|2[0-3]):[0-5]\d$/.test(value);

const validationSchema = Yup.object().shape({
  is_edit: Yup.boolean(),
  name: Yup.string().required('名称 不能为空'),
  type: Yup.number().required('渠道 不能为空'),
  key: Yup.string().when(['is_edit', 'type'], {
    is: (is_edit, type) => !is_edit && type !== 33,
    then: Yup.string().required('密钥 不能为空')
  }),
  other: Yup.string(),
  models: Yup.array().min(1, '模型 不能为空'),
  groups: Yup.array().min(1, '用户组 不能为空'),
  base_url: Yup.string().when('type', {
    is: (value) => [3, 50].includes(value),
    then: Yup.string().required('渠道API地址 不能为空'), // base_url 是必需的
    otherwise: Yup.string() // 在其他情况下，base_url 可以是任意字符串
  }),
  model_mapping: Yup.string().test('is-json', '必须是有效的JSON字符串', function (value) {
    try {
      if (value === '' || value === null || value === undefined) {
        return true;
      }
      const parsedValue = JSON.parse(value);
      if (typeof parsedValue === 'object') {
        return true;
      }
    } catch (e) {
      return false;
    }
    return false;
  }),
  // model_ratio and completion_ratio validation removed - now handled through model_configs
  model_configs: Yup.string().nullable().test('is-valid-model-configs', '模型配置格式无效', function (value) {
    if (value === '' || value === null || value === undefined) {
      return true;
    }

    try {
      const configs = JSON.parse(value);

      if (typeof configs !== 'object' || configs === null || Array.isArray(configs)) {
        return this.createError({ message: '模型配置必须是JSON对象' });
      }

      for (const [modelName, config] of Object.entries(configs)) {
        if (!modelName || modelName.trim() === '') {
          return this.createError({ message: '模型名称不能为空' });
        }

        if (typeof config !== 'object' || config === null || Array.isArray(config)) {
          return this.createError({ message: `模型"${modelName}"的配置必须是对象` });
        }

        // Validate ratio
        if (config.ratio !== undefined) {
          if (typeof config.ratio !== 'number' || config.ratio < 0) {
            return this.createError({ message: `模型"${modelName}"的ratio无效：必须是非负数` });
          }
        }

        // Validate completion_ratio
        if (config.completion_ratio !== undefined) {
          if (typeof config.completion_ratio !== 'number' || config.completion_ratio < 0) {
            return this.createError({ message: `模型"${modelName}"的completion_ratio无效：必须是非负数` });
          }
        }

        // Validate max_tokens
        if (config.max_tokens !== undefined) {
          if (!Number.isInteger(config.max_tokens) || config.max_tokens < 0) {
            return this.createError({ message: `模型"${modelName}"的max_tokens无效：必须是非负整数` });
          }
        }

        if (config.time_windows !== undefined) {
          if (!Array.isArray(config.time_windows)) {
            return this.createError({ message: `Model "${modelName}" time_windows must be an array` });
          }
          for (const [index, window] of config.time_windows.entries()) {
            if (typeof window !== 'object' || window === null || Array.isArray(window)) {
              return this.createError({ message: `Model "${modelName}" time window ${index + 1} must be an object` });
            }
            if (!Array.isArray(window.ranges) || window.ranges.length === 0) {
              return this.createError({ message: `Model "${modelName}" time window ${index + 1} ranges must be a non-empty array` });
            }
            for (const range of window.ranges) {
              if (typeof range !== 'object' || range === null || !isClockHHMM(range.start) || !isClockHHMM(range.end)) {
                return this.createError({ message: `Model "${modelName}" time window ${index + 1} ranges must use HH:MM strings` });
              }
            }
            if (typeof window.overlay !== 'object' || window.overlay === null || Array.isArray(window.overlay)) {
              return this.createError({ message: `Model "${modelName}" time window ${index + 1} overlay must be an object` });
            }
          }
        }

        // Check if at least one meaningful field is provided
        if (
          config.ratio === undefined &&
          config.completion_ratio === undefined &&
          config.max_tokens === undefined &&
          (!Array.isArray(config.time_windows) || config.time_windows.length === 0)
        ) {
          return this.createError({ message: `模型"${modelName}"必须至少有一个配置字段（ratio、completion_ratio或max_tokens）` });
        }
      }

      return true;
    } catch (error) {
      return this.createError({ message: `JSON格式无效：${error.message}` });
    }
  }),
  inference_profile_arn_map: Yup.string().nullable().test('is-json', '必须是有效的JSON字符串', function (value) {
    try {
      if (value === '' || value === null || value === undefined) {
        return true;
      }
      const parsedValue = JSON.parse(value);
      if (typeof parsedValue === 'object') {
        return true;
      }
    } catch (e) {
      return false;
    }
    return false;
  })
});

// Helper component for labels with tooltips
const LabelWithTooltip = ({ label, helpText, children, ...props }) => (
  <Box sx={{ display: 'flex', alignItems: 'center', ...props.sx }}>
    <InputLabel {...props}>
      {label}
    </InputLabel>
    {helpText && (
      <Tooltip title={helpText} placement="top" arrow>
        <IconButton size="small" sx={{ ml: 0.5, p: 0.25 }}>
          <HelpOutline sx={{ fontSize: 16, color: 'text.secondary' }} />
        </IconButton>
      </Tooltip>
    )}
    {children}
  </Box>
);

const EditModal = ({ open, channelId, onCancel, onOk }) => {
  const theme = useTheme();
  const [loading, setLoading] = useState(false);
  const [initialInput, setInitialInput] = useState(defaultConfig.input);
  const [inputLabel, setInputLabel] = useState(defaultConfig.inputLabel); //
  const [inputPrompt, setInputPrompt] = useState(defaultConfig.prompt);
  const [groupOptions, setGroupOptions] = useState([]);
  const [modelOptions, setModelOptions] = useState([]);
  const [batchAdd, setBatchAdd] = useState(false);
  const [basicModels, setBasicModels] = useState([]);
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
    let localModels = getChannelModels(typeValue);
    setBasicModels(localModels);

    setFieldValue('models', []);
    setFieldValue('model_configs', '');
    setFieldValue('model_mapping', '');
    setFieldValue('system_prompt', '');
    setFieldValue('inference_profile_arn_map', '');
    setFieldValue('base_url', '');
    setFieldValue('other', '');

    setFieldValue('config', { api_format: 'chat_completion' });
    // Load default pricing for the new channel type
    loadDefaultPricing(typeValue);
  };

  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      setGroupOptions(res.data.data);
    } catch (error) {
      showError(error.message);
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
      showError(error.message);
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
    setSubmitting(true);
    if (values.base_url && values.base_url.endsWith('/')) {
      values.base_url = values.base_url.slice(0, values.base_url.length - 1);
    }
    if (values.type === 3 && values.other === '') {
      values.other = '2023-09-01-preview';
    }
    if (values.type === 18 && values.other === '') {
      values.other = 'v2.1';
    }
    if (values.key === '') {
      if (values.config.ak && values.config.sk && values.config.region) {
        values.key = `${values.config.ak}|${values.config.sk}|${values.config.region}`;
      } else if (values.config.region && values.config.vertex_ai_project_id && values.config.vertex_ai_adc) {
        values.key = `${values.config.region}|${values.config.vertex_ai_project_id}|${values.config.vertex_ai_adc}`;
      }
    }

    let res;
    const modelsStr = values.models.map((model) => model.id).join(',');
    const configStr = JSON.stringify(values.config);
    values.group = values.groups.join(',');

    // Handle pricing fields - convert empty strings to null for the API
    if (values.model_ratio === '') {
      values.model_ratio = null;
    }
    if (values.completion_ratio === '') {
      values.completion_ratio = null;
    }
    if (values.model_configs === '') {
      values.model_configs = null;
    }
    if (channelId) {
      res = await API.put(`/api/channel/`, {
        ...values,
        id: parseInt(channelId),
        models: modelsStr,
        config: configStr
      });
    } else {
      res = await API.post(`/api/channel/`, { ...values, models: modelsStr, config: configStr });
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
            console.log('Loaded model_configs for channel:', data.id, 'type:', data.type, 'models:', Object.keys(parsedConfigs));
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
        loadDefaultPricing(data.type);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchGroups().then();
    fetchModels().then();
  }, []);

  useEffect(() => {
    setBatchAdd(false);
    if (channelId) {
      loadChannel().then();
    } else {
      initChannel(1);
      setInitialInput({ ...defaultConfig.input, is_edit: false });
      // Load default pricing for new channels
      loadDefaultPricing(1);
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
          <Formik initialValues={initialInput} enableReinitialize validationSchema={validationSchema} onSubmit={submit}>
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
                        setFieldValue('models', initialModel(basicModels));
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
                    {channelId === 0 && (
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
  channelId: PropTypes.number,
  onCancel: PropTypes.func,
  onOk: PropTypes.func
};
