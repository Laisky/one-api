import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { API, isMobile, showError, showInfo, showSuccess, verifyJSON } from '../../helpers';
import { CHANNEL_OPTIONS } from '../../constants';
import Title from "@douyinfe/semi-ui/lib/es/typography/title";
import { SideSheet, Space, Spin, Button, Input, Typography, Select, TextArea, Checkbox, Banner, Tooltip } from "@douyinfe/semi-ui";
import { IconHelpCircle } from "@douyinfe/semi-icons";
import ChannelDebugPanel from '../../components/ChannelDebugPanel';

const MODEL_MAPPING_EXAMPLE = {
    'gpt-3.5-turbo-0301': 'gpt-3.5-turbo',
    'gpt-4-0314': 'gpt-4',
    'gpt-4-32k-0314': 'gpt-4-32k'
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
            return { valid: false, error: '模型配置必须是JSON对象' };
        }

        for (const [modelName, config] of Object.entries(configs)) {
            if (!modelName || modelName.trim() === '') {
                return { valid: false, error: '模型名称不能为空' };
            }

            if (typeof config !== 'object' || config === null || Array.isArray(config)) {
                return { valid: false, error: `模型"${modelName}"的配置必须是对象` };
            }

            // Validate ratio
            if (config.ratio !== undefined) {
                if (typeof config.ratio !== 'number' || config.ratio < 0) {
                    return { valid: false, error: `模型"${modelName}"的ratio无效：必须是非负数` };
                }
            }

            // Validate completion_ratio
            if (config.completion_ratio !== undefined) {
                if (typeof config.completion_ratio !== 'number' || config.completion_ratio < 0) {
                    return { valid: false, error: `模型"${modelName}"的completion_ratio无效：必须是非负数` };
                }
            }

            // Validate max_tokens
            if (config.max_tokens !== undefined) {
                if (!Number.isInteger(config.max_tokens) || config.max_tokens < 0) {
                    return { valid: false, error: `模型"${modelName}"的max_tokens无效：必须是非负整数` };
                }
            }

            if (config.tool_whitelist !== undefined) {
                if (!Array.isArray(config.tool_whitelist)) {
                    return { valid: false, error: `模型"${modelName}"的tool_whitelist必须是字符串数组` };
                }
                for (const entry of config.tool_whitelist) {
                    if (typeof entry !== 'string' || entry.trim() === '') {
                        return { valid: false, error: `模型"${modelName}"的tool_whitelist包含无效条目` };
                    }
                }
            }

            if (config.tool_pricing !== undefined) {
                if (typeof config.tool_pricing !== 'object' || config.tool_pricing === null || Array.isArray(config.tool_pricing)) {
                    return { valid: false, error: `模型"${modelName}"的tool_pricing必须是对象` };
                }
                const entries = Object.entries(config.tool_pricing);
                if (entries.length === 0) {
                    return { valid: false, error: `模型"${modelName}"的tool_pricing不能为空` };
                }
                for (const [toolName, pricing] of entries) {
                    if (!toolName || toolName.trim() === '') {
                        return { valid: false, error: `模型"${modelName}"的tool_pricing存在空的工具名称` };
                    }
                    if (typeof pricing !== 'object' || pricing === null || Array.isArray(pricing)) {
                        return { valid: false, error: `模型"${modelName}"的工具"${toolName}"的pricing必须是对象` };
                    }
                    const { usd_per_call, quota_per_call } = pricing;
                    if (usd_per_call !== undefined && (typeof usd_per_call !== 'number' || usd_per_call < 0)) {
                        return { valid: false, error: `模型"${modelName}"的工具"${toolName}"的usd_per_call必须是非负数` };
                    }
                    if (quota_per_call !== undefined && (typeof quota_per_call !== 'number' || quota_per_call < 0)) {
                        return { valid: false, error: `模型"${modelName}"的工具"${toolName}"的quota_per_call必须是非负数` };
                    }
                    if (usd_per_call === undefined && quota_per_call === undefined) {
                        return { valid: false, error: `模型"${modelName}"的工具"${toolName}"必须设置usd_per_call或quota_per_call` };
                    }
                }
            }

            // Check if at least one meaningful field is provided
            const hasPricingField = config.ratio !== undefined || config.completion_ratio !== undefined || config.max_tokens !== undefined;
            const hasToolField = (Array.isArray(config.tool_whitelist) && config.tool_whitelist.length > 0) ||
                (config.tool_pricing && Object.keys(config.tool_pricing).length > 0);
            if (!hasPricingField && !hasToolField) {
                return { valid: false, error: `模型"${modelName}"必须包含价格配置或工具配置` };
            }
        }

        return { valid: true };
    } catch (error) {
        return { valid: false, error: `JSON格式无效：${error.message}` };
    }
};

function type2secretPrompt(type) {
    // inputs.type === 15 ? '按照如下格式输入：APIKey|SecretKey' : (inputs.type === 18 ? '按照如下格式输入：APPID|APISecret|APIKey' : '请输入渠道对应的鉴权密钥')
    switch (type) {
        case 15:
            return '按照如下格式输入：APIKey|SecretKey';
        case 18:
            return '按照如下格式输入：APPID|APISecret|APIKey';
        case 22:
            return '按照如下格式输入：APIKey-AppId，例如：fastgpt-0sp2gtvfdgyi4k30jwlgwf1i-64f335d84283f05518e9e041';
        case 23:
            return '按照如下格式输入：AppId|SecretId|SecretKey';
        default:
            return '请输入渠道对应的鉴权密钥';
    }
}

// Helper component for labels with tooltips
const LabelWithTooltip = ({ label, helpText, children, ...props }) => (
    <div style={{ display: 'flex', alignItems: 'center', marginBottom: '8px' }}>
        <Typography.Text strong {...props}>
            {label}
        </Typography.Text>
        {helpText && (
            <Tooltip content={helpText} position="top">
                <IconHelpCircle
                    style={{
                        marginLeft: '6px',
                        color: '#999',
                        cursor: 'help',
                        fontSize: '14px'
                    }}
                />
            </Tooltip>
        )}
        {children}
    </div>
);

const EditChannel = (props) => {
    const navigate = useNavigate();
    const channelId = props.editingChannel.id;
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
            // Load default pricing for the new channel type
            loadDefaultPricing(value);
            setConfig({ ...defaultConfig });
        }
        //setAutoBan
    };


    const loadChannel = async () => {
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
                    console.log('Loaded model_configs for channel:', data.id, 'type:', data.type, 'models:', Object.keys(parsedConfigs));
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
            setInputs(data);
            if (data.config && data.config !== '') {
                try {
                    const parsedConfig = JSON.parse(data.config);
                    setConfig({
                        ...defaultConfig,
                        ...parsedConfig,
                        api_format: parsedConfig.api_format || 'chat_completion',
                    });
                } catch (error) {
                    console.error('Failed to parse channel config:', error);
                    setConfig({ ...defaultConfig });
                }
            } else {
                setConfig({ ...defaultConfig });
            }
            // Load default pricing for this channel type, but don't override existing model_configs
            loadDefaultPricing(data.type);
            if (data.auto_ban === 0) {
                setAutoBan(false);
            } else {
                setAutoBan(true);
            }
            // console.log(data);
        } else {
            showError(message);
        }
        setLoading(false);
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
            showError(error.message);
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
            showError(error.message);
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
        fetchModels().then();
        fetchGroups().then();
        if (isEdit) {
            loadChannel().then();
        } else {
            setInputs(originInputs);
            setConfig({ ...defaultConfig });
            // Load default pricing for new channels
            loadDefaultPricing(originInputs.type);
        }
    }, [props.editingChannel.id]);


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
        if (localInputs.type === 3 && localInputs.other === '') {
            localInputs.other = '2024-03-01-preview';
        }
        if (localInputs.type === 18 && localInputs.other === '') {
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
        localInputs.config = JSON.stringify(config);
        if (isEdit) {
            res = await API.put(`/api/channel/`, { ...localInputs, id: parseInt(channelId) });
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
                            <Button theme='solid' size={'large'} onClick={submit}>提交</Button>
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
                        {
                            inputs.type === 3 && (
                                <>
                                    <div style={{ marginTop: 10 }}>
                                        <Banner type={"warning"} description={
                                            <>
                                                注意，<strong>模型部署名称必须和模型名称保持一致</strong>，因为 One API 会把请求体中的
                                                model
                                                参数替换为你的部署名称（模型名称中的点会被剔除），<a target='_blank'
                                                    href='https://github.com/songquanpeng/one-api/issues/133?notification_referrer_id=NT_kwDOAmJSYrM2NjIwMzI3NDgyOjM5OTk4MDUw#issuecomment-1571602271'>图片演示</a>。
                                            </>
                                        }>
                                        </Banner>
                                    </div>
                                    <div style={{ marginTop: 10 }}>
                                        <Typography.Text strong>AZURE_OPENAI_ENDPOINT：</Typography.Text>
                                    </div>
                                    <Input
                                        label='AZURE_OPENAI_ENDPOINT'
                                        name='azure_base_url'
                                        placeholder={'请输入 AZURE_OPENAI_ENDPOINT，例如：https://docs-test-001.openai.azure.com'}
                                        onChange={value => {
                                            handleInputChange('base_url', value)
                                        }}
                                        value={inputs.base_url}
                                        autoComplete='new-password'
                                    />
                                    <div style={{ marginTop: 10 }}>
                                        <Typography.Text strong>默认 API 版本：</Typography.Text>
                                    </div>
                                    <Input
                                        label='默认 API 版本'
                                        name='azure_other'
                                        placeholder={'请输入默认 API 版本，例如：2024-03-01-preview，该配置可以被实际的请求查询参数所覆盖'}
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
                            inputs.type === 50 && (
                                <>
                                    <div style={{ marginTop: 10 }}>
                                        <Typography.Text strong>Base URL：</Typography.Text>
                                    </div>
                                    <Input
                                        name='base_url'
                                        placeholder={'请输入 OpenAI 兼容渠道的 Base URL，例如：https://oneapi.laisky.com/v1'}
                                        onChange={value => {
                                            handleInputChange('base_url', value)
                                        }}
                                        value={inputs.base_url}
                                        autoComplete='new-password'
                                    />
                                    <div style={{ marginTop: 10 }}>
                                        <Typography.Text strong>上游 API 格式：</Typography.Text>
                                    </div>
                                    <Select
                                        placeholder={'请选择上游 API 格式'}
                                        optionList={[
                                            { label: 'ChatCompletion（默认）', value: 'chat_completion' },
                                            { label: 'Response', value: 'response' },
                                        ]}
                                        value={config.api_format}
                                        onChange={value => handleConfigChange('api_format', value)}
                                    />
                                    <Typography.Text type='tertiary' style={{ fontSize: 12 }}>
                                        ChatCompletion 适用于传统的 OpenAI 兼容接口；当上游仅支持 Response API 时请选择 Response。
                                    </Typography.Text>
                                </>
                            )
                        }
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
                        <div style={{ marginTop: 10, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                            <LabelWithTooltip
                                label="模型重定向"
                                helpText="将传入的模型请求重定向到不同的模型。例如，将'gpt-4-0314'映射到'gpt-4'以处理已弃用的模型名称。JSON格式：{&quot;请求模型&quot;: &quot;实际模型&quot;}"
                            />
                            <div>
                                <Button
                                    theme="borderless"
                                    size="small"
                                    onClick={() => {
                                        const formatted = formatJSON(inputs.model_mapping);
                                        handleInputChange('model_mapping', formatted);
                                    }}
                                    disabled={!inputs.model_mapping || inputs.model_mapping.trim() === ''}
                                >
                                    格式化JSON
                                </Button>
                            </div>
                        </div>
                        <TextArea
                            placeholder={`此项可选，用于修改请求体中的模型名称，为一个 JSON 字符串，键为请求中模型名称，值为要替换的模型名称，例如：\n${JSON.stringify(MODEL_MAPPING_EXAMPLE, null, 2)}`}
                            name='model_mapping'
                            onChange={value => {
                                handleInputChange('model_mapping', value)
                            }}
                            autosize
                            value={inputs.model_mapping}
                            autoComplete='new-password'
                            style={{
                                fontFamily: 'JetBrains Mono, Consolas, Monaco, "Courier New", monospace',
                                fontSize: '13px',
                                lineHeight: '1.4',
                                backgroundColor: '#f8f9fa',
                                border: `1px solid ${isValidJSON(inputs.model_mapping) ? 'var(--semi-color-border)' : 'var(--semi-color-danger)'}`,
                            }}
                        />
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: '5px' }}>
                            <Typography.Text style={{
                                color: 'rgba(var(--semi-blue-5), 1)',
                                fontSize: '12px'
                            }}>
                                此项可选，用于修改请求体中的模型名称。
                            </Typography.Text>
                            {inputs.model_mapping && inputs.model_mapping.trim() !== '' && (
                                <Typography.Text style={{
                                    color: isValidJSON(inputs.model_mapping) ? 'var(--semi-color-success)' : 'var(--semi-color-danger)',
                                    fontWeight: 'bold',
                                    fontSize: '11px'
                                }}>
                                    {isValidJSON(inputs.model_mapping) ? '✓ 有效JSON' : '✗ 无效JSON'}
                                </Typography.Text>
                            )}
                        </div>
                        <div style={{ marginTop: 10, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                            <LabelWithTooltip
                                label="模型配置"
                                helpText="为每个模型配置定价和限制。'ratio'设置输入token成本，'completion_ratio'设置输出token成本倍数，'max_tokens'设置请求限制。覆盖默认定价。"
                            />
                            <div>
                                <Button
                                    theme="borderless"
                                    size="small"
                                    onClick={() => {
                                        const formatted = formatJSON(defaultPricing.model_configs);
                                        handleInputChange('model_configs', formatted);
                                    }}
                                    style={{ marginRight: 8 }}
                                >
                                    加载默认值
                                </Button>
                                <Button
                                    theme="borderless"
                                    size="small"
                                    onClick={() => {
                                        const formatted = formatJSON(inputs.model_configs);
                                        handleInputChange('model_configs', formatted);
                                    }}
                                    disabled={!inputs.model_configs || inputs.model_configs.trim() === ''}
                                >
                                    格式化JSON
                                </Button>
                            </div>
                        </div>
                        <TextArea
                            placeholder={`此项可选，统一的模型配置包括定价和属性。JSON格式，键为模型名称，值包含ratio、completion_ratio和max_tokens字段，例如：\n${JSON.stringify(MODEL_CONFIGS_EXAMPLE, null, 2)}`}
                            name='model_configs'
                            onChange={value => {
                                handleInputChange('model_configs', value)
                            }}

                            autosize
                            minRows={8}
                            value={inputs.model_configs}
                            autoComplete='new-password'
                            style={{
                                fontFamily: 'JetBrains Mono, Consolas, Monaco, "Courier New", monospace',
                                fontSize: '13px',
                                lineHeight: '1.4',
                                backgroundColor: '#f8f9fa',
                                border: `1px solid ${isValidJSON(inputs.model_configs) ? 'var(--semi-color-border)' : 'var(--semi-color-danger)'}`,
                            }}
                        />
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: '5px' }}>
                            <Typography.Text style={{
                                color: 'rgba(var(--semi-blue-5), 1)',
                                fontSize: '12px'
                            }}>
                                此项可选，统一的模型配置包括定价和属性。
                            </Typography.Text>
                            {inputs.model_configs && inputs.model_configs.trim() !== '' && (
                                <Typography.Text style={{
                                    color: isValidJSON(inputs.model_configs) ? 'var(--semi-color-success)' : 'var(--semi-color-danger)',
                                    fontWeight: 'bold',
                                    fontSize: '11px'
                                }}>
                                    {isValidJSON(inputs.model_configs) ? '✓ 有效JSON' : '✗ 无效JSON'}
                                </Typography.Text>
                            )}
                        </div>
                        <Typography.Text style={{
                            color: 'rgba(var(--semi-blue-5), 1)',
                            userSelect: 'none',
                            cursor: 'pointer'
                        }} onClick={
                            () => {
                                handleInputChange('model_configs', JSON.stringify(MODEL_CONFIGS_EXAMPLE, null, 2))
                            }
                        }>
                            填入模板
                        </Typography.Text>
                        <div style={{ marginTop: 10 }}>
                            <LabelWithTooltip
                                label="系统提示词"
                                helpText="为通过此渠道的所有请求强制设置特定的系统提示词。适用于创建专门的AI助手或强制执行特定的行为模式。"
                            />
                        </div>
                        <TextArea
                            placeholder={`此项可选，用于强制设置给定的系统提示词，请配合自定义模型 & 模型重定向使用，首先创建一个唯一的自定义模型名称并在上面填入，之后将该自定义模型重定向映射到该渠道一个原生支持的模型`}
                            name='system_prompt'
                            onChange={value => {
                                handleInputChange('system_prompt', value)
                            }}
                            autosize
                            value={inputs.system_prompt}
                            autoComplete='new-password'
                        />
                        <Typography.Text style={{
                            color: 'rgba(var(--semi-blue-5), 1)',
                            userSelect: 'none',
                            cursor: 'pointer'
                        }} onClick={
                            () => {
                                handleInputChange('model_mapping', JSON.stringify(MODEL_MAPPING_EXAMPLE, null, 2))
                            }
                        }>
                            填入模板
                        </Typography.Text>
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
