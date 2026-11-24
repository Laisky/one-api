import { showError as reportUIError } from '../../helpers/utils';
import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { API, isMobile, showError, showInfo, showSuccess, verifyJSON } from '../../helpers';
import { CHANNEL_OPTIONS } from '../../constants';
import Title from "@douyinfe/semi-ui/lib/es/typography/title";
import { SideSheet, Space, Spin, Button, Input, Typography, Select, TextArea, Checkbox } from "@douyinfe/semi-ui";
import ChannelDebugPanel from '../../components/ChannelDebugPanel';
import { omitUnownedChannelFields } from '../../../../legacy/channel-update-payload.mjs';
import { resolveOtherSave, snapshotOther, storedOtherOverride } from '../../helpers/channel-other';
import { type2secretPrompt, validateModelConfigs } from './editChannelConfig';
import LabelWithTooltip from './LabelWithTooltip';
import ChannelProviderFields from './ChannelProviderFields';
import ChannelAdvancedFields from './ChannelAdvancedFields';

/**
 * EditChannel renders the Air side sheet that creates a channel or edits a stored one.
 *
 * @param {object} props The component props: editingChannel (the channel being edited, empty for a new one), visible, handleClose and refresh.
 * @returns {JSX.Element} The channel editor side sheet.
 */
const EditChannel = (props) => {
    const navigate = useNavigate();
    const channelId = props.editingChannel.uuid || props.editingChannel.id;
    const isEdit = channelId !== undefined;
    const [loading, setLoading] = useState(isEdit);
    const handleCancel = () => {
        props.handleClose()
    };
    const originInputs = {
        name: '',
        type: 1,
        key: '',
        openai_organization: '',
        base_url: '',
        other: '',
        model_mapping: '',
        system_prompt: '',
        models: [],
        auto_ban: 1,
        groups: ['default'],
        model_ratio: '',
        completion_ratio: '',
        model_configs: '',
        ratelimit: 0,
        inference_profile_arn_map: ''
    };
    const [batch, setBatch] = useState(false);
    const [autoBan, setAutoBan] = useState(true);
    // const [autoBan, setAutoBan] = useState(true);
    const [inputs, setInputs] = useState(originInputs);
    const [originModelOptions, setOriginModelOptions] = useState([]);
    const [modelOptions, setModelOptions] = useState([]);
    const [groupOptions, setGroupOptions] = useState([]);
    const [basicModels, setBasicModels] = useState([]);
    const [fullModels, setFullModels] = useState([]);
    const [customModel, setCustomModel] = useState('');
    const defaultConfig = {
        region: '',
        sk: '',
        ak: '',
        user_id: '',
        vertex_ai_project_id: '',
        vertex_ai_adc: '',
        auth_type: 'personal_access_token',
        api_format: 'chat_completion',
    };
    const [config, setConfig] = useState(defaultConfig);
    // otherSnapshot remembers the stored `other` value and typed override of the loaded channel.
    const otherSnapshot = useRef(null);
    const [defaultPricing, setDefaultPricing] = useState({
        model_configs: '',
    });

    const loadDefaultPricing = async (channelType) => {
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

    const handleInputChange = (name, value) => {
        setInputs((prev) => {
            const next = { ...prev, [name]: value };
            if (name === 'type') {
                next.base_url = '';
                next.other = '';
                next.model_mapping = '';
                next.system_prompt = '';
                next.models = [];
                next.model_configs = '';
                next.inference_profile_arn_map = '';
            }
            return next;
        });
        if (name === 'type') {
            // The new type starts from the default config, so no stored override remains.
            otherSnapshot.current = null;
            // Load default pricing for the new channel type
            loadDefaultPricing(value).catch(reportUIError);
            setConfig({ ...defaultConfig });
        }
        //setAutoBan
    };


    const loadChannel = async () => {
      try {
          setLoading(true)
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
                  data.model_mapping = JSON.stringify(JSON.parse(data.model_mapping), null, 2);
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
                      console.log('Loaded model_configs for channel:', channelId, 'type:', data.type, 'models:', Object.keys(parsedConfigs));
                  } catch (e) {
                      console.error('Failed to parse model_configs:', e);
                      // If parsing fails, keep original value but log the error
                  }
              }
              if (data.inference_profile_arn_map && data.inference_profile_arn_map !== '') {
                  try {
                      data.inference_profile_arn_map = JSON.stringify(JSON.parse(data.inference_profile_arn_map), null, 2);
                  } catch (e) {
                      console.error('Failed to parse inference_profile_arn_map:', e);
                  }
              }
              let loadedConfig = { ...defaultConfig };
              if (data.config && data.config !== '') {
                  try {
                      const parsedConfig = JSON.parse(data.config);
                      loadedConfig = {
                          ...defaultConfig,
                          ...parsedConfig,
                          api_format: parsedConfig.api_format || 'chat_completion',
                      };
                  } catch (error) {
                      console.error('Failed to parse channel config:', error);
                  }
              }
              // The field shows the value the relay uses: a stored typed override wins over `other`. The raw
              // stored text keeps keys that JSON.parse collapses, so the override is read as the relay reads it.
              const { displayed, snapshot } = snapshotOther(data.type, data.other, typeof data.config === 'string' ? data.config : loadedConfig);
              otherSnapshot.current = snapshot;
              data.other = displayed;
              setInputs(data);
              setConfig(loadedConfig);
              // Load default pricing for this channel type, but don't override existing model_configs
              loadDefaultPricing(data.type).catch(reportUIError);
              if (data.auto_ban === 0) {
                  setAutoBan(false);
              } else {
                  setAutoBan(true);
              }
              // console.log(data);
          } else {
              showError(message);
          }
      } finally {
        setLoading(false);
      }
    };

    const fetchModels = async () => {
        try {
            let res = await API.get(`/api/channel/models`);
            let localModelOptions = res.data.data.map((model) => ({
                label: model.id,
                value: model.id
            }));
            setOriginModelOptions(localModelOptions);
            setFullModels(res.data.data.map((model) => model.id));
            setBasicModels(res.data.data.filter((model) => {
                return model.id.startsWith('gpt-3') || model.id.startsWith('text-');
            }).map((model) => model.id));
        } catch (error) {
            showError(error);
        }
    };

    const handleConfigChange = (name, value) => {
        setConfig((prev) => ({
            ...prev,
            [name]: value,
        }));
    };

    const fetchGroups = async () => {
        try {
            let res = await API.get(`/api/group/`);
            setGroupOptions(res.data.data.map((group) => ({
                label: group,
                value: group
            })));
        } catch (error) {
            showError(error);
        }
    };

    useEffect(() => {
        let localModelOptions = [...originModelOptions];
        inputs.models.forEach((model) => {
            if (!localModelOptions.find((option) => option.key === model)) {
                localModelOptions.push({
                    label: model,
                    value: model
                });
            }
        });
        setModelOptions(localModelOptions);
    }, [originModelOptions, inputs.models]);

    useEffect(() => {
        fetchModels().then().catch(reportUIError);
        fetchGroups().then().catch(reportUIError);
        if (isEdit) {
            loadChannel().then().catch(reportUIError);
        } else {
            otherSnapshot.current = null;
            setInputs(originInputs);
            setConfig({ ...defaultConfig });
            // Load default pricing for new channels
            loadDefaultPricing(originInputs.type).catch(reportUIError);
        }
    }, [props.editingChannel.uuid, props.editingChannel.id]);


    const submit = async () => {
        if (!isEdit && (inputs.name === '' || inputs.key === '')) {
            showInfo('请填写渠道名称和渠道密钥！');
            return;
        }
        if (inputs.models.length === 0) {
            showInfo('请至少选择一个模型！');
            return;
        }
        if (inputs.model_mapping !== '' && !verifyJSON(inputs.model_mapping)) {
            showInfo('模型映射必须是合法的 JSON 格式！');
            return;
        }
        if (inputs.model_configs !== '') {
            const validation = validateModelConfigs(inputs.model_configs);
            if (!validation.valid) {
                showInfo(`模型配置无效：${validation.error}`);
                return;
            }
        }
        if (inputs.inference_profile_arn_map !== '' && !verifyJSON(inputs.inference_profile_arn_map)) {
            showInfo('推理配置文件ARN映射必须是合法的 JSON 格式！');
            return;
        }
        let localInputs = { ...inputs };
        if (localInputs.key === '') {
            if (config.ak && config.sk && config.region) {
                localInputs.key = `${config.ak}|${config.sk}|${config.region}`;
            } else if (config.region && config.vertex_ai_project_id && config.vertex_ai_adc) {
                localInputs.key = `${config.region}|${config.vertex_ai_project_id}|${config.vertex_ai_adc}`;
            }
        }
        if (localInputs.base_url && localInputs.base_url.endsWith('/')) {
            localInputs.base_url = localInputs.base_url.slice(0, localInputs.base_url.length - 1);
        }
        // An untouched field resends the stored values; an edit also replaces a stored override.
        const otherSave = resolveOtherSave(otherSnapshot.current, localInputs.type, localInputs.other, config);
        localInputs.other = otherSave.other;
        // The legacy defaults apply only when no typed override supplies the value.
        const overridden = storedOtherOverride(localInputs.type, otherSave.config) !== '';
        if (!overridden && localInputs.type === 3 && localInputs.other === '') {
            localInputs.other = '2024-03-01-preview';
        }
        if (!overridden && localInputs.type === 18 && localInputs.other === '') {
            localInputs.other = 'v2.1';
        }
        let res;
        if (!Array.isArray(localInputs.models)) {
            showError('提交失败，请勿重复提交！');
            handleCancel();
            return;
        }
        localInputs.auto_ban = autoBan ? 1 : 0;
        localInputs.models = localInputs.models.join(',');
        localInputs.group = localInputs.groups.join(',');

        // Handle pricing fields - convert empty strings to null for the API
        if (localInputs.model_ratio === '') {
            localInputs.model_ratio = null;
        }
        if (localInputs.completion_ratio === '') {
            localInputs.completion_ratio = null;
        }
        localInputs.config = JSON.stringify(otherSave.config);
        if (isEdit) {
            res = await API.put(`/api/channel/`, omitUnownedChannelFields({ ...localInputs, uuid: channelId }));
        } else {
            res = await API.post(`/api/channel/`, localInputs);
        }
        const { success, message } = res.data;
        if (success) {
            if (isEdit) {
                showSuccess('渠道更新成功！');
            } else {
                showSuccess('渠道创建成功！');
                setInputs(originInputs);
            }
            props.refresh();
            props.handleClose();
        } else {
            showError(message);
        }
    };

    const addCustomModel = () => {
        if (customModel.trim() === '') return;
        if (inputs.models.includes(customModel)) return showError("该模型已存在！");
        let localModels = [...inputs.models];
        localModels.push(customModel);
        let localModelOptions = [];
        localModelOptions.push({
            key: customModel,
            text: customModel,
            value: customModel
        });
        setModelOptions(modelOptions => {
            return [...modelOptions, ...localModelOptions];
        });
        setCustomModel('');
        handleInputChange('models', localModels);
    };

    return (
        <>
            <SideSheet
                maskClosable={false}
                placement={isEdit ? 'right' : 'left'}
                title={
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', width: '100%' }}>
                        <Title level={3}>{isEdit ? '更新渠道信息' : '创建新的渠道'}</Title>
                        {isEdit && (
                            <ChannelDebugPanel
                                channelId={channelId}
                                channelType={inputs.type}
                                channelName={inputs.name}
                            />
                        )}
                    </div>
                }
                headerStyle={{ borderBottom: '1px solid var(--semi-color-border)' }}
                bodyStyle={{ borderBottom: '1px solid var(--semi-color-border)' }}
                visible={props.visible}
                footer={
                    <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                        <Space>
                            <Button theme='solid' size={'large'} onClick={(...uiArgs) => submit(...uiArgs).catch(reportUIError)}>提交</Button>
                            <Button theme='solid' size={'large'} type={'tertiary'} onClick={handleCancel}>取消</Button>
                        </Space>
                    </div>
                }
                closeIcon={null}
                onCancel={() => handleCancel()}
                width={isMobile() ? '100%' : 600}
            >
                {loading ? (
                    <div style={{
                        minHeight: '400px',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        backgroundColor: 'var(--semi-color-bg-2)',
                        borderRadius: '8px',
                        margin: '1rem 0'
                    }}>
                        <div style={{
                            display: 'flex',
                            flexDirection: 'column',
                            alignItems: 'center',
                            gap: '1rem',
                            color: 'var(--semi-color-text-1)'
                        }}>
                            <Spin size="large" />
                            <span>正在加载渠道信息...</span>
                        </div>
                    </div>
                ) : (
                    <>
                        <div style={{ marginTop: 10 }}>
                            <Typography.Text strong>类型：</Typography.Text>
                        </div>
                        <Select
                            name='type'
                            required
                            optionList={CHANNEL_OPTIONS}
                            value={inputs.type}
                            onChange={value => handleInputChange('type', value)}
                            style={{ width: '50%' }}
                        />
                        <ChannelProviderFields
                            inputs={inputs}
                            config={config}
                            handleInputChange={handleInputChange}
                            handleConfigChange={handleConfigChange}
                        />
                        <div style={{ marginTop: 10 }}>
                            <Typography.Text strong>名称：</Typography.Text>
                        </div>
                        <Input
                            required
                            name='name'
                            placeholder={'请为渠道命名'}
                            onChange={value => {
                                handleInputChange('name', value)
                            }}
                            value={inputs.name}
                            autoComplete='new-password'
                        />
                        <div style={{ marginTop: 10 }}>
                            <Typography.Text strong>分组：</Typography.Text>
                        </div>
                        <Select
                            placeholder={'请选择可以使用该渠道的分组'}
                            name='groups'
                            required
                            multiple
                            selection
                            allowAdditions
                            additionLabel={'请在系统设置页面编辑分组倍率以添加新的分组：'}
                            onChange={value => {
                                handleInputChange('groups', value)
                            }}
                            value={inputs.groups}
                            autoComplete='new-password'
                            optionList={groupOptions}
                        />
                        {
                            inputs.type === 18 && (
                                <>
                                    <div style={{ marginTop: 10 }}>
                                        <Typography.Text strong>模型版本：</Typography.Text>
                                    </div>
                                    <Input
                                        name='other'
                                        placeholder={'请输入星火大模型版本，注意是接口地址中的版本号，例如：v2.1'}
                                        onChange={value => {
                                            handleInputChange('other', value)
                                        }}
                                        value={inputs.other}
                                        autoComplete='new-password'
                                    />
                                </>
                            )
                        }
                        {
                            inputs.type === 21 && (
                                <>
                                    <div style={{ marginTop: 10 }}>
                                        <Typography.Text strong>知识库 ID：</Typography.Text>
                                    </div>
                                    <Input
                                        label='知识库 ID'
                                        name='other'
                                        placeholder={'请输入知识库 ID，例如：123456'}
                                        onChange={value => {
                                            handleInputChange('other', value)
                                        }}
                                        value={inputs.other}
                                        autoComplete='new-password'
                                    />
                                </>
                            )
                        }
                        <div style={{ marginTop: 10 }}>
                            <Typography.Text strong>模型：</Typography.Text>
                        </div>
                        <Select
                            placeholder={'请选择该渠道所支持的模型'}
                            name='models'
                            required
                            multiple
                            selection
                            onChange={value => {
                                handleInputChange('models', value)
                            }}
                            value={inputs.models}
                            autoComplete='new-password'
                            optionList={modelOptions}
                        />
                        <div style={{ lineHeight: '40px', marginBottom: '12px' }}>
                            <Space>
                                <Button type='primary' onClick={() => {
                                    handleInputChange('models', basicModels);
                                }}>填入基础模型</Button>
                                <Button type='secondary' onClick={() => {
                                    handleInputChange('models', fullModels);
                                }}>填入所有模型</Button>
                                <Button type='warning' onClick={() => {
                                    handleInputChange('models', []);
                                }}>清除所有模型</Button>
                            </Space>
                            <Input
                                addonAfter={
                                    <Button type='primary' onClick={addCustomModel}>填入</Button>
                                }
                                placeholder='输入自定义模型名称'
                                value={customModel}
                                onChange={(value) => {
                                    setCustomModel(value.trim());
                                }}
                            />
                        </div>
                        <ChannelAdvancedFields
                            inputs={inputs}
                            defaultPricing={defaultPricing}
                            handleInputChange={handleInputChange}
                            formatJSON={formatJSON}
                            isValidJSON={isValidJSON}
                        />
                        <div style={{ marginTop: 10 }}>
                            <Typography.Text strong>密钥：</Typography.Text>
                        </div>
                        {
                            batch ?
                                <TextArea
                                    label='密钥'
                                    name='key'
                                    required
                                    placeholder={'请输入密钥，一行一个'}
                                    onChange={value => {
                                        handleInputChange('key', value)
                                    }}
                                    value={inputs.key}
                                    style={{ minHeight: 150, fontFamily: 'JetBrains Mono, Consolas' }}
                                    autoComplete='new-password'
                                />
                                :
                                <Input
                                    label='密钥'
                                    name='key'
                                    required
                                    placeholder={type2secretPrompt(inputs.type)}
                                    onChange={value => {
                                        handleInputChange('key', value)
                                    }}
                                    value={inputs.key}
                                    autoComplete='new-password'
                                />
                        }
                        <div style={{ marginTop: 10 }}>
                            <Typography.Text strong>组织：</Typography.Text>
                        </div>
                        <Input
                            label='组织，可选，不填则为默认组织'
                            name='openai_organization'
                            placeholder='请输入组织org-xxx'
                            onChange={value => {
                                handleInputChange('openai_organization', value)
                            }}
                            value={inputs.openai_organization}
                        />
                        <div style={{ marginTop: 10, display: 'flex' }}>
                            <Space>
                                <Checkbox
                                    name='auto_ban'
                                    checked={autoBan}
                                    onChange={
                                        () => {
                                            setAutoBan(!autoBan);
                                        }
                                    }
                                // onChange={handleInputChange}
                                />
                                <Typography.Text
                                    strong>是否自动禁用（仅当自动禁用开启时有效），关闭后不会自动禁用该渠道：</Typography.Text>
                            </Space>
                        </div>

                        {
                            !isEdit && (
                                <div style={{ marginTop: 10, display: 'flex' }}>
                                    <Space>
                                        <Checkbox
                                            checked={batch}
                                            label='批量创建'
                                            name='batch'
                                            onChange={() => setBatch(!batch)}
                                        />
                                        <Typography.Text strong>批量创建</Typography.Text>
                                    </Space>
                                </div>
                            )
                        }
                        {
                            inputs.type !== 3 && inputs.type !== 8 && inputs.type !== 22 && (
                                <>
                                    <div style={{ marginTop: 10 }}>
                                        <Typography.Text strong>代理：</Typography.Text>
                                    </div>
                                    <Input
                                        label='代理'
                                        name='base_url'
                                        placeholder={'此项可选，用于通过代理站来进行 API 调用'}
                                        onChange={value => {
                                            handleInputChange('base_url', value)
                                        }}
                                        value={inputs.base_url}
                                        autoComplete='new-password'
                                    />
                                </>
                            )
                        }
                        {
                            inputs.type === 22 && (
                                <>
                                    <div style={{ marginTop: 10 }}>
                                        <Typography.Text strong>私有部署地址：</Typography.Text>
                                    </div>
                                    <Input
                                        name='base_url'
                                        placeholder={'请输入私有部署地址，格式为：https://fastgpt.run/api/openapi'}
                                        onChange={value => {
                                            handleInputChange('base_url', value)
                                        }}
                                        value={inputs.base_url}
                                        autoComplete='new-password'
                                    />
                                </>
                            )
                        }

                        {/* Channel-specific pricing fields - now handled through model_configs */}

                        {/* Rate Limit Field */}
                        <div style={{ marginTop: 20 }}>
                            <LabelWithTooltip
                                label="渠道限速"
                                helpText="控制每个令牌在每个渠道3分钟内的最大请求次数。设置为0表示不限制。这有助于防止滥用和管理API使用量。"
                            />
                        </div>
                        <Input
                            name='ratelimit'
                            placeholder='为每个Token 的每个Channel限速 (3分钟), 默认0为不限速'
                            onChange={value => {
                                handleInputChange('ratelimit', parseInt(value) || 0)
                            }}
                            value={inputs.ratelimit}
                            type="number"
                            min={0}
                        />

                        {/* AWS-specific inference profile ARN mapping */}
                        {inputs.type === 33 && (
                            <>
                                <div style={{ marginTop: 20 }}>
                                    <Typography.Text strong>推理配置文件ARN映射：</Typography.Text>
                                </div>
                                <TextArea
                                    placeholder={`可选，AWS Bedrock 推理配置文件 ARN 映射，JSON 格式。\n示例：\n${JSON.stringify({
                                        "claude-3-5-sonnet-20241022": "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3-5-sonnet-20241022-v2:0",
                                        "claude-3-haiku-20240307": "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3-haiku-20240307-v1:0"
                                    }, null, 2)}`}
                                    style={{
                                        minHeight: 150,
                                        fontFamily: 'JetBrains Mono, Consolas',
                                    }}
                                    onChange={(value) => handleInputChange('inference_profile_arn_map', value)}
                                    value={inputs.inference_profile_arn_map}
                                    autoComplete="new-password"
                                />
                                <div style={{ fontSize: '12px', color: '#666', marginTop: '5px' }}>
                                    JSON 格式：{`{"模型名称": "arn:aws:bedrock:region:account:inference-profile/profile-id"}`}。将模型名称映射到 AWS Bedrock 推理配置文件 ARN。留空则使用默认模型 ID。
                                </div>
                            </>
                        )}
                    </>
                )}
            </SideSheet>
        </>
    );
};

export default EditChannel;
