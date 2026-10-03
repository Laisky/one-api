import { showError as reportUIError } from '../../helpers/utils';
import React, { useEffect, useState } from 'react';
import { Card, Input, Table, Typography, Spin, Empty, Collapsible, Tag, Select, Row, Col, Button } from '@douyinfe/semi-ui';
import { API, showError } from '../../helpers';

const { Title, Text } = Typography;

const Models = () => {
  const [modelsData, setModelsData] = useState({});
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedChannels, setSelectedChannels] = useState([]);
  const [filteredData, setFilteredData] = useState({});

  const fetchModelsData = async () => {
    try {
      setLoading(true);
      const res = await API.get('/api/models/display');
      const { success, message, data } = res.data;
      if (success) {
        setModelsData(data || {});
        setFilteredData(data || {});
      } else {
        showError(message);
      }
    } catch (error) {
      if (error.response && error.response.status === 401) {
        showError('Failed to fetch models data: Please login to view available models');
      } else {
        showError('Failed to fetch models data');
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchModelsData().catch(reportUIError);
  }, []);

  useEffect(() => {
    let filtered = { ...modelsData };

    // Filter by selected channels
    if (selectedChannels.length > 0) {
      const channelFiltered = {};
      selectedChannels.forEach(channelName => {
        if (filtered[channelName]) {
          channelFiltered[channelName] = filtered[channelName];
        }
      });
      filtered = channelFiltered;
    }

    // Filter by search term
    if (searchTerm) {
      const searchFiltered = {};
      Object.keys(filtered).forEach(channelName => {
        const channelData = filtered[channelName];
        const filteredModels = {};

        Object.keys(channelData.models).forEach(modelName => {
          if (modelName.toLowerCase().includes(searchTerm.toLowerCase())) {
            filteredModels[modelName] = channelData.models[modelName];
          }
        });

        if (Object.keys(filteredModels).length > 0) {
          searchFiltered[channelName] = {
            ...channelData,
            models: filteredModels
          };
        }
      });
      filtered = searchFiltered;
    }

    setFilteredData(filtered);
  }, [searchTerm, selectedChannels, modelsData]);

  const formatPrice = (price) => {
    if (price === 0) return 'Free';
    if (price < 0.001) return `$${price.toFixed(6)}`;
    if (price < 1) return `$${price.toFixed(4)}`;
    return `$${price.toFixed(2)}`;
  };

  const formatMaxTokens = (maxTokens) => {
    if (maxTokens === 0) return 'Unlimited';
    if (maxTokens >= 1000000) return `${(maxTokens / 1000000).toFixed(1)}M`;
    if (maxTokens >= 1000) return `${(maxTokens / 1000).toFixed(0)}K`;
    return maxTokens.toString();
  };

  const renderChannelModels = (channelName, channelInfo) => {
    const models = Object.keys(channelInfo.models).map(modelName => ({
      key: modelName,
      model: modelName,
      inputPrice: channelInfo.models[modelName].input_price,
      outputPrice: channelInfo.models[modelName].output_price,
      maxTokens: channelInfo.models[modelName].max_tokens
    }));

    const columns = [
      {
        title: 'Model',
        dataIndex: 'model',
        key: 'model',
        width: '40%',
        render: (text) => (
          <Text code style={{ fontSize: '12px' }}>
            {text}
          </Text>
        )
      },
      {
        title: 'Input Price (per 1M tokens)',
        dataIndex: 'inputPrice',
        key: 'inputPrice',
        width: '25%',
        render: (price) => (
          <Text type={price === 0 ? 'success' : 'primary'}>
            {formatPrice(price)}
          </Text>
        )
      },
      {
        title: 'Output Price (per 1M tokens)',
        dataIndex: 'outputPrice',
        key: 'outputPrice',
        width: '25%',
        render: (price) => (
          <Text type={price === 0 ? 'success' : 'primary'}>
            {formatPrice(price)}
          </Text>
        )
      },
      {
        title: 'Max Tokens',
        dataIndex: 'maxTokens',
        key: 'maxTokens',
        width: '10%',
        render: (maxTokens) => (
          <Text>{formatMaxTokens(maxTokens)}</Text>
        )
      }
    ];

    const tableData = models.map(model => ({
      key: model.key,
      model: model.model,
      inputPrice: model.inputPrice,
      outputPrice: model.outputPrice,
      maxTokens: model.maxTokens
    }));

    return (
      <Card
        key={channelName}
        style={{ marginBottom: '16px' }}
        bodyStyle={{ padding: 0 }}
      >
        <Collapsible>
          <Collapsible.Panel
            header={
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <Title heading={6} style={{ margin: 0 }}>
                  {channelName}
                </Title>
                <Tag color="blue" size="small">
                  {models.length} models
                </Tag>
              </div>
            }
            itemKey={channelName}
          >
            <Table
              columns={columns}
              dataSource={tableData}
              pagination={false}
              size="small"
              bordered
            />
          </Collapsible.Panel>
        </Collapsible>
      </Card>
    );
  };

  if (loading) {
    return (
      <Card
        title="Supported Models"
        style={{ margin: '16px' }}
      >
        <div style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: '200px'
        }}>
          <Spin size="large" />
          <Text style={{ marginLeft: '16px' }}>Loading models...</Text>
        </div>
      </Card>
    );
  }

  const totalModels = Object.values(filteredData).reduce((total, channelInfo) =>
    total + Object.keys(channelInfo.models).length, 0
  );

  return (
    <Card
      title="Supported Models"
      style={{ margin: '16px' }}
    >
      <Text type="secondary" style={{ display: 'block', marginBottom: '16px' }}>
        Browse all models supported by the server, grouped by channel/adaptor with pricing information.
      </Text>

      <Row gutter={16} style={{ marginBottom: '16px' }}>
        <Col span={8}>
          <Input
            placeholder="Search models..."
            value={searchTerm}
            onChange={setSearchTerm}
            prefix={<span>🔍</span>}
          />
        </Col>
        <Col span={12}>
          <Select
            placeholder="Filter by channels..."
            multiple
            value={selectedChannels}
            onChange={setSelectedChannels}
            style={{ width: '100%' }}
            optionList={Object.keys(modelsData).sort().map(channelName => ({
              value: channelName,
              label: `${channelName} (${Object.keys(modelsData[channelName].models).length} models)`
            }))}
          />
        </Col>
        <Col span={4}>
          <Button
            onClick={() => {
              setSearchTerm('');
              setSelectedChannels([]);
            }}
            style={{ width: '100%' }}
          >
            Clear Filters
          </Button>
        </Col>
      </Row>

      {totalModels === 0 ? (
        <Empty
          title="No models found"
          description="Try adjusting your search terms."
          style={{ padding: '40px 0' }}
        />
      ) : (
        <>
          <Title heading={6} style={{ marginBottom: '16px' }}>
            Found {totalModels} models in {Object.keys(filteredData).length} channels
          </Title>
          {Object.keys(filteredData)
            .sort()
            .map(channelName => renderChannelModels(channelName, filteredData[channelName]))}
        </>
      )}
    </Card>
  );
};

export default Models;
