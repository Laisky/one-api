import { showError as reportUIError } from '../../../utils/common';
import React, { useState, useEffect } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  Box,
  Grid,
  Chip,
  CircularProgress,
  Alert,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Divider,
  IconButton,
} from '@mui/material';
import {
  Close as CloseIcon,
  Timeline as TimelineIcon,
  Info as InfoIcon,
  PlayArrow as PlayIcon,
  Send as SendIcon,
  Reply as ReplyIcon,
  CheckCircle as CheckIcon,
  Flag as FlagIcon,
  ArrowForward as ArrowIcon,
} from '@mui/icons-material';
import { API } from 'utils/api';
import { timestamp2string, renderQuota } from 'utils/common';

const TracingModal = ({ open, onClose, logId }) => {
  const [loading, setLoading] = useState(false);
  const [traceData, setTraceData] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (open && logId) {
      fetchTraceData().catch(reportUIError);
    }
  }, [open, logId]);

  const fetchTraceData = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await API.get(`/api/trace/log/${logId}`);
      if (res.data.success) {
        setTraceData(res.data.data);
      } else {
        setError(res.data.message || 'Failed to fetch trace data');
      }
    } catch (err) {
      setError(err.response?.data?.message || 'Failed to fetch trace data');
    } finally {
      setLoading(false);
    }
  };

  const formatDuration = (milliseconds) => {
    if (!milliseconds) return 'N/A';
    if (milliseconds < 1000) {
      return `${milliseconds}ms`;
    }
    return `${(milliseconds / 1000).toFixed(2)}s`;
  };

  const formatTimestamp = (timestamp) => {
    if (!timestamp) return 'N/A';
    return timestamp2string(Math.floor(timestamp / 1000));
  };

  const getStatusColor = (status) => {
    if (status >= 200 && status < 300) return 'success';
    if (status >= 300 && status < 400) return 'warning';
    if (status >= 400 && status < 500) return 'error';
    if (status >= 500) return 'error';
    return 'default';
  };

  const getEventIcon = (eventType) => {
    switch (eventType) {
      case 'received': return <PlayIcon color="primary" />;
      case 'forwarded': return <ArrowIcon color="info" />;
      case 'upstream_response': return <ReplyIcon color="secondary" />;
      case 'client_response': return <SendIcon color="warning" />;
      case 'upstream_completed': return <CheckIcon color="success" />;
      case 'completed': return <FlagIcon color="success" />;
      default: return <TimelineIcon />;
    }
  };

  const renderTimeline = () => {
    if (!traceData?.timestamps) return null;

    const { timestamps, durations } = traceData;
    const timelineEvents = [];

    if (timestamps.request_received) {
      timelineEvents.push({
        key: 'received',
        title: '请求接收',
        timestamp: timestamps.request_received,
        duration: null,
      });
    }

    if (timestamps.request_forwarded) {
      timelineEvents.push({
        key: 'forwarded',
        title: '转发到上游',
        timestamp: timestamps.request_forwarded,
        duration: durations?.processing_time,
      });
    }

    if (timestamps.first_upstream_response) {
      timelineEvents.push({
        key: 'upstream_response',
        title: '上游首次响应',
        timestamp: timestamps.first_upstream_response,
        duration: durations?.upstream_response_time,
      });
    }

    if (timestamps.first_client_response) {
      timelineEvents.push({
        key: 'client_response',
        title: '客户端首次响应',
        timestamp: timestamps.first_client_response,
        duration: durations?.response_processing_time,
      });
    }

    if (timestamps.upstream_completed) {
      timelineEvents.push({
        key: 'upstream_completed',
        title: '上游完成',
        timestamp: timestamps.upstream_completed,
        duration: durations?.streaming_time,
      });
    }

    if (timestamps.request_completed) {
      timelineEvents.push({
        key: 'completed',
        title: '请求完成',
        timestamp: timestamps.request_completed,
        duration: null,
      });
    }

    return (
      <Box sx={{ mt: 2 }}>
        <Typography variant="h6" gutterBottom>
          <TimelineIcon sx={{ mr: 1, verticalAlign: 'middle' }} />
          请求时间线
        </Typography>
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>事件</TableCell>
                <TableCell>时间戳</TableCell>
                <TableCell>耗时</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {timelineEvents.map((event) => (
                <TableRow key={event.key}>
                  <TableCell>
                    <Box sx={{ display: 'flex', alignItems: 'center' }}>
                      {getEventIcon(event.key)}
                      <Typography variant="body2" sx={{ ml: 1 }}>
                        {event.title}
                      </Typography>
                    </Box>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                      {formatTimestamp(event.timestamp)}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">
                      {event.duration ? formatDuration(event.duration) : '-'}
                    </Typography>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
        
        {durations?.total_time && (
          <Box sx={{ mt: 2, p: 2, bgcolor: 'primary.light', borderRadius: 1 }}>
            <Typography variant="subtitle1" color="primary.contrastText">
              总请求时间: {formatDuration(durations.total_time)}
            </Typography>
          </Box>
        )}
      </Box>
    );
  };

  const renderRequestInfo = () => {
    if (!traceData) return null;

    return (
      <Box>
        <Typography variant="h6" gutterBottom>
          <InfoIcon sx={{ mr: 1, verticalAlign: 'middle' }} />
          请求信息
        </Typography>
        <Grid container spacing={2}>
          <Grid item xs={12} md={6}>
            <Typography variant="subtitle2" color="text.secondary">
              URL
            </Typography>
            <Typography variant="body2" sx={{ wordBreak: 'break-all', fontFamily: 'monospace' }}>
              {traceData.url}
            </Typography>
          </Grid>
          <Grid item xs={6} md={3}>
            <Typography variant="subtitle2" color="text.secondary">
              方法
            </Typography>
            <Chip label={traceData.method} color="primary" size="small" />
          </Grid>
          <Grid item xs={6} md={3}>
            <Typography variant="subtitle2" color="text.secondary">
              状态码
            </Typography>
            <Chip 
              label={traceData.status || 'N/A'} 
              color={getStatusColor(traceData.status)} 
              size="small" 
            />
          </Grid>
          <Grid item xs={6} md={6}>
            <Typography variant="subtitle2" color="text.secondary">
              请求体大小
            </Typography>
            <Typography variant="body2">
              {traceData.body_size ? `${traceData.body_size} bytes` : 'N/A'}
            </Typography>
          </Grid>
          <Grid item xs={6} md={6}>
            <Typography variant="subtitle2" color="text.secondary">
              用户
            </Typography>
            <Typography variant="body2">
              {traceData.log?.username || 'N/A'}
            </Typography>
          </Grid>
          <Grid item xs={12}>
            <Typography variant="subtitle2" color="text.secondary">
              追踪ID
            </Typography>
            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.875rem' }}>
              {traceData.trace_id}
            </Typography>
          </Grid>
        </Grid>
      </Box>
    );
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle>
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <Typography variant="h6">
            <TimelineIcon sx={{ mr: 1, verticalAlign: 'middle' }} />
            请求追踪详情
          </Typography>
          <IconButton onClick={onClose} size="small">
            <CloseIcon />
          </IconButton>
        </Box>
      </DialogTitle>
      <DialogContent dividers>
        {loading && (
          <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
            <CircularProgress />
            <Typography sx={{ ml: 2 }}>加载追踪数据...</Typography>
          </Box>
        )}
        
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        
        {traceData && !loading && (
          <>
            {renderRequestInfo()}
            <Divider sx={{ my: 3 }} />
            {renderTimeline()}
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} startIcon={<CloseIcon />}>
          关闭
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default TracingModal;
