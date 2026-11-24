import React from 'react';
import { Button, TextArea, Typography } from '@douyinfe/semi-ui';
import LabelWithTooltip from './LabelWithTooltip';
import { MODEL_CONFIGS_EXAMPLE, MODEL_MAPPING_EXAMPLE } from './editChannelConfig';

/**
 * ChannelAdvancedFields renders the model mapping and model configs JSON editors, the system
 * prompt, and their template links. It owns no state: every edit goes through EditChannel.
 *
 * @param {object} props The component props.
 * @param {object} props.inputs The channel form state owned by EditChannel.
 * @param {{model_configs: string}} props.defaultPricing The default model configs loaded for the channel type.
 * @param {Function} props.handleInputChange EditChannel's handler that sets one form field by name.
 * @param {Function} props.formatJSON EditChannel's helper that pretty-prints JSON text, returning invalid text unchanged.
 * @param {Function} props.isValidJSON EditChannel's helper that reports whether text is empty or valid JSON.
 * @returns {JSX.Element} The advanced model settings fields.
 */
const ChannelAdvancedFields = ({ inputs, defaultPricing, handleInputChange, formatJSON, isValidJSON }) => (
    <>
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
    </>
);

export default ChannelAdvancedFields;
