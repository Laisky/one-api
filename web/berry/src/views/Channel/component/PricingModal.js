import { showError as reportUIError } from '../../../utils/common';
import React, { useState, useEffect } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Typography,
  Divider,
  Alert,
  Grid,
  Paper,
  IconButton,
  Tooltip,
  Autocomplete,
  Chip
} from '@mui/material';
import { Add as AddIcon, Delete as DeleteIcon, Refresh as RefreshIcon, AutoAwesome as AutoAwesomeIcon } from '@mui/icons-material';
import { API } from 'utils/api';
import { showError, showSuccess } from 'utils/common';

const PricingModal = ({ open, onClose, channelId, channelName, channelType }) => {
  const [modelRatio, setModelRatio] = useState({});
  const [completionRatio, setCompletionRatio] = useState({});
  const [loading, setLoading] = useState(false);
  const [newModelName, setNewModelName] = useState('');
  const [newModelPrice, setNewModelPrice] = useState('');
  const [newCompletionName, setNewCompletionName] = useState('');
  const [newCompletionPrice, setNewCompletionPrice] = useState('');
  const [supportedModels, setSupportedModels] = useState([]);

  useEffect(() => {
    if (open && channelId) {
      loadPricing().catch(reportUIError);
      loadSupportedModels().catch(reportUIError);
    }
  }, [open, channelId]);

  const loadSupportedModels = async () => {
    try {
      const response = await API.get('/api/models');
      if (response.data.success) {
        const channelModels = response.data.data[channelType] || [];
        setSupportedModels(channelModels);
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

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>
        <Box display="flex" justifyContent="space-between" alignItems="center">
          <Typography variant="h6">
            Channel Pricing - {channelName}
          </Typography>
          <Tooltip title="Refresh">
            <IconButton onClick={(...uiArgs) => loadPricing(...uiArgs).catch(reportUIError)} disabled={loading}>
              <RefreshIcon />
            </IconButton>
          </Tooltip>
        </Box>
      </DialogTitle>
      <Divider />
      <DialogContent>
        <Alert severity="info" sx={{ mb: 2 }}>
          Configure channel-specific pricing here. If a model is not listed, the global pricing will be used as fallback.
          Leave empty to use global pricing for all models.
        </Alert>

        {/* Model Ratio Section */}
        <Paper elevation={1} sx={{ p: 2, mb: 2 }}>
          <Typography variant="h6" gutterBottom>
            Model Pricing (Price per 1M tokens)
          </Typography>
          <Typography variant="body2" color="text.secondary" gutterBottom>
            Set custom pricing for specific models on this channel (displayed as price per 1M tokens)
          </Typography>

          <Grid container spacing={2} sx={{ mb: 2 }}>
            <Grid item xs={5}>
              <Autocomplete
                freeSolo
                options={supportedModels}
                value={newModelName}
                onChange={(event, newValue) => setNewModelName(newValue || '')}
                onInputChange={(event, newInputValue) => setNewModelName(newInputValue)}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label="Model Name"
                    placeholder="e.g., gpt-4"
                    size="small"
                  />
                )}
                renderOption={(props, option) => (
                  <Box component="li" {...props}>
                    <Chip label={option} size="small" variant="outlined" />
                  </Box>
                )}
              />
            </Grid>
            <Grid item xs={5}>
              <TextField
                fullWidth
                label="Price per 1M tokens"
                type="number"
                value={newModelPrice}
                onChange={(e) => setNewModelPrice(e.target.value)}
                placeholder="e.g., 30.00"
                size="small"
              />
            </Grid>
            <Grid item xs={2}>
              <Button
                fullWidth
                variant="contained"
                onClick={addModelRatio}
                disabled={!newModelName || !newModelPrice}
                startIcon={<AddIcon />}
                size="small"
              >
                Add
              </Button>
            </Grid>
          </Grid>

          {Object.entries(modelRatio).map(([modelName, ratio]) => (
            <Grid container spacing={2} key={modelName} sx={{ mb: 1 }}>
              <Grid item xs={5}>
                <TextField
                  fullWidth
                  value={modelName}
                  disabled
                  size="small"
                />
              </Grid>
              <Grid item xs={5}>
                <TextField
                  fullWidth
                  type="number"
                  value={formatPricePerMillion(ratio)}
                  onChange={(e) => updateModelRatio(modelName, e.target.value)}
                  size="small"
                />
              </Grid>
              <Grid item xs={2}>
                <IconButton
                  color="error"
                  onClick={() => removeModelRatio(modelName)}
                  size="small"
                >
                  <DeleteIcon />
                </IconButton>
              </Grid>
            </Grid>
          ))}
        </Paper>

        {/* Completion Ratio Section */}
        <Paper elevation={1} sx={{ p: 2 }}>
          <Typography variant="h6" gutterBottom>
            Completion Pricing (Price per 1M tokens)
          </Typography>
          <Typography variant="body2" color="text.secondary" gutterBottom>
            Set custom completion token pricing for specific models (displayed as price per 1M tokens)
          </Typography>

          <Grid container spacing={2} sx={{ mb: 2 }}>
            <Grid item xs={5}>
              <Autocomplete
                freeSolo
                options={supportedModels}
                value={newCompletionName}
                onChange={(event, newValue) => setNewCompletionName(newValue || '')}
                onInputChange={(event, newInputValue) => setNewCompletionName(newInputValue)}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label="Model Name"
                    placeholder="e.g., gpt-4"
                    size="small"
                  />
                )}
                renderOption={(props, option) => (
                  <Box component="li" {...props}>
                    <Chip label={option} size="small" variant="outlined" />
                  </Box>
                )}
              />
            </Grid>
            <Grid item xs={5}>
              <TextField
                fullWidth
                label="Price per 1M tokens"
                type="number"
                value={newCompletionPrice}
                onChange={(e) => setNewCompletionPrice(e.target.value)}
                placeholder="e.g., 3.00"
                size="small"
              />
            </Grid>
            <Grid item xs={2}>
              <Button
                fullWidth
                variant="contained"
                onClick={addCompletionRatio}
                disabled={!newCompletionName || !newCompletionPrice}
                startIcon={<AddIcon />}
                size="small"
              >
                Add
              </Button>
            </Grid>
          </Grid>

          {Object.entries(completionRatio).map(([modelName, ratio]) => (
            <Grid container spacing={2} key={modelName} sx={{ mb: 1 }}>
              <Grid item xs={5}>
                <TextField
                  fullWidth
                  value={modelName}
                  disabled
                  size="small"
                />
              </Grid>
              <Grid item xs={5}>
                <TextField
                  fullWidth
                  type="number"
                  value={formatPricePerMillion(ratio)}
                  onChange={(e) => updateCompletionRatio(modelName, e.target.value)}
                  size="small"
                />
              </Grid>
              <Grid item xs={2}>
                <IconButton
                  color="error"
                  onClick={() => removeCompletionRatio(modelName)}
                  size="small"
                >
                  <DeleteIcon />
                </IconButton>
              </Grid>
            </Grid>
          ))}
        </Paper>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={loading}>
          Cancel
        </Button>
        <Button onClick={(...uiArgs) => savePricing(...uiArgs).catch(reportUIError)} variant="contained" disabled={loading}>
          Save Pricing
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default PricingModal;
