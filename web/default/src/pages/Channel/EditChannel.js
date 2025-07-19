import React, {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Button, Card, Form, Input, Message} from 'semantic-ui-react';
import {useNavigate, useParams} from 'react-router-dom';
import {API, copy, getChannelModels, showError, showInfo, showSuccess, verifyJSON,} from '../../helpers';
import {CHANNEL_OPTIONS, COZE_AUTH_OPTIONS} from '../../constants';
import {renderChannelTip} from '../../helpers/render';
import ChannelDebugPanel from '../../components/ChannelDebugPanel';

const MODEL_MAPPING_EXAMPLE = {
  'gpt-3.5-turbo-0301': 'gpt-3.5-turbo',
  'gpt-4-0314': 'gpt-4',
  'gpt-4-32k-0314': 'gpt-4-32k',
};

const MODEL_CONFIGS_EXAMPLE = {
  'gpt-3.5-turbo-0301': {
    'ratio': 0.0015,
    'completion_ratio': 2.0,
    'max_tokens': 65536,
  },
  'gpt-4': {
    'ratio': 0.03,
    'completion_ratio': 2.0,
    'max_tokens': 128000,
  }
};

// Enhanced validation for model configs
const validateModelConfigs = (configStr) => {
  if (!configStr || configStr.trim() === '') {
    return { valid: true };
  }

  try {
    const configs = JSON.parse(configStr);

    if (typeof configs !== 'object' || configs === null || Array.isArray(configs)) {
      return { valid: false, error: 'Model configs must be a JSON object' };
    }

    for (const [modelName, config] of Object.entries(configs)) {
      if (!modelName || modelName.trim() === '') {
        return { valid: false, error: 'Model name cannot be empty' };
      }

      if (typeof config !== 'object' || config === null || Array.isArray(config)) {
        return { valid: false, error: `Configuration for model "${modelName}" must be an object` };
      }

      // Validate ratio
      if (config.ratio !== undefined) {
        if (typeof config.ratio !== 'number' || config.ratio < 0) {
          return { valid: false, error: `Invalid ratio for model "${modelName}": must be a non-negative number` };
        }
      }

      // Validate completion_ratio
      if (config.completion_ratio !== undefined) {
        if (typeof config.completion_ratio !== 'number' || config.completion_ratio < 0) {
          return { valid: false, error: `Invalid completion_ratio for model "${modelName}": must be a non-negative number` };
        }
      }

      // Validate max_tokens
      if (config.max_tokens !== undefined) {
        if (!Number.isInteger(config.max_tokens) || config.max_tokens < 0) {
          return { valid: false, error: `Invalid max_tokens for model "${modelName}": must be a non-negative integer` };
        }
      }

      // Check if at least one meaningful field is provided
      if (config.ratio === undefined && config.completion_ratio === undefined && config.max_tokens === undefined) {
        return { valid: false, error: `Model "${modelName}" must have at least one configuration field (ratio, completion_ratio, or max_tokens)` };
      }
    }

    return { valid: true };
  } catch (error) {
    return { valid: false, error: `Invalid JSON format: ${error.message}` };
  }
};

const OAUTH_JWT_CONFIG_EXAMPLE = {
  "client_type": "jwt",
  "client_id": "123456789",
  "coze_www_base": "https://www.coze.cn",
  "coze_api_base": "https://api.coze.cn",
  "private_key": "-----BEGIN PRIVATE KEY-----\n***\n-----END PRIVATE KEY-----",
  "public_key_id": "***********************************************************"
}

function type2secretPrompt(type, t) {
  switch (type) {
    case 15:
      return t('channel.edit.key_prompts.zhipu');
    case 18:
      return t('channel.edit.key_prompts.spark');
    case 22:
      return t('channel.edit.key_prompts.fastgpt');
    case 23:
      return t('channel.edit.key_prompts.tencent');
    default:
      return t('channel.edit.key_prompts.default');
  }
}

const EditChannel = () => {
  const { t } = useTranslation();
  const params = useParams();
  const navigate = useNavigate();
  const channelId = params.id;
  const isEdit = channelId !== undefined;
  const [loading, setLoading] = useState(isEdit);
  const handleCancel = () => {
    navigate('/channel');
  };

  const originInputs = {
    name: '',
    type: 1,
    key: '',
    base_url: '',
    other: '',
    model_mapping: '',
    system_prompt: '',
    models: [],
    groups: ['default'],
    ratelimit: 0,
    model_ratio: '',
    completion_ratio: '',
    model_configs: '',
    inference_profile_arn_map: '',
  };
  const [batch, setBatch] = useState(false);
  const [inputs, setInputs] = useState(originInputs);
  const [originModelOptions, setOriginModelOptions] = useState([]);
  const [modelOptions, setModelOptions] = useState([]);
  const [groupOptions, setGroupOptions] = useState([]);
  const [basicModels, setBasicModels] = useState([]);
  const [fullModels, setFullModels] = useState([]);
  const [customModel, setCustomModel] = useState('');
  const [config, setConfig] = useState({
    region: '',
    sk: '',
    ak: '',
    user_id: '',
    vertex_ai_project_id: '',
    vertex_ai_adc: '',
    auth_type: 'personal_access_token',
  });
  const [defaultPricing, setDefaultPricing] = useState({
    model_configs: '',
  });

  const loadDefaultPricing = async (channelType, existingModelConfigs = null) => {
    try {
      const res = await API.get(`/api/channel/default-pricing?type=${channelType}`);
      if (res.data.success) {
        // Convert old format to new unified format if needed
        let defaultModelConfigs = '';

        if (res.data.data.model_configs) {
          // Already in new format, but ensure it's properly formatted
          try {
            const parsed = JSON.parse(res.data.data.model_configs);
            defaultModelConfigs = JSON.stringify(parsed, null, 2);
          } catch (e) {
            // If parsing fails, use as-is
            defaultModelConfigs = res.data.data.model_configs;
          }
        } else if (res.data.data.model_ratio || res.data.data.completion_ratio) {
          // Convert from old format to new format
          const modelRatio = res.data.data.model_ratio ? JSON.parse(res.data.data.model_ratio) : {};
          const completionRatio = res.data.data.completion_ratio ? JSON.parse(res.data.data.completion_ratio) : {};

          const unifiedConfigs = {};
          const allModels = new Set([...Object.keys(modelRatio), ...Object.keys(completionRatio)]);

          for (const modelName of allModels) {
            unifiedConfigs[modelName] = {};
            if (modelRatio[modelName]) {
              unifiedConfigs[modelName].ratio = modelRatio[modelName];
            }
            if (completionRatio[modelName]) {
              unifiedConfigs[modelName].completion_ratio = completionRatio[modelName];
            }
          }

          defaultModelConfigs = JSON.stringify(unifiedConfigs, null, 2);
        }



        setDefaultPricing({
          model_configs: defaultModelConfigs,
        });

        // If current model_configs is empty, populate with defaults
        // Don't override if we have existing model_configs from loadChannel
        if (!inputs.model_configs && !existingModelConfigs) {
          setInputs((inputs) => ({
            ...inputs,
            model_configs: defaultModelConfigs,
          }));
        }
      }
    } catch (error) {
      console.error('Failed to load default pricing:', error);
    }
  };

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

  const handleInputChange = (e, { name, value }) => {
    setInputs((inputs) => ({ ...inputs, [name]: value }));
    if (name === 'type') {
      let localModels = getChannelModels(value);
      if (inputs.models.length === 0) {
        setInputs((inputs) => ({ ...inputs, models: localModels }));
      }
      setBasicModels(localModels);
      // Load default pricing for the new channel type
      loadDefaultPricing(value);
    }
  };

  const handleConfigChange = (e, { name, value }) => {
    setConfig((inputs) => ({ ...inputs, [name]: value }));
  };

  const loadChannel = async () => {
    // Add cache busting parameter to ensure fresh data
    const cacheBuster = Date.now();
    let res = await API.get(`/api/channel/${channelId}?_cb=${cacheBuster}`);
    const { success, message, data } = res.data;
    if (success) {
      if (data.models === '') {
        data.models = [];
      } else {
        data.models = data.models.split(',');
      }
      if (data.group === '') {
        data.groups = [];
      } else {
        data.groups = data.group.split(',');
      }
      if (data.model_mapping !== '') {
        data.model_mapping = JSON.stringify(
          JSON.parse(data.model_mapping),
          null,
          2
        );
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
      if (data.inference_profile_arn_map && data.inference_profile_arn_map !== '') {
        try {
          data.inference_profile_arn_map = JSON.stringify(JSON.parse(data.inference_profile_arn_map), null, 2);
        } catch (e) {
          console.error('Failed to parse inference_profile_arn_map:', e);
        }
      }
      setInputs(data);
      if (data.config !== '') {
        setConfig(JSON.parse(data.config));
      }
      setBasicModels(getChannelModels(data.type));
      // Load default pricing for this channel type, but don't override existing model_configs
      loadDefaultPricing(data.type, data.model_configs);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const fetchModels = async () => {
    try {
      let res = await API.get(`/api/channel/models`);
      let localModelOptions = res.data.data.map((model) => ({
        key: model.id,
        text: model.id,
        value: model.id,
      }));
      setOriginModelOptions(localModelOptions);
      setFullModels(res.data.data.map((model) => model.id));
    } catch (error) {
      showError(error.message);
    }
  };

  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      setGroupOptions(
        res.data.data.map((group) => ({
          key: group,
          text: group,
          value: group,
        }))
      );
    } catch (error) {
      showError(error.message);
    }
  };

  useEffect(() => {
    let localModelOptions = [...originModelOptions];
    inputs.models.forEach((model) => {
      if (!localModelOptions.find((option) => option.key === model)) {
        localModelOptions.push({
          key: model,
          text: model,
          value: model,
        });
      }
    });
    setModelOptions(localModelOptions);
  }, [originModelOptions, inputs.models]);

  useEffect(() => {
    if (isEdit) {
      loadChannel().then();
    } else {
      let localModels = getChannelModels(inputs.type);
      setBasicModels(localModels);
      // Load default pricing for new channels
      loadDefaultPricing(inputs.type);
    }
    fetchModels().then();
    fetchGroups().then();
  }, []);

  const submit = async () => {
    if (inputs.key === '') {
      if (config.ak !== '' && config.sk !== '' && config.region !== '') {
        inputs.key = `${config.ak}|${config.sk}|${config.region}`;
      } else if (
        config.region !== '' &&
        config.vertex_ai_project_id !== '' &&
        config.vertex_ai_adc !== ''
      ) {
        inputs.key = `${config.region}|${config.vertex_ai_project_id}|${config.vertex_ai_adc}`;
      }
    }
    if (!isEdit && (inputs.name === '' || inputs.key === '')) {
      showInfo(t('channel.edit.messages.name_required'));
      return;
    }
    if (inputs.type !== 43 && inputs.models.length === 0) {
      showInfo(t('channel.edit.messages.models_required'));
      return;
    }
    if (inputs.model_mapping !== '' && !verifyJSON(inputs.model_mapping)) {
      showInfo(t('channel.edit.messages.model_mapping_invalid'));
      return;
    }
    if (inputs.model_configs !== '') {
      const validation = validateModelConfigs(inputs.model_configs);
      if (!validation.valid) {
        showInfo(`${t('channel.edit.messages.model_configs_invalid')}: ${validation.error}`);
        return;
      }
    }

    // Note: model_ratio and completion_ratio are now handled through model_configs
    if (inputs.inference_profile_arn_map !== '' && !verifyJSON(inputs.inference_profile_arn_map)) {
      showInfo(t('channel.edit.messages.inference_profile_arn_map_invalid'));
      return;
    }

    if (inputs.type === 34 && config.auth_type === 'oauth_config') {
      if (!verifyJSON(inputs.key)) {
        showInfo(t('channel.edit.messages.oauth_config_invalid_format'));
        return;
      }

      try {
        const oauthConfig = JSON.parse(inputs.key);
        const requiredFields = [
          'client_type',
          'client_id',
          'coze_www_base',
          'coze_api_base',
          'private_key',
          'public_key_id'
        ];

        for (const field of requiredFields) {
          if (!oauthConfig.hasOwnProperty(field)) {
            showInfo(t('channel.edit.messages.oauth_config_missing_field', { field }));
            return;
          }
        }
      } catch (error) {
        showInfo(t('channel.edit.messages.oauth_config_parse_error', { error: error.message }));
        return;
      }
    }

    let localInputs = { ...inputs };
    if (localInputs.key === 'undefined|undefined|undefined') {
      localInputs.key = ''; // prevent potential bug
    }
    if (localInputs.base_url && localInputs.base_url.endsWith('/')) {
      localInputs.base_url = localInputs.base_url.slice(
        0,
        localInputs.base_url.length - 1
      );
    }
    if (localInputs.type === 3 && localInputs.other === '') {
      localInputs.other = '2024-03-01-preview';
    }
    let res;
    localInputs.models = localInputs.models.join(',');
    localInputs.group = localInputs.groups.join(',');
    localInputs.ratelimit = parseInt(localInputs.ratelimit);
    localInputs.config = JSON.stringify(config);

    // Handle pricing fields - convert empty strings to null for the API
    if (localInputs.model_ratio === '') {
      localInputs.model_ratio = null;
    }
    if (localInputs.completion_ratio === '') {
      localInputs.completion_ratio = null;
    }
    if (localInputs.inference_profile_arn_map === '') {
      localInputs.inference_profile_arn_map = null;
    }
    if (isEdit) {
      res = await API.put(`/api/channel/`, {
        ...localInputs,
        id: parseInt(channelId),
      });
    } else {
      res = await API.post(`/api/channel/`, localInputs);
    }
    const { success, message } = res.data;
    if (success) {
      if (isEdit) {
        showSuccess(t('channel.edit.messages.update_success'));
      } else {
        showSuccess(t('channel.edit.messages.create_success'));
        setInputs(originInputs);
      }
    } else {
      showError(message);
    }
  };

  const addCustomModel = () => {
    if (customModel.trim() === '') return;
    if (inputs.models.includes(customModel)) return;
    let localModels = [...inputs.models];
    localModels.push(customModel);
    let localModelOptions = [];
    localModelOptions.push({
      key: customModel,
      text: customModel,
      value: customModel,
    });
    setModelOptions((modelOptions) => {
      return [...modelOptions, ...localModelOptions];
    });
    setCustomModel('');
    handleInputChange(null, { name: 'models', value: localModels });
  };

  return (
    <div className='dashboard-container'>
      <Card fluid className='chart-card'>
        <Card.Content>
          <Card.Header className='header' style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span>
              {isEdit
                ? t('channel.edit.title_edit')
                : t('channel.edit.title_create')}
            </span>
            {isEdit && (
              <ChannelDebugPanel
                channelId={channelId}
                channelType={inputs.type}
                channelName={inputs.name}
              />
            )}
          </Card.Header>
          {loading ? (
            <div style={{
              minHeight: '400px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              backgroundColor: 'var(--card-bg)',
              borderRadius: '8px',
              margin: '1rem 0'
            }}>
              <div style={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                gap: '1rem',
                color: 'var(--text-secondary)'
              }}>
                <div className="ui active inline loader"></div>
                <span>{t('channel.edit.loading')}</span>
              </div>
            </div>
          ) : (
            <Form autoComplete='new-password'>
            <Form.Field>
              <Form.Select
                label={t('channel.edit.type')}
                name='type'
                required
                search
                options={CHANNEL_OPTIONS}
                value={inputs.type}
                onChange={handleInputChange}
              />
            </Form.Field>
            {renderChannelTip(inputs.type)}
            <Form.Field>
              <Form.Input
                label={t('channel.edit.name')}
                name='name'
                placeholder={t('channel.edit.name_placeholder')}
                onChange={handleInputChange}
                value={inputs.name}
                required
              />
            </Form.Field>
            <Form.Field>
              <Form.Dropdown
                label={t('channel.edit.group')}
                placeholder={t('channel.edit.group_placeholder')}
                name='groups'
                required
                fluid
                multiple
                selection
                allowAdditions
                additionLabel={t('channel.edit.group_addition')}
                onChange={handleInputChange}
                value={inputs.groups}
                autoComplete='new-password'
                options={groupOptions}
              />
            </Form.Field>

            {/* Azure OpenAI specific fields */}
            {inputs.type === 3 && (
            <>
              <Message>
                Note: <strong>The model deployment name must match the model name</strong>
                , because One API will replace the model parameter in the request body
                with your deployment name (dots in the model name will be removed).
                <a
                  target='_blank'
                  href='https://github.com/songquanpeng/one-api/issues/133?notification_referrer_id=NT_kwDOAmJSYrM2NjIwMzI3NDgyOjM5OTk4MDUw#issuecomment-1571602271'
                >
                  Image Demo
                </a>
              </Message>
              <Form.Field>
                <Form.Input
                  label='AZURE_OPENAI_ENDPOINT'
                  name='base_url'
                  placeholder='Please enter AZURE_OPENAI_ENDPOINT, for example: https://docs-test-001.openai.azure.com'
                  onChange={handleInputChange}
                  value={inputs.base_url}
                  autoComplete='new-password'
                />
              </Form.Field>
              <Form.Field>
                <Form.Input
                  label='Default API Version'
                  name='other'
                  placeholder='Please enter default API version, for example: 2024-03-01-preview. This configuration can be overridden by actual request query parameters'
                  onChange={handleInputChange}
                  value={inputs.other}
                  autoComplete='new-password'
                />
              </Form.Field>
            </>
          )}

            {/* Custom base URL field */}
            {inputs.type === 8 && (
              <Form.Field>
                <Form.Input
                    required
                    label={t('channel.edit.proxy_url')}
                    name='base_url'
                    placeholder={t('channel.edit.proxy_url_placeholder')}
                    onChange={handleInputChange}
                    value={inputs.base_url}
                    autoComplete='new-password'
                />
              </Form.Field>
            )}
            {inputs.type === 50 && (
                <Form.Field>
                  <Form.Input
                      required
                  label={t('channel.edit.base_url')}
                  name='base_url'
                  placeholder={t('channel.edit.base_url_placeholder')}
                  onChange={handleInputChange}
                  value={inputs.base_url}
                  autoComplete='new-password'
                />
              </Form.Field>
            )}

            {inputs.type === 18 && (
              <Form.Field>
                <Form.Input
                  label={t('channel.edit.spark_version')}
                  name='other'
                  placeholder={t('channel.edit.spark_version_placeholder')}
                  onChange={handleInputChange}
                  value={inputs.other}
                  autoComplete='new-password'
                />
              </Form.Field>
            )}
            {inputs.type === 21 && (
              <Form.Field>
                <Form.Input
                  label={t('channel.edit.knowledge_id')}
                  name='other'
                  placeholder={t('channel.edit.knowledge_id_placeholder')}
                  onChange={handleInputChange}
                  value={inputs.other}
                  autoComplete='new-password'
                />
              </Form.Field>
            )}
            {inputs.type === 17 && (
              <Form.Field>
                <Form.Input
                  label={t('channel.edit.plugin_param')}
                  name='other'
                  placeholder={t('channel.edit.plugin_param_placeholder')}
                  onChange={handleInputChange}
                  value={inputs.other}
                  autoComplete='new-password'
                />
              </Form.Field>
            )}
            {inputs.type === 34 && (
              <Message>{t('channel.edit.coze_notice')}</Message>
            )}
            {inputs.type === 40 && (
              <Message>
                {t('channel.edit.douban_notice')}
                <a
                  target='_blank'
                  href='https://console.volcengine.com/ark/region:ark+cn-beijing/endpoint'
                >
                  {t('channel.edit.douban_notice_link')}
                </a>
                {t('channel.edit.douban_notice_2')}
              </Message>
            )}
            {inputs.type !== 43 && (
              <Form.Field>
                <Form.Dropdown
                  label={t('channel.edit.models')}
                  placeholder={t('channel.edit.models_placeholder')}
                  name='models'
                  required
                  fluid
                  multiple
                  search
                  onLabelClick={(e, { value }) => {
                    copy(value).then();
                  }}
                  selection
                  onChange={handleInputChange}
                  value={inputs.models}
                  autoComplete='new-password'
                  options={modelOptions}
                />
              </Form.Field>
            )}
            {inputs.type !== 43 && (
              <div style={{ lineHeight: '40px', marginBottom: '12px' }}>
                <Button
                  type={'button'}
                  onClick={() => {
                    handleInputChange(null, {
                      name: 'models',
                      value: basicModels,
                    });
                  }}
                >
                  {t('channel.edit.buttons.fill_models')}
                </Button>
                <Button
                  type={'button'}
                  onClick={() => {
                    handleInputChange(null, {
                      name: 'models',
                      value: fullModels,
                    });
                  }}
                >
                  {t('channel.edit.buttons.fill_all')}
                </Button>
                <Button
                  type={'button'}
                  onClick={() => {
                    handleInputChange(null, { name: 'models', value: [] });
                  }}
                >
                  {t('channel.edit.buttons.clear')}
                </Button>
                <Input
                  action={
                    <Button type={'button'} onClick={addCustomModel}>
                      {t('channel.edit.buttons.add_custom')}
                    </Button>
                  }
                  placeholder={t('channel.edit.buttons.custom_placeholder')}
                  value={customModel}
                  onChange={(e, { value }) => {
                    setCustomModel(value);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      addCustomModel();
                      e.preventDefault();
                    }
                  }}
                />
              </div>
            )}
            {inputs.type !== 43 && (
              <>
                <Form.Field>
                  <label>
                    {t('channel.edit.model_mapping')}
                    <Button
                      type="button"
                      size="mini"
                      onClick={() => {
                        const formatted = formatJSON(inputs.model_mapping);
                        setInputs((inputs) => ({
                          ...inputs,
                          model_mapping: formatted,
                        }));
                      }}
                      style={{ marginLeft: '10px' }}
                      disabled={!inputs.model_mapping || inputs.model_mapping.trim() === ''}
                    >
                      Format JSON
                    </Button>
                  </label>
                  <Form.TextArea
                    placeholder={`${t(
                      'channel.edit.model_mapping_placeholder'
                    )}\n${JSON.stringify(MODEL_MAPPING_EXAMPLE, null, 2)}`}
                    name='model_mapping'
                    onChange={handleInputChange}
                    value={inputs.model_mapping}
                    style={{
                      minHeight: 150,
                      fontFamily: 'JetBrains Mono, Consolas, Monaco, "Courier New", monospace',
                      fontSize: '13px',
                      lineHeight: '1.4',
                      backgroundColor: '#f8f9fa',
                      border: `1px solid ${isValidJSON(inputs.model_mapping) ? '#e1e5e9' : '#ff6b6b'}`,
                      borderRadius: '4px',
                    }}
                    autoComplete='new-password'
                  />
                  <div style={{ fontSize: '12px', marginTop: '5px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <span style={{ color: '#666' }}>
                      {t('channel.edit.model_mapping_placeholder').split('\n')[0]}
                    </span>
                    {inputs.model_mapping && inputs.model_mapping.trim() !== '' && (
                      <span style={{
                        color: isValidJSON(inputs.model_mapping) ? '#28a745' : '#dc3545',
                        fontWeight: 'bold',
                        fontSize: '11px'
                      }}>
                        {isValidJSON(inputs.model_mapping) ? '✓ Valid JSON' : '✗ Invalid JSON'}
                      </span>
                    )}
                  </div>
                </Form.Field>
                <Form.Field>
                  <label>
                    {t('channel.edit.model_configs')}
                    <Button
                      type="button"
                      size="mini"
                      onClick={() => {
                        const formatted = formatJSON(defaultPricing.model_configs);
                        setInputs((inputs) => ({
                          ...inputs,
                          model_configs: formatted,
                        }));
                      }}
                      style={{ marginLeft: '10px' }}
                    >
                      {t('channel.edit.buttons.load_defaults')}
                    </Button>
                    <Button
                      type="button"
                      size="mini"
                      onClick={() => {
                        const formatted = formatJSON(inputs.model_configs);
                        setInputs((inputs) => ({
                          ...inputs,
                          model_configs: formatted,
                        }));
                      }}
                      style={{ marginLeft: '5px' }}
                      disabled={!inputs.model_configs || inputs.model_configs.trim() === ''}
                    >
                      Format JSON
                    </Button>
                  </label>
                  <Form.TextArea
                    placeholder={`${t(
                      'channel.edit.model_configs_placeholder'
                    )}\n${JSON.stringify(MODEL_CONFIGS_EXAMPLE, null, 2)}`}
                    name='model_configs'
                    onChange={handleInputChange}
                    value={inputs.model_configs}
                    style={{
                      minHeight: 200,
                      fontFamily: 'JetBrains Mono, Consolas, Monaco, "Courier New", monospace',
                      fontSize: '13px',
                      lineHeight: '1.4',
                      backgroundColor: '#f8f9fa',
                      border: `1px solid ${isValidJSON(inputs.model_configs) ? '#e1e5e9' : '#ff6b6b'}`,
                      borderRadius: '4px',
                    }}
                    autoComplete='new-password'
                  />
                  <div style={{ fontSize: '12px', marginTop: '5px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <span style={{ color: 'var(--text-secondary)' }}>
                      {t('channel.edit.model_configs_help')}
                    </span>
                    {inputs.model_configs && inputs.model_configs.trim() !== '' && (
                      <span style={{
                        color: isValidJSON(inputs.model_configs) ? 'var(--success-color)' : 'var(--error-color)',
                        fontWeight: 'bold',
                        fontSize: '11px'
                      }}>
                        {isValidJSON(inputs.model_configs) ? '✓ Valid JSON' : '✗ Invalid JSON'}
                      </span>
                    )}
                  </div>
                </Form.Field>
                <Form.Field>
                  <Form.TextArea
                    label={t('channel.edit.system_prompt')}
                    placeholder={t('channel.edit.system_prompt_placeholder')}
                    name='system_prompt'
                    onChange={handleInputChange}
                    value={inputs.system_prompt}
                    style={{
                      minHeight: 150,
                      fontFamily: 'JetBrains Mono, Consolas',
                    }}
                    autoComplete='new-password'
                  />
                </Form.Field>
              </>
            )}
            {/* Move Coze authentication type selection and input fields here */}
            {inputs.type === 34 && (
              <>
                <Form.Field>
                  <Form.Select
                    label={t('channel.edit.coze_auth_type')}
                    name="auth_type"
                    options={COZE_AUTH_OPTIONS.map(option => ({
                      ...option,
                      text: t(`channel.edit.coze_auth_options.${option.text}`)
                    }))}
                    value={config.auth_type}
                    onChange={(e, { name, value }) => handleConfigChange(e, { name, value })}
                  />
                </Form.Field>
                {config.auth_type === 'personal_access_token' ? (
                  <Form.Field>
                    <Form.Input
                      label={t('channel.edit.key')}
                      name='key'
                      required
                      placeholder={t('channel.edit.key_prompts.default')}
                      onChange={handleInputChange}
                      value={inputs.key}
                      autoComplete='new-password'
                    />
                  </Form.Field>
                ) : (
                  <Form.Field>
                    <Form.TextArea
                      label={t('channel.edit.oauth_jwt_config')}
                      name="key"
                      required
                      placeholder={`${t(
                          'channel.edit.oauth_jwt_config_placeholder'
                      )}\n${JSON.stringify(OAUTH_JWT_CONFIG_EXAMPLE, null, 2)}`}
                      onChange={handleInputChange}
                      value={inputs.key}
                      style={{
                        minHeight: 150,
                        fontFamily: 'JetBrains Mono, Consolas',
                      }}
                      autoComplete='new-password'
                    />
                  </Form.Field>
                )}
              </>
            )}

            {inputs.type === 33 && (
              <Form.Field>
                <Form.Input
                  label='Region'
                  name='region'
                  required
                  placeholder={t('channel.edit.aws_region_placeholder')}
                  onChange={handleConfigChange}
                  value={config.region}
                  autoComplete=''
                />
                <Form.Input
                  label='AK'
                  name='ak'
                  required
                  placeholder={t('channel.edit.aws_ak_placeholder')}
                  onChange={handleConfigChange}
                  value={config.ak}
                  autoComplete=''
                />
                <Form.Input
                  label='SK'
                  name='sk'
                  required
                  placeholder={t('channel.edit.aws_sk_placeholder')}
                  onChange={handleConfigChange}
                  value={config.sk}
                  autoComplete=''
                />
              </Form.Field>
            )}
            {inputs.type === 42 && (
              <Form.Field>
                <Form.Input
                  label='Region'
                  name='region'
                  required
                  placeholder={t('channel.edit.vertex_region_placeholder')}
                  onChange={handleConfigChange}
                  value={config.region}
                  autoComplete=''
                />
                <Form.Input
                  label={t('channel.edit.vertex_project_id')}
                  name='vertex_ai_project_id'
                  required
                  placeholder={t('channel.edit.vertex_project_id_placeholder')}
                  onChange={handleConfigChange}
                  value={config.vertex_ai_project_id}
                  autoComplete=''
                />
                <Form.Input
                  label={t('channel.edit.vertex_credentials')}
                  name='vertex_ai_adc'
                  required
                  placeholder={t('channel.edit.vertex_credentials_placeholder')}
                  onChange={handleConfigChange}
                  value={config.vertex_ai_adc}
                  autoComplete=''
                />
              </Form.Field>
            )}
            {inputs.type === 34 && (
              <Form.Input
                label={t('channel.edit.user_id')}
                name='user_id'
                required
                placeholder={t('channel.edit.user_id_placeholder')}
                onChange={handleConfigChange}
                value={config.user_id}
                autoComplete=''
              />
            )}
            {inputs.type !== 33 &&
              inputs.type !== 42 &&
              inputs.type !== 34 &&
              (batch ? (
                <Form.Field>
                  <Form.TextArea
                    label={t('channel.edit.key')}
                    name='key'
                    required
                    placeholder={t('channel.edit.batch_placeholder')}
                    onChange={handleInputChange}
                    value={inputs.key}
                    style={{
                      minHeight: 150,
                      fontFamily: 'JetBrains Mono, Consolas',
                    }}
                    autoComplete='new-password'
                  />
                </Form.Field>
              ) : (
                <Form.Field>
                  <Form.Input
                    label={t('channel.edit.key')}
                    name='key'
                    required
                    placeholder={type2secretPrompt(inputs.type, t)}
                    onChange={handleInputChange}
                    value={inputs.key}
                    autoComplete='new-password'
                  />
                </Form.Field>
              ))}
            {inputs.type === 37 && (
              <Form.Field>
                <Form.Input
                  label='Account ID'
                  name='user_id'
                  required
                  placeholder={
                    'Please enter Account ID, e.g.: d8d7c61dbc334c32d3ced580e4bf42b4'
                  }
                  onChange={handleConfigChange}
                  value={config.user_id}
                  autoComplete=''
                />
              </Form.Field>
            )}
            {inputs.type !== 33 && !isEdit && (
              <Form.Checkbox
                checked={batch}
                label={t('channel.edit.batch')}
                name='batch'
                onChange={() => setBatch(!batch)}
              />
            )}
            {inputs.type !== 3 &&
              inputs.type !== 33 &&
              inputs.type !== 8 &&
                inputs.type !== 50 &&
              inputs.type !== 22 && (
                <Form.Field>
                  <Form.Input
                      label={t('channel.edit.proxy_url')}
                    name='base_url'
                      placeholder={t('channel.edit.proxy_url_placeholder')}
                    onChange={handleInputChange}
                    value={inputs.base_url}
                    autoComplete='new-password'
                  />
                </Form.Field>
              )}
            {inputs.type === 22 && (
              <Form.Field>
                <Form.Input
                  label='Private Deployment URL'
                  name='base_url'
                  placeholder={
                    'Please enter the private deployment URL, format: https://fastgpt.run/api/openapi'
                  }
                  onChange={handleInputChange}
                  value={inputs.base_url}
                  autoComplete='new-password'
                />
              </Form.Field>
            )}

            <Form.Field>
              <Form.Input
                  label={t('channel.edit.ratelimit')}
                name='ratelimit'
                  placeholder={t('channel.edit.ratelimit_placeholder')}
                onChange={handleInputChange}
                value={inputs.ratelimit}
                autoComplete='new-password'
              />
            </Form.Field>

            {/* Channel-specific pricing fields - now handled through model_configs */}

            {/* AWS-specific inference profile ARN mapping */}
            {inputs.type === 33 && (
              <Form.Field>
                <label>
                  Inference Profile ARN Map
                </label>
                <Form.TextArea
                  name="inference_profile_arn_map"
                  placeholder={`Optional. JSON mapping of model names to AWS Bedrock Inference Profile ARNs.\nExample:\n${JSON.stringify({
                    "claude-3-5-sonnet-20241022": "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3-5-sonnet-20241022-v2:0",
                    "claude-3-haiku-20240307": "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3-haiku-20240307-v1:0"
                  }, null, 2)}`}
                  style={{
                    minHeight: 150,
                    fontFamily: 'JetBrains Mono, Consolas',
                  }}
                  onChange={handleInputChange}
                  value={inputs.inference_profile_arn_map}
                  autoComplete="new-password"
                />
                <div style={{ fontSize: '12px', color: 'var(--text-secondary)', marginTop: '5px' }}>
                  JSON format: {`{"model_name": "arn:aws:bedrock:region:account:inference-profile/profile-id"}`}. Maps model names to AWS Bedrock Inference Profile ARNs. Leave empty to use default model IDs.
                </div>
              </Form.Field>
            )}

            <Button onClick={handleCancel}>
              {t('channel.edit.buttons.cancel')}
            </Button>
            <Button
              type={isEdit ? 'button' : 'submit'}
              positive
              onClick={submit}
            >
              {t('channel.edit.buttons.submit')}
            </Button>
          </Form>
          )}
        </Card.Content>
      </Card>
    </div>
  );
};

export default EditChannel;
