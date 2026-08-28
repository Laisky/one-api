import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { API, isMobile, showError, showSuccess } from '../../helpers';
import { renderQuotaWithPrompt } from '../../helpers/render';
import Title from '@douyinfe/semi-ui/lib/es/typography/title';
import { Button, Divider, Input, Select, SideSheet, Space, Spin, Typography } from '@douyinfe/semi-ui';

const EditUser = (props) => {
  const userId = props.editingUser.uuid || props.editingUser.id;
  const [loading, setLoading] = useState(true);
  const [inputs, setInputs] = useState({
    username: '',
    display_name: '',
    password: '',
    github_id: '',
    wechat_id: '',
    email: '',
    quota: 0,
    group: 'default'
  });
  const [groupOptions, setGroupOptions] = useState([]);

  // TOTP related state
  const [totpEnabled, setTotpEnabled] = useState(false);
  const [totpLoading, setTotpLoading] = useState(false);

  const { username, display_name, password, github_id, wechat_id, telegram_id, email, quota, group } =
    inputs;
  const handleInputChange = (name, value) => {
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  };
  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      setGroupOptions(res.data.data.map((group) => ({
        label: group,
        value: group
      })));
    } catch (error) {
      showError(error.message);
    }
  };
  const navigate = useNavigate();
  const handleCancel = () => {
    props.handleClose();
  };
  const loadUser = async () => {
    setLoading(true);
    let res = undefined;
    if (userId) {
      res = await API.get(`/api/user/${userId}`);
    } else {
      res = await API.get(`/api/user/self`);
    }
    const { success, message, data } = res.data;
    if (success) {
      data.password = '';
      setInputs(data);
      // The user DTO never exposes the TOTP secret; ask the dedicated
      // admin status endpoint instead.
      if (userId) {
        await loadTotpStatus();
      }
    } else {
      showError(message);
    }
    setLoading(false);
  };

  // loadTotpStatus fetches whether the edited user has TOTP enabled via the
  // admin-only status endpoint and updates the local flag. Failures are
  // reported but leave the flag untouched.
  const loadTotpStatus = async () => {
    try {
      const res = await API.get(`/api/user/totp/status/${userId}`);
      const { success, message, data } = res.data;
      if (success) {
        setTotpEnabled(Boolean(data?.totp_enabled));
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
  };

  const adminDisableTotp = async () => {
    setTotpLoading(true);
    try {
      const res = await API.post(`/api/user/totp/disable/${userId}`);
      if (res.data.success) {
        showSuccess('TOTP has been successfully disabled for the user');
        setTotpEnabled(false);
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError('Failed to disable TOTP');
    }
    setTotpLoading(false);
  };

  useEffect(() => {
    loadUser().then();
    if (userId) {
      fetchGroups().then();
    }
  }, [props.editingUser.uuid, props.editingUser.id]);

  const submit = async () => {
    setLoading(true);
    let res = undefined;
    if (userId) {
      let data = { ...inputs, uuid: userId };
      if (typeof data.quota === 'string') {
        data.quota = parseInt(data.quota);
      }
      res = await API.put(`/api/user/`, data);
    } else {
      res = await API.put(`/api/user/self`, inputs);
    }
    const { success, message } = res.data;
    if (success) {
      showSuccess('用户信息更新成功！');
      props.refresh();
      props.handleClose();
    } else {
      showError(message);
    }
    setLoading(false);
  };

  return (
    <>
      <SideSheet
        placement={'right'}
        title={<Title level={3}>{'编辑用户'}</Title>}
        headerStyle={{ borderBottom: '1px solid var(--semi-color-border)' }}
        bodyStyle={{ borderBottom: '1px solid var(--semi-color-border)' }}
        visible={props.visible}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <Space>
              <Button theme="solid" size={'large'} onClick={submit}>提交</Button>
              <Button theme="solid" size={'large'} type={'tertiary'} onClick={handleCancel}>取消</Button>
            </Space>
          </div>
        }
        closeIcon={null}
        onCancel={() => handleCancel()}
        width={isMobile() ? '100%' : 600}
      >
        {loading ? (
          <div style={{
            minHeight: '400px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            backgroundColor: 'var(--semi-color-bg-2)',
            borderRadius: '8px',
            margin: '1rem 0'
          }}>
            <div style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              gap: '1rem',
              color: 'var(--semi-color-text-1)'
            }}>
              <Spin size="large" />
              <span>正在加载用户信息...</span>
            </div>
          </div>
        ) : (
          <>
            <div style={{ marginTop: 20 }}>
              <Typography.Text>用户名</Typography.Text>
            </div>
          <Input
            label="用户名"
            name="username"
            placeholder={'请输入新的用户名'}
            onChange={value => handleInputChange('username', value)}
            value={username}
            autoComplete="new-password"
          />
          <div style={{ marginTop: 20 }}>
            <Typography.Text>密码</Typography.Text>
          </div>
          <Input
            label="密码"
            name="password"
            type={'password'}
            placeholder={'请输入新的密码，最短 8 位'}
            onChange={value => handleInputChange('password', value)}
            value={password}
            autoComplete="new-password"
          />
          <div style={{ marginTop: 20 }}>
            <Typography.Text>显示名称</Typography.Text>
          </div>
          <Input
            label="显示名称"
            name="display_name"
            placeholder={'请输入新的显示名称'}
            onChange={value => handleInputChange('display_name', value)}
            value={display_name}
            autoComplete="new-password"
          />
          {
            userId && <>
              <div style={{ marginTop: 20 }}>
                <Typography.Text>分组</Typography.Text>
              </div>
              <Select
                placeholder={'请选择分组'}
                name="group"
                fluid
                search
                selection
                allowAdditions
                additionLabel={'请在系统设置页面编辑分组倍率以添加新的分组：'}
                onChange={value => handleInputChange('group', value)}
                value={inputs.group}
                autoComplete="new-password"
                optionList={groupOptions}
              />
              <div style={{ marginTop: 20 }}>
                <Typography.Text>{`剩余额度${renderQuotaWithPrompt(quota)}`}</Typography.Text>
              </div>
              <Input
                name="quota"
                placeholder={'请输入新的剩余额度'}
                onChange={value => handleInputChange('quota', value)}
                value={quota}
                type={'number'}
                autoComplete="new-password"
              />
            </>
          }
          <Divider style={{ marginTop: 20 }}>以下信息不可修改</Divider>
          <div style={{ marginTop: 20 }}>
            <Typography.Text>已绑定的 GitHub 账户</Typography.Text>
          </div>
          <Input
            name="github_id"
            value={github_id}
            autoComplete="new-password"
            placeholder="此项只读，需要用户通过个人设置页面的相关绑定按钮进行绑定，不可直接修改"
            readonly
          />
          <div style={{ marginTop: 20 }}>
            <Typography.Text>已绑定的微信账户</Typography.Text>
          </div>
          <Input
            name="wechat_id"
            value={wechat_id}
            autoComplete="new-password"
            placeholder="此项只读，需要用户通过个人设置页面的相关绑定按钮进行绑定，不可直接修改"
            readonly
          />
          <Input
            name="telegram_id"
            value={telegram_id}
            autoComplete="new-password"
            placeholder="此项只读，需要用户通过个人设置页面的相关绑定按钮进行绑定，不可直接修改"
            readonly
          />
          <div style={{ marginTop: 20 }}>
            <Typography.Text>已绑定的邮箱账户</Typography.Text>
          </div>
          <Input
            name="email"
            value={email}
            autoComplete="new-password"
            placeholder="此项只读，需要用户通过个人设置页面的相关绑定按钮进行绑定，不可直接修改"
            readonly
          />

          {/* Admin TOTP Section - Show when admin is editing other users */}
          {userId && (
            <>
              <Divider style={{ marginTop: 20 }}>双因子认证 (TOTP) - 管理员控制</Divider>
              {totpEnabled ? (
                <div style={{ marginTop: 20, padding: 16, backgroundColor: '#fff3cd', border: '1px solid #ffeaa7', borderRadius: 4 }}>
                  <Typography.Text strong style={{ color: '#856404' }}>此用户已启用 TOTP</Typography.Text>
                  <div style={{ marginTop: 8 }}>
                    <Typography.Text style={{ color: '#856404' }}>作为管理员，如果用户被锁定，您可以为其禁用 TOTP。</Typography.Text>
                  </div>
                  <div style={{ marginTop: 12 }}>
                    <Button
                      theme="solid"
                      type="danger"
                      onClick={adminDisableTotp}
                      loading={totpLoading}
                    >
                      管理员禁用 TOTP
                    </Button>
                  </div>
                </div>
              ) : (
                <div style={{ marginTop: 20, padding: 16, backgroundColor: '#d1ecf1', border: '1px solid #bee5eb', borderRadius: 4 }}>
                  <Typography.Text strong style={{ color: '#0c5460' }}>此用户未启用 TOTP</Typography.Text>
                  <div style={{ marginTop: 8 }}>
                    <Typography.Text style={{ color: '#0c5460' }}>此用户尚未启用双因子认证。</Typography.Text>
                  </div>
                </div>
              )}
            </>
          )}
          </>
        )}
      </SideSheet>
    </>
  );
};

export default EditUser;
