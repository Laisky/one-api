import { showError as reportUIError } from '../helpers/utils';
import React, { useState, useEffect } from 'react';
import { Modal, Form, Button, Input, Select, Table, Typography, Space, Divider, Banner } from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../helpers';
import { IconDelete, IconPlus, IconSave, IconClose, IconCoinMoneyStroked } from '@douyinfe/semi-icons';

const { Text } = Typography;

const PricingModal = ({ visible, onClose, channelId, channelName, channelType }) => {
  const [modelRatio, setModelRatio] = useState({});
  const [completionRatio, setCompletionRatio] = useState({});
  const [loading, setLoading] = useState(false);
  const [newModelName, setNewModelName] = useState('');
  const [newModelPrice, setNewModelPrice] = useState('');
  const [newCompletionName, setNewCompletionName] = useState('');
  const [newCompletionPrice, setNewCompletionPrice] = useState('');
  const [supportedModels, setSupportedModels] = useState([]);

  useEffect(() => {
    if (visible && channelId) {
      loadPricing().catch(reportUIError);
      loadSupportedModels().catch(reportUIError);
    }
  }, [visible, channelId]);

  const loadSupportedModels = async () => {
    try {
      const response = await API.get('/api/models');
      if (response.data.success) {
        const channelModels = response.data.data[channelType] || [];
        setSupportedModels(channelModels.map(model => ({ label: model, value: model })));
      }
    } catch (error) {
      console.error('Failed to load supported models:', error);
    }
  };

  const loadPricing = async () => {
    setLoading(true);
    try {
      const response = await API.get(`/api/channel/pricing/${channelId}`);
      if (response.data.success) {
        setModelRatio(response.data.data.model_ratio || {});
        setCompletionRatio(response.data.data.completion_ratio || {});
      } else {
        showError(response.data.message);
      }
    } catch (error) {
      showError('Failed to load pricing data');
    } finally {
      setLoading(false);
    }
  };

  const savePricing = async () => {
    setLoading(true);
    try {
      const response = await API.put(`/api/channel/pricing/${channelId}`, {
        model_ratio: Object.keys(modelRatio).length > 0 ? modelRatio : null,
        completion_ratio: Object.keys(completionRatio).length > 0 ? completionRatio : null
      });
      if (response.data.success) {
        showSuccess('Pricing updated successfully');
        onClose();
      } else {
        showError(response.data.message);
      }
    } catch (error) {
      showError('Failed to save pricing data');
    } finally {
      setLoading(false);
    }
  };

  const addModelRatio = () => {
    if (newModelName && newModelPrice) {
      setModelRatio(prev => ({
        ...prev,
        [newModelName]: convertToRatio(newModelPrice)
      }));
      setNewModelName('');
      setNewModelPrice('');
    }
  };

  const addCompletionRatio = () => {
    if (newCompletionName && newCompletionPrice) {
      setCompletionRatio(prev => ({
        ...prev,
        [newCompletionName]: convertToRatio(newCompletionPrice)
      }));
      setNewCompletionName('');
      setNewCompletionPrice('');
    }
  };

  const removeModelRatio = (modelName) => {
    setModelRatio(prev => {
      const newRatio = { ...prev };
      delete newRatio[modelName];
      return newRatio;
    });
  };

  const removeCompletionRatio = (modelName) => {
    setCompletionRatio(prev => {
      const newRatio = { ...prev };
      delete newRatio[modelName];
      return newRatio;
    });
  };

  const updateModelRatio = (modelName, value) => {
    setModelRatio(prev => ({
      ...prev,
      [modelName]: convertToRatio(value)
    }));
  };

  const updateCompletionRatio = (modelName, value) => {
    setCompletionRatio(prev => ({
      ...prev,
      [modelName]: convertToRatio(value)
    }));
  };

  // Convert ratio to price per 1M tokens for display
  const formatPricePerMillion = (ratio) => {
    if (!ratio || ratio === 0) return '0';
    const pricePerMillion = ratio * 1000000;
    return pricePerMillion.toFixed(2);
  };

  // Convert price per 1M tokens back to ratio
  const convertToRatio = (pricePerMillion) => {
    if (!pricePerMillion || pricePerMillion === 0) return 0;
    return parseFloat(pricePerMillion) / 1000000;
  };

  const modelRatioColumns = [
    {
      title: 'Model Name',
      dataIndex: 'modelName',
      key: 'modelName',
    },
    {
      title: 'Price per 1M tokens',
      dataIndex: 'ratio',
      key: 'ratio',
      render: (ratio, record) => (
        <Input
          type="number"
          step="0.01"
          value={formatPricePerMillion(ratio)}
          onChange={(value) => updateModelRatio(record.modelName, value)}
        />
      ),
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (_, record) => (
        <Button
          theme="borderless"
          type="danger"
          icon={<IconDelete />}
          onClick={() => removeModelRatio(record.modelName)}
        />
      ),
    },
  ];

  const completionRatioColumns = [
    {
      title: 'Model Name',
      dataIndex: 'modelName',
      key: 'modelName',
    },
    {
      title: 'Price per 1M tokens',
      dataIndex: 'ratio',
      key: 'ratio',
      render: (ratio, record) => (
        <Input
          type="number"
          step="0.01"
          value={formatPricePerMillion(ratio)}
          onChange={(value) => updateCompletionRatio(record.modelName, value)}
        />
      ),
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (_, record) => (
        <Button
          theme="borderless"
          type="danger"
          icon={<IconDelete />}
          onClick={() => removeCompletionRatio(record.modelName)}
        />
      ),
    },
  ];

  const modelRatioData = Object.entries(modelRatio).map(([modelName, ratio]) => ({
    key: modelName,
    modelName,
    ratio,
  }));

  const completionRatioData = Object.entries(completionRatio).map(([modelName, ratio]) => ({
    key: modelName,
    modelName,
    ratio,
  }));

  return (
    <Modal
      title={
        <Space>
          <IconCoinMoneyStroked />
          <Text>Channel Pricing - {channelName}</Text>
        </Space>
      }
      visible={visible}
      onCancel={onClose}
      width={800}
      footer={
        <Space>
          <Button onClick={onClose} disabled={loading}>
            <IconClose /> Cancel
          </Button>
          <Button theme="solid" type="primary" onClick={(...uiArgs) => savePricing(...uiArgs).catch(reportUIError)} loading={loading}>
            <IconSave /> Save Pricing
          </Button>
        </Space>
      }
    >
      <Banner
        type="info"
        description="Configure channel-specific pricing here. If a model is not listed, the global pricing will be used as fallback. Leave empty to use global pricing for all models."
      />

      <Divider margin="24px" />

      {/* Model Ratio Section */}
      <Typography.Title heading={5}>Model Pricing (Price per 1M tokens)</Typography.Title>
      <Text type="secondary">Set custom pricing for specific models on this channel (displayed as price per 1M tokens)</Text>

      <Space style={{ marginTop: 16, marginBottom: 16 }}>
        <Select
          placeholder="Select or type model name"
          filter
          allowCreate
          value={newModelName}
          optionList={supportedModels}
          onChange={setNewModelName}
          style={{ width: 200 }}
        />
        <Input
          type="number"
          step="0.01"
          placeholder="e.g., 30.00"
          value={newModelPrice}
          onChange={setNewModelPrice}
          style={{ width: 120 }}
        />
        <Button
          theme="solid"
          type="primary"
          icon={<IconPlus />}
          onClick={addModelRatio}
          disabled={!newModelName || !newModelPrice}
        >
          Add
        </Button>
      </Space>

      {modelRatioData.length > 0 && (
        <Table
          columns={modelRatioColumns}
          dataSource={modelRatioData}
          pagination={false}
          size="small"
        />
      )}

      <Divider margin="24px" />

      {/* Completion Ratio Section */}
      <Typography.Title heading={5}>Completion Pricing (Price per 1M tokens)</Typography.Title>
      <Text type="secondary">Set custom completion token pricing for specific models (displayed as price per 1M tokens)</Text>

      <Space style={{ marginTop: 16, marginBottom: 16 }}>
        <Select
          placeholder="Select or type model name"
          filter
          allowCreate
          value={newCompletionName}
          optionList={supportedModels}
          onChange={setNewCompletionName}
          style={{ width: 200 }}
        />
        <Input
          type="number"
          step="0.01"
          placeholder="e.g., 3.00"
          value={newCompletionPrice}
          onChange={setNewCompletionPrice}
          style={{ width: 120 }}
        />
        <Button
          theme="solid"
          type="primary"
          icon={<IconPlus />}
          onClick={addCompletionRatio}
          disabled={!newCompletionName || !newCompletionPrice}
        >
          Add
        </Button>
      </Space>

      {completionRatioData.length > 0 && (
        <Table
          columns={completionRatioColumns}
          dataSource={completionRatioData}
          pagination={false}
          size="small"
        />
      )}
    </Modal>
  );
};

export default PricingModal;
