import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Form,
  Icon,
  Label,
  Popup,
  Table,
  Dropdown,
} from 'semantic-ui-react';
import { Link } from 'react-router-dom';
import {
  API,
  showError,
  showSuccess,
  timestamp2string,
  renderQuota,
  copy,
} from '../helpers';
import { ITEMS_PER_PAGE } from '../constants';
import BaseTable from './shared/BaseTable';
import { cleanDisplay } from './shared/tableUtils';

function renderTimestamp(timestamp) {
  return <>{timestamp2string(timestamp)}</>;
}

function renderTokenStatus(status, t) {
  switch (status) {
    case 1:
      return (
        <Label basic color='green'>
          {t('status_enabled')}
        </Label>
      );
    case 2:
      return (
        <Label basic color='red'>
          {t('status_disabled')}
        </Label>
      );
    case 3:
      return (
        <Label basic color='grey'>
          {t('status_expired')}
        </Label>
      );
    case 4:
      return (
        <Label basic color='orange'>
          {t('status_depleted')}
        </Label>
      );
    default:
      return (
        <Label basic color='black'>
          {t('status_unknown')}
        </Label>
      );
  }
}

const TokensTable = () => {
  const { t } = useTranslation();
  const [tokens, setTokens] = useState([]);
  const [loading, setLoading] = useState(true);
  const [activePage, setActivePage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [sortBy, setSortBy] = useState('');
  const [sortOrder, setSortOrder] = useState('desc');
  const [tokenOptions, setTokenOptions] = useState([]);
  const [tokenSearchLoading, setTokenSearchLoading] = useState(false);
  const [showKeys, setShowKeys] = useState({});

  // Function to mask token key for security
  const maskKey = (key) => {
    if (!key || key.length <= 8) return '***';
    return key.substring(0, 4) + '***' + key.substring(key.length - 4);
  };

  // Function to toggle key visibility
  const toggleKeyVisibility = (tokenId) => {
    setShowKeys((prev) => ({
      ...prev,
      [tokenId]: !prev[tokenId],
    }));
  };

  // Function to copy key to clipboard
  const copyTokenKey = async (key) => {
    try {
      const success = await copy(key);
      if (success) {
        showSuccess(t('common:copy_success', 'Copied to clipboard!'));
      } else {
        showError(t('common:copy_failed', 'Failed to copy to clipboard'));
      }
    } catch (error) {
      showError(t('common:copy_failed', 'Failed to copy to clipboard'));
    }
  };

  const SORT_OPTIONS = [
    { key: '', text: t('tokens.sort.default', 'Default'), value: '' },
    { key: 'id', text: t('tokens.sort.id', 'ID'), value: 'id' },
    { key: 'name', text: t('tokens.sort.name', 'Name'), value: 'name' },
    { key: 'status', text: t('tokens.sort.status', 'Status'), value: 'status' },
    {
      key: 'used_quota',
      text: t('tokens.sort.used_quota', 'Used Quota'),
      value: 'used_quota',
    },
    {
      key: 'remain_quota',
      text: t('tokens.sort.remain_quota', 'Remaining Quota'),
      value: 'remain_quota',
    },
    {
      key: 'created_time',
      text: t('tokens.sort.created_time', 'Created Time'),
      value: 'created_time',
    },
  ];

  const loadTokens = async (page = 0, sortBy = '', sortOrder = 'desc') => {
    setLoading(true);
    let url = `/api/token/?p=${page}`;
    if (sortBy) {
      url += `&sort=${sortBy}&order=${sortOrder}`;
    }
    const res = await API.get(url);
    const { success, message, data, total } = res.data;
    if (success) {
      setTokens(data);
      setTotalPages(Math.ceil(total / ITEMS_PER_PAGE));
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const onPaginationChange = (e, { activePage }) => {
    setActivePage(activePage);
    loadTokens(activePage - 1, sortBy, sortOrder);
  };

  useEffect(() => {
    loadTokens(0, sortBy, sortOrder)
      .then()
      .catch((reason) => {
        showError(reason);
      });
  }, [sortBy, sortOrder]); // eslint-disable-line react-hooks/exhaustive-deps

  const manageToken = async (id, action, idx) => {
    let data = { id };
    let res;
    switch (action) {
      case 'delete':
        res = await API.delete(`/api/token/${id}`);
        break;
      case 'enable':
        data.status = 1;
        res = await API.put('/api/token/?status_only=true', data);
        break;
      case 'disable':
        data.status = 2;
        res = await API.put('/api/token/?status_only=true', data);
        break;
      default:
        return;
    }
    const { success, message } = res.data;
    if (success) {
      showSuccess(t('token.messages.operation_success'));
      let token = res.data.data;
      let newTokens = [...tokens];
      if (action === 'delete') {
        newTokens[idx].deleted = true;
      } else {
        newTokens[idx].status = token.status;
      }
      setTokens(newTokens);
    } else {
      showError(message);
    }
  };

  const searchTokensByName = async (searchQuery) => {
    if (!searchQuery.trim()) {
      setTokenOptions([]);
      return;
    }

    setTokenSearchLoading(true);
    try {
      const res = await API.get(`/api/token/search?keyword=${searchQuery}`);
      const { success, data } = res.data;
      if (success) {
        const options = data.map((token) => ({
          key: token.id,
          value: token.name,
          text: `${token.name}`,
          content: (
            <div style={{ display: 'flex', flexDirection: 'column' }}>
              <div style={{ fontWeight: 'bold' }}>{token.name}</div>
              <div style={{ fontSize: '0.9em', color: '#666' }}>
                ID: {token.id} • Status:{' '}
                {token.status === 1 ? 'Enabled' : 'Disabled'}
              </div>
            </div>
          ),
        }));
        setTokenOptions(options);
      }
    } catch (error) {
      console.error('Failed to search tokens:', error);
    } finally {
      setTokenSearchLoading(false);
    }
  };

  const searchTokens = async () => {
    if (searchKeyword === '') {
      await loadTokens(0, sortBy, sortOrder);
      setActivePage(1);
      return;
    }
    let url = `/api/token/search?keyword=${searchKeyword}`;
    if (sortBy) {
      url += `&sort=${sortBy}&order=${sortOrder}`;
    }
    const res = await API.get(url);
    const { success, message, data } = res.data;
    if (success) {
      setTokens(data);
      setActivePage(1);
    } else {
      showError(message);
    }
  };

  const sortToken = async (key) => {
    const newSortOrder =
      sortBy === key && sortOrder === 'desc' ? 'asc' : 'desc';
    setSortBy(key);
    setSortOrder(newSortOrder);
    await loadTokens(activePage - 1, key, newSortOrder);
  };

  const getSortIcon = (columnKey) => {
    if (sortBy !== columnKey) {
      return <Icon name='sort' style={{ opacity: 0.5 }} />;
    }
    return <Icon name={sortOrder === 'asc' ? 'sort up' : 'sort down'} />;
  };

  const handleSortChange = async (e, { value }) => {
    setSortBy(value);
    setSortOrder('desc');
    setActivePage(1);
    await loadTokens(0, value, 'desc');
  };

  const refresh = async () => {
    setLoading(true);
    await loadTokens(0, sortBy, sortOrder);
    setActivePage(1);
  };

  const headerCells = [
    {
      content: (
        <>
          {t('id')} {getSortIcon('id')}
        </>
      ),
      sortable: true,
      onClick: () => sortToken('id'),
    },
    {
      content: (
        <>
          {t('name')} {getSortIcon('name')}
        </>
      ),
      sortable: true,
      onClick: () => sortToken('name'),
    },
    {
      content: t('key'),
      sortable: false,
    },
    {
      content: (
        <>
          {t('status')} {getSortIcon('status')}
        </>
      ),
      sortable: true,
      onClick: () => sortToken('status'),
    },
    {
      content: (
        <>
          {t('used_quota')} {getSortIcon('used_quota')}
        </>
      ),
      sortable: true,
      onClick: () => sortToken('used_quota'),
    },
    {
      content: (
        <>
          {t('remain_quota')} {getSortIcon('remain_quota')}
        </>
      ),
      sortable: true,
      onClick: () => sortToken('remain_quota'),
    },
    {
      content: (
        <>
          {t('created_time')} {getSortIcon('created_time')}
        </>
      ),
      sortable: true,
      onClick: () => sortToken('created_time'),
    },
    {
      content: t('actions'),
      sortable: false,
    },
  ];

  const footerButtons = [
    {
      content: t('add'),
      as: Link,
      to: '/token/add',
      loading: loading,
    },
    {
      content: t('refresh'),
      onClick: refresh,
      loading: loading,
    },
  ];

  return (
    <>
      <Form onSubmit={searchTokens}>
        <Form.Group>
          <Form.Field width={12}>
            <Dropdown
              fluid
              selection
              search
              clearable
              allowAdditions
              placeholder={t(
                'tokens.search.placeholder',
                'Search by token name...',
              )}
              value={searchKeyword}
              options={tokenOptions}
              onSearchChange={(_, { searchQuery }) =>
                searchTokensByName(searchQuery)
              }
              onChange={(_, { value }) => setSearchKeyword(value)}
              loading={tokenSearchLoading}
              noResultsMessage={t('tokens.no_tokens_found', 'No tokens found')}
              additionLabel={t('tokens.use_token_name', 'Use token name: ')}
              onAddItem={(_, { value }) => {
                const newOption = {
                  key: value,
                  value: value,
                  text: value,
                };
                setTokenOptions([...tokenOptions, newOption]);
              }}
            />
          </Form.Field>
          <Form.Dropdown
            width={4}
            selection
            placeholder={t('tokens.sort.placeholder', 'Sort by...')}
            options={SORT_OPTIONS}
            value={sortBy}
            onChange={handleSortChange}
          />
        </Form.Group>
      </Form>

      <BaseTable
        loading={loading}
        activePage={activePage}
        totalPages={totalPages}
        onPageChange={onPaginationChange}
        headerCells={headerCells}
        footerButtons={footerButtons}
        colSpan={8}
      >
        {tokens.map((token, idx) => {
          if (token.deleted) return null;
          return (
            <Table.Row key={token.id}>
              <Table.Cell data-label='ID'>{token.id}</Table.Cell>
              <Table.Cell data-label='Name'>
                {cleanDisplay(token.name)}
              </Table.Cell>
              <Table.Cell data-label='Key'>
                <div
                  style={{ display: 'flex', alignItems: 'center', gap: '8px' }}
                >
                  <span style={{ fontFamily: 'monospace', fontSize: '0.9em' }}>
                    {showKeys[token.id] ? token.key : maskKey(token.key)}
                  </span>
                  <Popup
                    trigger={
                      <Button
                        size='mini'
                        icon
                        onClick={() => toggleKeyVisibility(token.id)}
                        aria-label={
                          showKeys[token.id]
                            ? t('common:hide', 'Hide')
                            : t('common:show', 'Show')
                        }
                      >
                        <Icon name={showKeys[token.id] ? 'eye slash' : 'eye'} />
                      </Button>
                    }
                    content={
                      showKeys[token.id]
                        ? t('common:hide', 'Hide')
                        : t('common:show', 'Show')
                    }
                    basic
                    inverted
                  />
                  <Popup
                    trigger={
                      <Button
                        size='mini'
                        icon
                        onClick={() => copyTokenKey(token.key)}
                        aria-label={t('common:copy', 'Copy')}
                      >
                        <Icon name='copy' />
                      </Button>
                    }
                    content={t('common:copy', 'Copy')}
                    basic
                    inverted
                  />
                </div>
              </Table.Cell>
              <Table.Cell data-label='Status'>
                {renderTokenStatus(token.status, t)}
              </Table.Cell>
              <Table.Cell data-label='Used Quota'>
                {token.used_quota ? renderQuota(token.used_quota) : '$0.00'}
              </Table.Cell>
              <Table.Cell data-label='Remaining Quota'>
                {token.remain_quota === 0 ||
                token.remain_quota === null ||
                token.remain_quota === undefined
                  ? token.unlimited_quota
                    ? t('common:unlimited')
                    : '$0.00'
                  : renderQuota(token.remain_quota)}
              </Table.Cell>
              <Table.Cell data-label='Created Time'>
                {renderTimestamp(token.created_time)}
              </Table.Cell>
              <Table.Cell data-label='Actions'>
                <div>
                  <Popup
                    trigger={
                      <Button
                        size='small'
                        positive={token.status === 1}
                        negative={token.status !== 1}
                        onClick={() => {
                          manageToken(
                            token.id,
                            token.status === 1 ? 'disable' : 'enable',
                            idx,
                          );
                        }}
                        aria-label={
                          token.status === 1
                            ? t('common:disable', 'Disable')
                            : t('common:enable', 'Enable')
                        }
                      >
                        {token.status === 1 ? (
                          <Icon name='pause' />
                        ) : (
                          <Icon name='play' />
                        )}
                      </Button>
                    }
                    content={
                      token.status === 1
                        ? t('common:disable', 'Disable')
                        : t('common:enable', 'Enable')
                    }
                    basic
                    inverted
                  />
                  <Popup
                    trigger={
                      <Button
                        size='small'
                        color='blue'
                        as={Link}
                        to={'/token/edit/' + token.id}
                        aria-label={t('common:edit', 'Edit')}
                      >
                        <Icon name='edit' />
                      </Button>
                    }
                    content={t('common:edit', 'Edit')}
                    basic
                    inverted
                  />
                  <Popup
                    trigger={
                      <Button
                        size='small'
                        negative
                        aria-label={t('common:delete', 'Delete')}
                      >
                        <Icon name='trash' />
                      </Button>
                    }
                    on='click'
                    flowing
                    hoverable
                  >
                    <Button
                      negative
                      onClick={() => {
                        manageToken(token.id, 'delete', idx);
                      }}
                    >
                      {t('token.confirm_delete', 'Delete Token')}
                    </Button>
                  </Popup>
                </div>
              </Table.Cell>
            </Table.Row>
          );
        })}
      </BaseTable>
    </>
  );
};

export default TokensTable;
