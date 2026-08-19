import { useEffect, useState } from 'react';
import { Grid, Typography, Card, CardContent, TextField, Button, ButtonGroup, Alert, Autocomplete } from '@mui/material';
import { gridSpacing } from 'store/constant';
import StatisticalLineChartCard from './component/StatisticalLineChartCard';
import StatisticalBarChart from './component/StatisticalBarChart';
import { generateChartOptions, getLastSevenDays } from 'utils/chart';
import { API } from 'utils/api';
import { showError, calculateQuota, renderNumber, isRoot } from 'utils/common';
import UserCard from 'ui-component/cards/UserCard';

const Dashboard = () => {
  const [isLoading, setLoading] = useState(true);
  const [statisticalData, setStatisticalData] = useState([]);
  const [requestChart, setRequestChart] = useState(null);
  const [quotaChart, setQuotaChart] = useState(null);
  const [tokenChart, setTokenChart] = useState(null);
  const [users, setUsers] = useState([]);
  const [dashboardUsers, setDashboardUsers] = useState([]);
  const [selectedUserId, setSelectedUserId] = useState('');
  const [isRootUser, setIsRootUser] = useState(false);
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');
  const [dateError, setDateError] = useState('');
  const [statisticsMetric, setStatisticsMetric] = useState('expenses'); // Default to expenses for berry template

  const handleStatisticsMetricChange = (event, newValue) => {
    setStatisticsMetric(newValue);
  };

  const fetchDashboardUsers = async () => {
    try {
      const res = await API.get('/api/user/dashboard/users');
      const { success, message, data } = res.data;
      if (success) {
        setDashboardUsers(data || []);
        // Only set default selection if no user is currently selected
        if (!selectedUserId) {
          setSelectedUserId('all');
        }
      } else {
        showError(message);
      }
    } catch (error) {
      console.error('Failed to fetch dashboard users:', error);
    }
  };

  const userDashboard = async (userId = selectedUserId, customFromDate = fromDate, customToDate = toDate) => {
    let url = '/api/user/dashboard';
    const params = new URLSearchParams();

    if (isRootUser && userId && userId !== 'all') {
      params.append('user_id', userId);
    }

    if (customFromDate && customToDate) {
      params.append('from_date', customFromDate);
      params.append('to_date', customToDate);
    }

    if (params.toString()) {
      url += `?${params.toString()}`;
    }

    const res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
      if (data) {
        // Handle new API response structure
        const dashboardData = data.logs || data || [];
        let lineData = getLineDataGroup(dashboardData);
        setRequestChart(getLineCardOption(lineData, 'RequestCount'));
        setQuotaChart(getLineCardOption(lineData, 'Quota'));
        setTokenChart(getLineCardOption(lineData, 'PromptTokens'));
        setStatisticalData(getBarDataGroup(dashboardData, statisticsMetric));
      }
      setDateError('');
    } else {
      showError(message);
      setDateError(message || 'Failed to fetch dashboard data');
    }
    setLoading(false);
  };

  const loadUser = async () => {
    let res = await API.get(`/api/user/self`);
    const { success, message, data } = res.data;
    if (success) {
      setUsers(data);
    } else {
      showError(message);
    }
  };

  useEffect(() => {
    const rootUser = isRoot();
    setIsRootUser(rootUser);

    // Initialize default date range (last 7 days)
    const today = new Date();
    const sevenDaysAgo = new Date();
    sevenDaysAgo.setDate(today.getDate() - 6);

    setToDate(today.toISOString().split('T')[0]);
    setFromDate(sevenDaysAgo.toISOString().split('T')[0]);

    if (rootUser) {
      fetchDashboardUsers();
    } else {
      userDashboard();
    }
    loadUser();
  }, []);

  useEffect(() => {
    if (selectedUserId && fromDate && toDate) {
      userDashboard(selectedUserId, fromDate, toDate);
    }
  }, [selectedUserId, fromDate, toDate]);

  useEffect(() => {
    // Refresh data when statistics metric changes
    if (selectedUserId && fromDate && toDate) {
      userDashboard(selectedUserId, fromDate, toDate);
    }
  }, [statisticsMetric]);



  const handleDateChange = (field, value) => {
    if (field === 'from') {
      setFromDate(value);
    } else {
      setToDate(value);
    }
  };

  const handleRefresh = () => {
    userDashboard(selectedUserId, fromDate, toDate);
  };

  const handlePresetDateRange = (preset) => {
    const today = new Date();
    let startDate;

    switch (preset) {
      case 'today':
        startDate = new Date(today);
        break;
      case '7days':
        startDate = new Date();
        startDate.setDate(today.getDate() - 6);
        break;
      case '30days':
        startDate = new Date();
        startDate.setDate(today.getDate() - 29);
        break;
      default:
        return;
    }

    const fromDateStr = startDate.toISOString().split('T')[0];
    const toDateStr = today.toISOString().split('T')[0];

    // Set dates and immediately trigger data fetch
    setFromDate(fromDateStr);
    setToDate(toDateStr);
    userDashboard(selectedUserId, fromDateStr, toDateStr);
  };

  const getMaxDate = () => {
    const today = new Date();
    return today.toISOString().split('T')[0];
  };

  const getMinDate = () => {
    if (isRootUser) {
      // Root users can go back 1 year
      const oneYearAgo = new Date();
      oneYearAgo.setFullYear(oneYearAgo.getFullYear() - 1);
      return oneYearAgo.toISOString().split('T')[0];
    } else {
      // Regular users can only go back 7 days from today
      const sevenDaysAgo = new Date();
      sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7);
      return sevenDaysAgo.toISOString().split('T')[0];
    }
  };

  return (
    <Grid container spacing={gridSpacing}>
      {/* Controls for root users and date range */}
      <Grid item xs={12}>
        <Card>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              Dashboard Controls
            </Typography>

            {/* User selector for root users */}
            {isRootUser && (
              <Grid container spacing={2} sx={{ mb: 2 }}>
                <Grid item xs={12}>
                  <Autocomplete
                    fullWidth
                    options={dashboardUsers}
                    getOptionLabel={(option) =>
                      option.id === 0 ? option.display_name : `${option.display_name || option.username} (${option.username})`
                    }
                    value={dashboardUsers.find(user =>
                      (user.id === 0 ? 'all' : String(user.uuid || user.id)) === selectedUserId
                    ) || null}
                    onChange={(_, newValue) => {
                      const value = newValue ? (newValue.id === 0 ? 'all' : String(newValue.uuid || newValue.id)) : '';
                      setSelectedUserId(value);
                    }}
                    renderInput={(params) => (
                      <TextField
                        {...params}
                        label="Select User"
                        placeholder="Search and select a user to view dashboard"
                      />
                    )}
                    renderOption={(props, option) => (
                      <li {...props}>
                        <div>
                          <div>{option.id === 0 ? option.display_name : `${option.display_name || option.username} (${option.username})`}</div>
                          <div style={{ fontSize: '0.8em', color: '#666' }}>
                            {option.id === 0 ? 'View site-wide statistics' : `User ID: ${option.id}`}
                          </div>
                        </div>
                      </li>
                    )}
                    noOptionsText="No users found"
                    clearOnEscape
                  />
                </Grid>
              </Grid>
            )}

            {/* Date range preset buttons */}
            <Grid container spacing={2} sx={{ mb: 2 }}>
              <Grid item xs={12}>
                <Typography variant="subtitle2" gutterBottom>
                  Quick Date Selection
                </Typography>
                <ButtonGroup variant="outlined" size="small">
                  <Button onClick={() => handlePresetDateRange('today')}>
                    Today
                  </Button>
                  <Button onClick={() => handlePresetDateRange('7days')}>
                    Last 7 Days
                  </Button>
                  <Button onClick={() => handlePresetDateRange('30days')}>
                    Last 30 Days
                  </Button>
                </ButtonGroup>
              </Grid>
            </Grid>

            {/* Date range selector */}
            <Grid container spacing={2} alignItems="end">
              <Grid item xs={12} sm={4}>
                <TextField
                  label="From Date"
                  type="date"
                  value={fromDate}
                  onChange={(e) => handleDateChange('from', e.target.value)}
                  InputLabelProps={{
                    shrink: true,
                  }}
                  inputProps={{
                    min: getMinDate(),
                    max: getMaxDate(),
                  }}
                  fullWidth
                />
              </Grid>
              <Grid item xs={12} sm={4}>
                <TextField
                  label="To Date"
                  type="date"
                  value={toDate}
                  onChange={(e) => handleDateChange('to', e.target.value)}
                  InputLabelProps={{
                    shrink: true,
                  }}
                  inputProps={{
                    min: getMinDate(),
                    max: getMaxDate(),
                  }}
                  fullWidth
                />
              </Grid>
              <Grid item xs={12} sm={4}>
                <Button
                  variant="contained"
                  onClick={handleRefresh}
                  fullWidth
                  sx={{ height: '56px' }}
                >
                  Refresh
                </Button>
              </Grid>
            </Grid>

            {dateError && (
              <Alert severity="error" sx={{ mt: 2 }}>
                {dateError}
              </Alert>
            )}

            <Alert severity="info" sx={{ mt: 2 }}>
              <Typography variant="body2">
                {isRootUser
                  ? 'As root user, you can select up to 1 year of data.'
                  : 'You can select up to 7 days of data.'
                }
              </Typography>
            </Alert>
          </CardContent>
        </Card>
      </Grid>

      <Grid item xs={12}>
        <Grid container spacing={gridSpacing}>
          <Grid item lg={4} xs={12}>
            <StatisticalLineChartCard
              isLoading={isLoading}
              title="总请求量"
              chartData={requestChart?.chartData}
              todayValue={requestChart?.todayValue}
            />
          </Grid>
          <Grid item lg={4} xs={12}>
            <StatisticalLineChartCard
              isLoading={isLoading}
              title="总消费"
              chartData={quotaChart?.chartData}
              todayValue={quotaChart?.todayValue}
            />
          </Grid>
          <Grid item lg={4} xs={12}>
            <StatisticalLineChartCard
              isLoading={isLoading}
              title="总 Token"
              chartData={tokenChart?.chartData}
              todayValue={tokenChart?.todayValue}
            />
          </Grid>
        </Grid>
      </Grid>
      <Grid item xs={12}>
        <Grid container spacing={gridSpacing}>
          <Grid item lg={8} xs={12}>
            <StatisticalBarChart
              isLoading={isLoading}
              chartDatas={statisticalData}
              statisticsMetric={statisticsMetric}
              onMetricChange={handleStatisticsMetricChange}
            />
          </Grid>
          <Grid item lg={4} xs={12}>
            <UserCard>
              <Grid container spacing={gridSpacing} justifyContent="center" alignItems="center" paddingTop={'20px'}>
                <Grid item xs={4}>
                  <Typography variant="h4">余额：</Typography>
                </Grid>
                <Grid item xs={8}>
                  <Typography variant="h3"> {users?.quota ? '$' + calculateQuota(users.quota) : '未知'}</Typography>
                </Grid>
                <Grid item xs={4}>
                  <Typography variant="h4">已使用：</Typography>
                </Grid>
                <Grid item xs={8}>
                  <Typography variant="h3"> {users?.used_quota ? '$' + calculateQuota(users.used_quota) : '未知'}</Typography>
                </Grid>
                <Grid item xs={4}>
                  <Typography variant="h4">调用次数：</Typography>
                </Grid>
                <Grid item xs={8}>
                  <Typography variant="h3"> {users?.request_count || '未知'}</Typography>
                </Grid>
              </Grid>
            </UserCard>
          </Grid>
        </Grid>
      </Grid>
    </Grid>
  );
};
export default Dashboard;

function getLineDataGroup(statisticalData) {
  let groupedData = statisticalData.reduce((acc, cur) => {
    if (!acc[cur.Day]) {
      acc[cur.Day] = {
        date: cur.Day,
        RequestCount: 0,
        Quota: 0,
        PromptTokens: 0,
        CompletionTokens: 0
      };
    }
    acc[cur.Day].RequestCount += cur.RequestCount;
    acc[cur.Day].Quota += cur.Quota;
    acc[cur.Day].PromptTokens += cur.PromptTokens;
    acc[cur.Day].CompletionTokens += cur.CompletionTokens;
    return acc;
  }, {});
  let lastSevenDays = getLastSevenDays();
  return lastSevenDays.map((day) => {
    if (!groupedData[day]) {
      return {
        date: day,
        RequestCount: 0,
        Quota: 0,
        PromptTokens: 0,
        CompletionTokens: 0
      };
    } else {
      return groupedData[day];
    }
  });
}

function getBarDataGroup(data, metric = 'expenses') {
  const lastSevenDays = getLastSevenDays();
  const result = [];
  const map = new Map();

  // Calculate total request count for each model to sort by usage
  const modelRequestCounts = {};
  data.forEach(item => {
    if (!modelRequestCounts[item.ModelName]) {
      modelRequestCounts[item.ModelName] = 0;
    }
    modelRequestCounts[item.ModelName] += item.RequestCount;
  });

  for (const item of data) {
    if (!map.has(item.ModelName)) {
      const newData = { name: item.ModelName, data: new Array(7) };
      map.set(item.ModelName, newData);
      result.push(newData);
    }
    const index = lastSevenDays.indexOf(item.Day);
    if (index !== -1) {
      let value;
      switch (metric) {
        case 'requests':
          value = item.RequestCount;
          break;
        case 'tokens':
          value = item.PromptTokens + item.CompletionTokens;
          break;
        case 'expenses':
        default:
          value = calculateQuota(item.Quota, 3);
          break;
      }
      map.get(item.ModelName).data[index] = value;
    }
  }

  for (const item of result) {
    for (let i = 0; i < 7; i++) {
      if (item.data[i] === undefined) {
        item.data[i] = 0;
      }
    }
  }

  // Sort result by total request count (descending order)
  result.sort((a, b) => modelRequestCounts[b.name] - modelRequestCounts[a.name]);

  return { data: result, xaxis: lastSevenDays };
}

function getLineCardOption(lineDataGroup, field) {
  let todayValue = 0;
  let chartData = null;
  const lastItem = lineDataGroup.length - 1;
  let lineData = lineDataGroup.map((item, index) => {
    let tmp = {
      date: item.date,
      value: item[field]
    };
    switch (field) {
      case 'Quota':
        tmp.value = calculateQuota(item.Quota, 3);
        break;
      case 'PromptTokens':
        tmp.value += item.CompletionTokens;
        break;
    }

    if (index == lastItem) {
      todayValue = tmp.value;
    }
    return tmp;
  });

  switch (field) {
    case 'RequestCount':
      chartData = generateChartOptions(lineData, '次');
      todayValue = renderNumber(todayValue);
      break;
    case 'Quota':
      chartData = generateChartOptions(lineData, '美元');
      todayValue = '$' + renderNumber(todayValue);
      break;
    case 'PromptTokens':
      chartData = generateChartOptions(lineData, '');
      todayValue = renderNumber(todayValue);
      break;
  }

  return { chartData: chartData, todayValue: todayValue };
}
