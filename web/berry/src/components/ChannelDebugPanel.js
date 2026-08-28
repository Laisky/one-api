import React, { useState } from 'react';
import { 
  Button, 
  Dialog, 
  DialogTitle, 
  DialogContent, 
  DialogActions, 
  Typography, 
  List, 
  ListItem, 
  ListItemText, 
  Chip, 
  Box,
  Alert
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { API } from '../utils/api';
import { showError, showSuccess } from '../utils/common';

const ChannelDebugPanel = ({ channelId, channelType, channelName }) => {
  const theme = useTheme();
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [migrationStatus, setMigrationStatus] = useState(null);

  const checkMigrationStatus = async () => {
    setLoading(true);
    try {
      const res = await API.get(`/api/debug/channel/${channelId}/migration-status`);
      if (res.data.success) {
        setMigrationStatus(res.data.data);
      } else {
        showError('Failed to get migration status: ' + res.data.message);
      }
    } catch (error) {
      showError('Failed to check migration status: ' + error.message);
    }
    setLoading(false);
  };

  const fixChannel = async () => {
    setLoading(true);
    try {
      const res = await API.post(`/api/debug/channel/${channelId}/fix`);
      if (res.data.success) {
        showSuccess('Channel fixed successfully. Please refresh the page.');
        await checkMigrationStatus(); // Refresh status
      } else {
        showError('Failed to fix channel: ' + res.data.message);
      }
    } catch (error) {
      showError('Failed to fix channel: ' + error.message);
    }
    setLoading(false);
  };

  const debugChannel = async () => {
    setLoading(true);
    try {
      const res = await API.post(`/api/debug/channel/${channelId}/debug`);
      if (res.data.success) {
        showSuccess('Debug information logged. Check application logs.');
      } else {
        showError('Failed to debug channel: ' + res.data.message);
      }
    } catch (error) {
      showError('Failed to debug channel: ' + error.message);
    }
    setLoading(false);
  };

  const getMigrationStatusColor = (status) => {
    switch (status) {
      case 'migrated': return 'success';
      case 'migrated_with_legacy': return 'warning';
      case 'needs_migration': return 'error';
      case 'empty': return 'default';
      default: return 'default';
    }
  };

  const getMigrationStatusText = (status) => {
    switch (status) {
      case 'migrated': return 'Fully Migrated';
      case 'migrated_with_legacy': return 'Migrated (Legacy Data Present)';
      case 'needs_migration': return 'Needs Migration';
      case 'empty': return 'No Pricing Data';
      default: return 'Unknown';
    }
  };

  return (
    <>
      <Button 
        size="small" 
        variant="outlined"
        onClick={() => {
          setOpen(true);
          checkMigrationStatus();
        }}
        sx={{ ml: 1 }}
      >
        Debug
      </Button>

      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>
          Channel Debug: {channelName} (ID: {channelId})
        </DialogTitle>
        <DialogContent>
          <Typography variant="h6" gutterBottom>
            Migration Status
          </Typography>
          
          {migrationStatus && (
            <>
              <Chip 
                label={getMigrationStatusText(migrationStatus.migration_status)}
                color={getMigrationStatusColor(migrationStatus.migration_status)}
                size="medium"
                sx={{ mb: 2 }}
              />
              
              <Box sx={{ mt: 2 }}>
                <Typography variant="subtitle2" gutterBottom>
                  Channel Info:
                </Typography>
                <List dense>
                  <ListItem>
                    <ListItemText primary={`UUID: ${migrationStatus.channel_uuid}`} />
                  </ListItem>
                  <ListItem>
                    <ListItemText primary={`Name: ${migrationStatus.channel_name}`} />
                  </ListItem>
                  <ListItem>
                    <ListItemText primary={`Type: ${migrationStatus.channel_type}`} />
                  </ListItem>
                </List>
              </Box>

              <Box sx={{ mt: 2 }}>
                <Typography variant="subtitle2" gutterBottom>
                  Data Status:
                </Typography>
                <List dense>
                  <ListItem>
                    <Chip 
                      label={migrationStatus.has_model_configs ? 'YES' : 'NO'}
                      color={migrationStatus.has_model_configs ? 'success' : 'error'}
                      size="small"
                      sx={{ mr: 1 }}
                    />
                    <ListItemText 
                      primary={`Has Model Configs (Unified Format)${migrationStatus.model_configs_count ? ` - ${migrationStatus.model_configs_count} models` : ''}`}
                    />
                  </ListItem>
                  <ListItem>
                    <Chip 
                      label={migrationStatus.has_model_ratio ? 'YES' : 'NO'}
                      color={migrationStatus.has_model_ratio ? 'warning' : 'default'}
                      size="small"
                      sx={{ mr: 1 }}
                    />
                    <ListItemText 
                      primary={`Has Model Ratio (Legacy)${migrationStatus.model_ratio_count ? ` - ${migrationStatus.model_ratio_count} models` : ''}`}
                    />
                  </ListItem>
                  <ListItem>
                    <Chip 
                      label={migrationStatus.has_completion_ratio ? 'YES' : 'NO'}
                      color={migrationStatus.has_completion_ratio ? 'warning' : 'default'}
                      size="small"
                      sx={{ mr: 1 }}
                    />
                    <ListItemText primary="Has Completion Ratio (Legacy)" />
                  </ListItem>
                </List>
              </Box>

              {migrationStatus.model_configs_models && (
                <Box sx={{ mt: 2 }}>
                  <Typography variant="subtitle2" gutterBottom>
                    Models in Unified Config:
                  </Typography>
                  <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                    {migrationStatus.model_configs_models.map(model => (
                      <Chip key={model} label={model} size="small" />
                    ))}
                  </Box>
                </Box>
              )}

              {migrationStatus.model_ratio_models && (
                <Box sx={{ mt: 2 }}>
                  <Typography variant="subtitle2" gutterBottom>
                    Models in Legacy Ratio:
                  </Typography>
                  <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                    {migrationStatus.model_ratio_models.map(model => (
                      <Chip key={model} label={model} size="small" color="warning" />
                    ))}
                  </Box>
                </Box>
              )}

              {migrationStatus.migration_status === 'needs_migration' && (
                <Alert severity="warning" sx={{ mt: 2 }}>
                  <Typography variant="body2">
                    <strong>Migration Needed</strong><br />
                    This channel has legacy pricing data that needs to be migrated to the unified format.
                  </Typography>
                </Alert>
              )}

              {migrationStatus.migration_status === 'migrated_with_legacy' && (
                <Alert severity="info" sx={{ mt: 2 }}>
                  <Typography variant="body2">
                    <strong>Migration Complete with Legacy Data</strong><br />
                    This channel has been migrated but still contains legacy data for backward compatibility.
                  </Typography>
                </Alert>
              )}
            </>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={debugChannel} disabled={loading}>
            Log Debug Info
          </Button>
          <Button onClick={checkMigrationStatus} disabled={loading}>
            Refresh Status
          </Button>
          {migrationStatus && migrationStatus.migration_status === 'needs_migration' && (
            <Button color="warning" onClick={fixChannel} disabled={loading}>
              Fix Channel
            </Button>
          )}
          <Button onClick={() => setOpen(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

export default ChannelDebugPanel;
