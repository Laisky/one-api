// editChannelConfig holds the Air channel editor's static examples and pure validators so the
// EditChannel component file only owns form state and rendering.

// MODEL_MAPPING_EXAMPLE is the sample model_mapping JSON shown in the placeholder and filled by the template link.
export const MODEL_MAPPING_EXAMPLE = {
    'gpt-3.5-turbo-0301': 'gpt-3.5-turbo',
    'gpt-4-0314': 'gpt-4',
    'gpt-4-32k-0314': 'gpt-4-32k'
};

// MODEL_CONFIGS_EXAMPLE is the sample model_configs JSON shown in the placeholder and filled by the template link.
export const MODEL_CONFIGS_EXAMPLE = {
    'gpt-3.5-turbo-0301': {
        'ratio': 0.0015,
        'completion_ratio': 2.0,
        'max_tokens': 65536,
    },
    'gpt-4': {
        'ratio': 0.03,
        'completion_ratio': 2.0,
        'max_tokens': 128000,
    },
    'deepseek-v4-flash': {
        'ratio': 0.00000014,
        'completion_ratio': 2.0,
        'cached_input_ratio': 0.0000000028,
    }
};

/**
 * isClockHHMM reports whether the supplied value is a 24-hour HH:MM clock string.
 *
 * @param {unknown} value The candidate time-window boundary.
 * @returns {boolean} True when the value is a string such as "09:30" or "23:59".
 */
export const isClockHHMM = (value) => typeof value === 'string' && /^([01]\d|2[0-3]):[0-5]\d$/.test(value);

// MODEL_CONFIG_RECOGNIZED_KEYS lists the model_configs entry keys this editor accepts: the JSON keys
// of the backend model.ModelConfigLocal (model/channel_pricing_config.go) plus this editor's
// tool_whitelist/tool_pricing. Every field is optional server-side, so an entry such as
// {"image":{"price_per_image_usd":0.2}}, or the {} the legacy image-price migration writes, is valid.
export const MODEL_CONFIG_RECOGNIZED_KEYS = new Set([
    'per_call', 'per_page', 'ratio', 'completion_ratio', 'cached_input_ratio', 'cache_write_5m_ratio',
    'cache_write_1h_ratio', 'tiers', 'max_tokens', 'context_length', 'video', 'audio', 'image', 'embedding',
    'time_windows', 'tool_whitelist', 'tool_pricing',
]);

/**
 * validateModelConfigs checks the model_configs JSON the operator typed before it is submitted.
 *
 * @param {string} configStr The raw model_configs text; empty or blank text is valid.
 * @returns {{valid: boolean, error?: string}} valid is true when the text is acceptable; otherwise error describes the first problem.
 */
export const validateModelConfigs = (configStr) => {
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

            if (config.time_windows !== undefined) {
                if (!Array.isArray(config.time_windows)) {
                    return { valid: false, error: `Model "${modelName}" time_windows must be an array` };
                }
                for (const [index, window] of config.time_windows.entries()) {
                    if (typeof window !== 'object' || window === null || Array.isArray(window)) {
                        return { valid: false, error: `Model "${modelName}" time window ${index + 1} must be an object` };
                    }
                    if (!Array.isArray(window.ranges) || window.ranges.length === 0) {
                        return { valid: false, error: `Model "${modelName}" time window ${index + 1} ranges must be a non-empty array` };
                    }
                    for (const range of window.ranges) {
                        if (typeof range !== 'object' || range === null || !isClockHHMM(range.start) || !isClockHHMM(range.end)) {
                            return { valid: false, error: `Model "${modelName}" time window ${index + 1} ranges must use HH:MM strings` };
                        }
                    }
                    if (typeof window.overlay !== 'object' || window.overlay === null || Array.isArray(window.overlay)) {
                        return { valid: false, error: `Model "${modelName}" time window ${index + 1} overlay must be an object` };
                    }
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

            // Accept an empty entry or one with at least one recognized key; reject only a non-empty
            // entry whose keys are all unrecognized, which is almost always a typo the backend ignores.
            const entryKeys = Object.keys(config);
            if (entryKeys.length > 0 && !entryKeys.some((key) => MODEL_CONFIG_RECOGNIZED_KEYS.has(key))) {
                return { valid: false, error: `Model "${modelName}" has no recognized configuration keys (unknown: ${entryKeys.join(', ')})` };
            }
        }

        return { valid: true };
    } catch (error) {
        return { valid: false, error: `JSON格式无效：${error.message}` };
    }
};

/**
 * type2secretPrompt returns the key input placeholder for the supplied channel type.
 *
 * @param {number} type The selected channel type.
 * @returns {string} The prompt describing the key format that channel type expects.
 */
export function type2secretPrompt(type) {
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
