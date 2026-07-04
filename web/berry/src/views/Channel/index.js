import { useState, useEffect } from 'react';
import { showError, showSuccess, showInfo, loadChannelModels } from 'utils/common';

import { useTheme } from '@mui/material/styles';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableContainer from '@mui/material/TableContainer';
import PerfectScrollbar from 'react-perfect-scrollbar';
import TablePagination from '@mui/material/TablePagination';
import LinearProgress from '@mui/material/LinearProgress';
import ButtonGroup from '@mui/material/ButtonGroup';
import Toolbar from '@mui/material/Toolbar';
import useMediaQuery from '@mui/material/useMediaQuery';

import { Button, IconButton, Card, Box, Stack, Container, Typography, Divider } from '@mui/material';
import ChannelTableRow from './component/TableRow';
import ChannelTableHead from './component/TableHead';
import TableToolBar from 'ui-component/TableToolBar';
import { API } from 'utils/api';
import { ITEMS_PER_PAGE } from 'constants';
import { IconRefresh, IconHttpDelete, IconPlus, IconBrandSpeedtest, IconCoinYuan } from '@tabler/icons-react';
import EditeModal from './component/EditModal';
import PricingModal from './component/PricingModal';

// ----------------------------------------------------------------------
// CHANNEL_OPTIONS,
const refPayload = (ref) => (typeof ref === 'string' ? { uuid: ref } : { id: ref });
const itemRef = (item) => item?.uuid || item?.id;

export default function ChannelPage() {
  const [channels, setChannels] = useState([]);
  const [totalChannels, setTotalChannels] = useState(0);
  const [activePage, setActivePage] = useState(0);
  const [searching, setSearching] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState('');
  const theme = useTheme();
  const matchUpMd = useMediaQuery(theme.breakpoints.up('sm'));
  const [openModal, setOpenModal] = useState(false);
  const [editChannelId, setEditChannelId] = useState('');
  const [openPricingModal, setOpenPricingModal] = useState(false);
  const [pricingChannelId, setPricingChannelId] = useState(0);
  const [pricingChannelName, setPricingChannelName] = useState('');
  const [pricingChannelType, setPricingChannelType] = useState(0);

  const loadChannels = async (startIdx) => {
    setSearching(true);
    try {
      const res = await API.get(`/api/channel/?p=${startIdx}&size=${ITEMS_PER_PAGE}`);
      const { success, message, data, total } = res.data;
      if (success) {
        const resolvedTotal = typeof total === 'number' ? total : (Array.isArray(data) ? data.length : 0);
        setTotalChannels(resolvedTotal);
        setChannels((prev) => {
          if (startIdx === 0) {
            return Array.isArray(data) ? data : [];
          }
          const next = Array.isArray(prev) ? [...prev] : [];
          const pageData = Array.isArray(data) ? data : [];
          next.splice(startIdx * ITEMS_PER_PAGE, pageData.length, ...pageData);
          return next;
        });
      } else {
        showError(message);
      }
    } catch (err) {
      showError(err?.message || err);
    }
    setSearching(false);
  };

  const onPaginationChange = (event, activePage) => {
    (async () => {
      const loadedPages = Math.ceil(channels.length / ITEMS_PER_PAGE);
      const expectedPages = Math.ceil(totalChannels / ITEMS_PER_PAGE);
      if (activePage >= loadedPages && activePage < expectedPages) {
        await loadChannels(activePage);
      }
      setActivePage(activePage);
    })();
  };

  const searchChannels = async (event) => {
    event.preventDefault();
    if (searchKeyword === '') {
      await loadChannels(0);
      setActivePage(0);
      return;
    }
    setSearching(true);
    try {
      const res = await API.get(`/api/channel/search?keyword=${searchKeyword}`);
      const { success, message, data } = res.data;
      if (success) {
        setChannels(Array.isArray(data) ? data : []);
        setTotalChannels(Array.isArray(data) ? data.length : 0);
        setActivePage(0);
      } else {
        showError(message);
      }
    } catch (err) {
      showError(err?.message || err);
    }
    setSearching(false);
  };

  const handleSearchKeyword = (event) => {
    setSearchKeyword(event.target.value);
  };

  const manageChannel = async (id, action, value) => {
    const url = '/api/channel/';
    let data = refPayload(id);
    let res;
    switch (action) {
      case 'delete':
        res = await API.delete(url + id);
        break;
      case 'status':
        res = await API.put(`${url}?status_only=1`, {
          ...data,
          status: value
        });
        break;
      case 'priority':
        if (value === '') {
          return;
        }
        {
          const parsedPriority = parseInt(value, 10);
          if (Number.isNaN(parsedPriority)) {
            showError('优先级必须是数字');
            return;
          }
          const channel = channels.find((item) => itemRef(item) === id || item.id === id);
          if (!channel) {
            showError('未找到对应的渠道，稍后重试');
            return;
          }
          res = await API.put(url, {
            ...data,
            name: channel.name,
            priority: parsedPriority
          });
        }
        break;
      case 'test':
        res = await API.get(url + `test/${id}`);
        break;
      default:
        showError(`未知操作类型: ${action}`);
        return;
    }
    const { success, message, data: responseData } = res.data;
    if (success) {
      showSuccess('操作成功完成！');
      if (action === 'delete') {
        await handleRefresh();
      } else if (action === 'priority') {
        setChannels((prev) =>
          prev.map((item) => (itemRef(item) === id || item.id === id ? { ...item, priority: responseData?.priority ?? item.priority } : item))
        );
      }
    } else {
      showError(message);
    }

    return res.data;
  };

  // 处理刷新
  const handleRefresh = async () => {
    await loadChannels(activePage);
  };

  // 处理测试所有启用渠道
  const testAllChannels = async () => {
    const res = await API.get(`/api/channel/test`);
    const { success, message } = res.data;
    if (success) {
      showInfo('已成功开始测试所有渠道，请刷新页面查看结果。');
    } else {
      showError(message);
    }
  };

  // 处理删除所有禁用渠道
  const deleteAllDisabledChannels = async () => {
    const res = await API.delete(`/api/channel/disabled`);
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(`已删除所有禁用渠道，共计 ${data} 个`);
      await handleRefresh();
    } else {
      showError(message);
    }
  };

  // 处理更新所有启用渠道余额
  const updateAllChannelsBalance = async () => {
    setSearching(true);
    const res = await API.get(`/api/channel/update_balance`);
    const { success, message } = res.data;
    if (success) {
      showInfo('已更新完毕所有已启用渠道余额！');
    } else {
      showError(message);
    }
    setSearching(false);
  };

  const handleOpenModal = (channelId) => {
    setEditChannelId(channelId);
    setOpenModal(true);
  };

  const handleCloseModal = () => {
    setOpenModal(false);
    setEditChannelId('');
  };

  const handleOkModal = (status) => {
    if (status === true) {
      handleCloseModal();
      handleRefresh();
    }
  };

  const handleOpenPricingModal = (channelId, channelName, channelType) => {
    setPricingChannelId(channelId);
    setPricingChannelName(channelName);
    setPricingChannelType(channelType);
    setOpenPricingModal(true);
  };

  const handleClosePricingModal = () => {
    setOpenPricingModal(false);
    setPricingChannelId(0);
    setPricingChannelName('');
    setPricingChannelType(0);
  };

  useEffect(() => {
    loadChannels(0)
      .then()
      .catch((reason) => {
        showError(reason);
      });
    loadChannelModels().then();
  }, []);

  return (
    <>
      <Stack direction="row" alignItems="center" justifyContent="space-between" mb={2.5}>
        <Typography variant="h4">渠道</Typography>
        <Button variant="contained" color="primary" startIcon={<IconPlus />} onClick={() => handleOpenModal('')}>
          新建渠道
        </Button>
      </Stack>
      <Card>
        <Box component="form" onSubmit={searchChannels} noValidate sx={{ marginTop: 2 }}>
          <TableToolBar filterName={searchKeyword} handleFilterName={handleSearchKeyword} placeholder={'搜索渠道的 ID，名称和密钥 ...'} />
        </Box>
        <Toolbar
          sx={{
            textAlign: 'right',
            height: 50,
            display: 'flex',
            justifyContent: 'space-between',
            p: (theme) => theme.spacing(0, 1, 0, 3)
          }}
        >
          <Container>
            {matchUpMd ? (
              <ButtonGroup variant="outlined" aria-label="outlined small primary button group" sx={{ marginBottom: 2 }}>
                <Button onClick={handleRefresh} startIcon={<IconRefresh width={'18px'} />}>
                  刷新
                </Button>
                <Button onClick={testAllChannels} startIcon={<IconBrandSpeedtest width={'18px'} />}>
                  测试启用渠道
                </Button>
                {/*<Button onClick={updateAllChannelsBalance} startIcon={<IconCoinYuan width={'18px'} />}>*/}
                {/*  更新启用余额*/}
                {/*</Button>*/}
                <Button onClick={deleteAllDisabledChannels} startIcon={<IconHttpDelete width={'18px'} />}>
                  删除禁用渠道
                </Button>
              </ButtonGroup>
            ) : (
              <Stack
                direction="row"
                spacing={1}
                divider={<Divider orientation="vertical" flexItem />}
                justifyContent="space-around"
                alignItems="center"
              >
                <IconButton onClick={handleRefresh} size="large">
                  <IconRefresh />
                </IconButton>
                <IconButton onClick={testAllChannels} size="large">
                  <IconBrandSpeedtest />
                </IconButton>
                <IconButton onClick={updateAllChannelsBalance} size="large">
                  <IconCoinYuan />
                </IconButton>
                <IconButton onClick={deleteAllDisabledChannels} size="large">
                  <IconHttpDelete />
                </IconButton>
              </Stack>
            )}
          </Container>
        </Toolbar>
        {searching && <LinearProgress />}
        <PerfectScrollbar component="div">
          <TableContainer sx={{ overflow: 'unset' }}>
            <Table sx={{ minWidth: 800 }}>
              <ChannelTableHead />
              <TableBody>
                {channels.slice(activePage * ITEMS_PER_PAGE, (activePage + 1) * ITEMS_PER_PAGE).map((row) => (
                  <ChannelTableRow
                    item={row}
                    manageChannel={manageChannel}
                    key={itemRef(row)}
                    handleOpenModal={handleOpenModal}
                    setModalChannelId={setEditChannelId}
                    handleOpenPricingModal={handleOpenPricingModal}
                  />
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </PerfectScrollbar>
        <TablePagination
          page={activePage}
          component="div"
          count={totalChannels}
          rowsPerPage={ITEMS_PER_PAGE}
          onPageChange={onPaginationChange}
          rowsPerPageOptions={[ITEMS_PER_PAGE]}
        />
      </Card>
      <EditeModal open={openModal} onCancel={handleCloseModal} onOk={handleOkModal} channelId={editChannelId} />
      <PricingModal
        open={openPricingModal}
        onClose={handleClosePricingModal}
        channelId={pricingChannelId}
        channelName={pricingChannelName}
        channelType={pricingChannelType}
      />
    </>
  );
}
