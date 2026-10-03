import { showError as reportUIError } from '../../utils/common';
import React, { useEffect, useState } from 'react';
import {
  Box,
  Container,
  Typography,
  TextField,
  Card,
  CardContent,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Chip,
  Alert,
  CircularProgress,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Grid,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  OutlinedInput,
  Button
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import MainCard from 'ui-component/cards/MainCard';
import { API } from 'utils/api';
import { showError } from 'utils/common';

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
        showError('Please login to view available models');
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
      model: modelName,
      inputPrice: channelInfo.models[modelName].input_price,
      outputPrice: channelInfo.models[modelName].output_price,
      maxTokens: channelInfo.models[modelName].max_tokens
    }));

    return (
      <Accordion key={channelName} sx={{ mb: 2 }}>
        <AccordionSummary expandIcon={<ExpandMoreIcon />}>
          <Typography variant="h6">
            {channelName}
            <Chip
              label={`${models.length} models`}
              size="small"
              sx={{ ml: 2 }}
              color="primary"
            />
          </Typography>
        </AccordionSummary>
        <AccordionDetails>
          <TableContainer component={Paper}>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell><strong>Model</strong></TableCell>
                  <TableCell><strong>Input Price (per 1M tokens)</strong></TableCell>
                  <TableCell><strong>Output Price (per 1M tokens)</strong></TableCell>
                  <TableCell><strong>Max Tokens</strong></TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {models.map(model => (
                  <TableRow key={model.model} hover>
                    <TableCell>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                        {model.model}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color={model.inputPrice === 0 ? 'success.main' : 'text.primary'}>
                        {formatPrice(model.inputPrice)}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color={model.outputPrice === 0 ? 'success.main' : 'text.primary'}>
                        {formatPrice(model.outputPrice)}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2">
                        {formatMaxTokens(model.maxTokens)}
                      </Typography>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </AccordionDetails>
      </Accordion>
    );
  };

  if (loading) {
    return (
      <Box>
        <Container sx={{ paddingTop: '40px' }}>
          <MainCard title="Supported Models">
            <Box display="flex" justifyContent="center" alignItems="center" minHeight="200px">
              <CircularProgress />
              <Typography variant="body1" sx={{ ml: 2 }}>
                Loading models...
              </Typography>
            </Box>
          </MainCard>
        </Container>
      </Box>
    );
  }

  const totalModels = Object.values(filteredData).reduce((total, channelInfo) =>
    total + Object.keys(channelInfo.models).length, 0
  );

  return (
    <Box>
      <Container sx={{ paddingTop: '40px' }}>
        <MainCard title="Supported Models">
          <Typography variant="body2" sx={{ mb: 3 }}>
            Browse all models supported by the server, grouped by channel/adaptor with pricing information.
          </Typography>

          <Grid container spacing={2} sx={{ mb: 3 }}>
            <Grid item xs={12} md={4}>
              <TextField
                fullWidth
                variant="outlined"
                placeholder="Search models..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
              />
            </Grid>
            <Grid item xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel>Filter by channels</InputLabel>
                <Select
                  multiple
                  value={selectedChannels}
                  onChange={(e) => setSelectedChannels(e.target.value)}
                  input={<OutlinedInput label="Filter by channels" />}
                  renderValue={(selected) => (
                    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                      {selected.map((value) => (
                        <Chip key={value} label={value} size="small" />
                      ))}
                    </Box>
                  )}
                >
                  {Object.keys(modelsData).sort().map((channelName) => (
                    <MenuItem key={channelName} value={channelName}>
                      {channelName} ({Object.keys(modelsData[channelName].models).length} models)
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={12} md={2}>
              <Button
                fullWidth
                variant="outlined"
                onClick={() => {
                  setSearchTerm('');
                  setSelectedChannels([]);
                }}
                sx={{ height: '56px' }}
              >
                Clear Filters
              </Button>
            </Grid>
          </Grid>

          {totalModels === 0 ? (
            <Alert severity="info">
              <Typography variant="h6">No models found</Typography>
              <Typography variant="body2">Try adjusting your search terms.</Typography>
            </Alert>
          ) : (
            <>
              <Typography variant="h6" sx={{ mb: 2 }}>
                Found {totalModels} models in {Object.keys(filteredData).length} channels
              </Typography>
              {Object.keys(filteredData)
                .sort()
                .map(channelName => renderChannelModels(channelName, filteredData[channelName]))}
            </>
          )}
        </MainCard>
      </Container>
    </Box>
  );
};

export default Models;
