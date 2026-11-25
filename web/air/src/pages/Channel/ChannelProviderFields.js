import React from 'react';
import { Banner, Input, Select, Typography } from '@douyinfe/semi-ui';

/**
 * ChannelProviderFields renders the provider connection fields shown right after the channel type
 * selector: the Azure OpenAI endpoint and default API version (type 3) and the OpenAI-compatible
 * base URL and upstream API format (type 50). It owns no state.
 *
 * @param {object} props The component props.
 * @param {object} props.inputs The channel form state owned by EditChannel.
 * @param {object} props.config The channel config state owned by EditChannel.
 * @param {Function} props.handleInputChange EditChannel's handler that sets one form field by name.
 * @param {Function} props.handleConfigChange EditChannel's handler that sets one config field by name.
 * @returns {JSX.Element} The fields for the selected channel type, or an empty fragment for other types.
 */
const ChannelProviderFields = ({ inputs, config, handleInputChange, handleConfigChange }) => (
    <>
        {
            inputs.type === 3 && (
                <>
                    <div style={{ marginTop: 10 }}>
                        <Banner type={"warning"} description={
                            <>
                                注意，<strong>模型部署名称必须和模型名称保持一致</strong>，因为 One API 会把请求体中的
                                model
                                参数替换为你的部署名称（模型名称中的点会被剔除），<a target='_blank'
                                    href='https://github.com/Laisky/one-api/issues/133?notification_referrer_id=NT_kwDOAmJSYrM2NjIwMzI3NDgyOjM5OTk4MDUw#issuecomment-1571602271'>图片演示</a>。
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
    </>
);

export default ChannelProviderFields;
