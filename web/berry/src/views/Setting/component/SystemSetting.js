import { useState, useEffect } from 'react';
import SubCard from 'ui-component/cards/SubCard';
import {
  Stack,
  FormControl,
  InputLabel,
  OutlinedInput,
  Checkbox,
  Button,
  FormControlLabel,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Divider,
  Alert,
  Autocomplete,
  TextField
} from '@mui/material';
import Grid from '@mui/material/Unstable_Grid2';
import { showError, showSuccess, removeTrailingSlash } from 'utils/common'; //,
import { API } from 'utils/api';
import { createFilterOptions } from '@mui/material/Autocomplete';

const filter = createFilterOptions();
const SystemSetting = () => {
  let [inputs, setInputs] = useState({
    PasswordLoginEnabled: '',
    PasswordRegisterEnabled: '',
    EmailVerificationEnabled: '',
    GitHubOAuthEnabled: '',
    GitHubClientId: '',
    GitHubClientSecret: '',
    LarkClientId: '',
    LarkClientSecret: '',
    OidcEnabled: '',
    OidcWellKnown: '',
    OidcClientId: '',
    OidcClientSecret: '',
    OidcAuthorizationEndpoint: '',
    OidcTokenEndpoint: '',
    OidcUserinfoEndpoint: '',
    Notice: '',
    SMTPServer: '',
    SMTPPort: '',
    SMTPAccount: '',
    SMTPFrom: '',
    SMTPToken: '',
    ServerAddress: '',
    Footer: '',
    WeChatAuthEnabled: '',
    WeChatServerAddress: '',
    WeChatServerToken: '',
    WeChatAccountQRCodeImageURL: '',
    TurnstileCheckEnabled: '',
    TurnstileSiteKey: '',
    TurnstileSecretKey: '',
    RegisterEnabled: '',
    EmailDomainRestrictionEnabled: '',
    EmailDomainWhitelist: [],
    MessagePusherAddress: '',
    MessagePusherToken: ''
  });
  const [originInputs, setOriginInputs] = useState({});
  let [loading, setLoading] = useState(false);
  const [EmailDomainWhitelist, setEmailDomainWhitelist] = useState([]);
  const [showPasswordWarningModal, setShowPasswordWarningModal] = useState(false);

  const getOptions = async () => {
    const res = await API.get('/api/option/');
    const { success, message, data } = res.data;
    if (success) {
      let newInputs = {};
      data.forEach((item) => {
        newInputs[item.key] = item.value;
      });
      setInputs({
        ...newInputs,
        EmailDomainWhitelist: newInputs.EmailDomainWhitelist.split(',')
      });
      setOriginInputs(newInputs);

      setEmailDomainWhitelist(newInputs.EmailDomainWhitelist.split(','));
    } else {
      showError(message);
    }
  };

  useEffect(() => {
    getOptions().then();
  }, []);

  const updateOption = async (key, value) => {
    setLoading(true);
    switch (key) {
      case 'PasswordLoginEnabled':
      case 'PasswordRegisterEnabled':
      case 'EmailVerificationEnabled':
      case 'GitHubOAuthEnabled':
      case 'WeChatAuthEnabled':
      case 'TurnstileCheckEnabled':
      case 'EmailDomainRestrictionEnabled':
      case 'RegisterEnabled':
      case 'OidcEnabled':
        value = inputs[key] === 'true' ? 'false' : 'true';
        break;
      default:
        break;
    }
    const res = await API.put('/api/option/', {
      key,
      value
    });
    const { success, message } = res.data;
    if (success) {
      if (key === 'EmailDomainWhitelist') {
        value = value.split(',');
      }
      setInputs((inputs) => ({
        ...inputs,
        [key]: value
      }));
      showSuccess('设置成功！');
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const handleInputChange = async (event) => {
    let { name, value } = event.target;

    if (name === 'PasswordLoginEnabled' && inputs[name] === 'true') {
      // block disabling password login
      setShowPasswordWarningModal(true);
      return;
    }
    if (
      name === 'Notice' ||
      name.startsWith('SMTP') ||
      name === 'ServerAddress' ||
      name === 'GitHubClientId' ||
      name === 'GitHubClientSecret' ||
      name === 'WeChatServerAddress' ||
      name === 'WeChatServerToken' ||
      name === 'WeChatAccountQRCodeImageURL' ||
      name === 'TurnstileSiteKey' ||
      name === 'TurnstileSecretKey' ||
      name === 'EmailDomainWhitelist' ||
      name === 'MessagePusherAddress' ||
      name === 'MessagePusherToken' ||
      name === 'LarkClientId' ||
      name === 'LarkClientSecret' ||
      name === 'OidcClientId' ||
      name === 'OidcClientSecret' ||
      name === 'OidcWellKnown' ||
      name === 'OidcAuthorizationEndpoint' ||
      name === 'OidcTokenEndpoint' ||
      name === 'OidcUserinfoEndpoint'
    )
    {
      setInputs((inputs) => ({ ...inputs, [name]: value }));
    } else {
      await updateOption(name, value);
    }
  };

  const submitServerAddress = async () => {
    let ServerAddress = removeTrailingSlash(inputs.ServerAddress);
    await updateOption('ServerAddress', ServerAddress);
  };

  const submitSMTP = async () => {
    if (originInputs['SMTPServer'] !== inputs.SMTPServer) {
      await updateOption('SMTPServer', inputs.SMTPServer);
    }
    if (originInputs['SMTPAccount'] !== inputs.SMTPAccount) {
      await updateOption('SMTPAccount', inputs.SMTPAccount);
    }
    if (originInputs['SMTPFrom'] !== inputs.SMTPFrom) {
      await updateOption('SMTPFrom', inputs.SMTPFrom);
    }
    if (originInputs['SMTPPort'] !== inputs.SMTPPort && inputs.SMTPPort !== '') {
      await updateOption('SMTPPort', inputs.SMTPPort);
    }
    if (originInputs['SMTPToken'] !== inputs.SMTPToken && inputs.SMTPToken !== '') {
      await updateOption('SMTPToken', inputs.SMTPToken);
    }
  };

  const submitEmailDomainWhitelist = async () => {
    await updateOption('EmailDomainWhitelist', inputs.EmailDomainWhitelist.join(','));
  };

  const submitWeChat = async () => {
    if (originInputs['WeChatServerAddress'] !== inputs.WeChatServerAddress) {
      await updateOption('WeChatServerAddress', removeTrailingSlash(inputs.WeChatServerAddress));
    }
    if (originInputs['WeChatAccountQRCodeImageURL'] !== inputs.WeChatAccountQRCodeImageURL) {
      await updateOption('WeChatAccountQRCodeImageURL', inputs.WeChatAccountQRCodeImageURL);
    }
    if (originInputs['WeChatServerToken'] !== inputs.WeChatServerToken && inputs.WeChatServerToken !== '') {
      await updateOption('WeChatServerToken', inputs.WeChatServerToken);
    }
  };

  const submitGitHubOAuth = async () => {
    if (originInputs['GitHubClientId'] !== inputs.GitHubClientId) {
      await updateOption('GitHubClientId', inputs.GitHubClientId);
    }
    if (originInputs['GitHubClientSecret'] !== inputs.GitHubClientSecret && inputs.GitHubClientSecret !== '') {
      await updateOption('GitHubClientSecret', inputs.GitHubClientSecret);
    }
  };

  const submitTurnstile = async () => {
    if (originInputs['TurnstileSiteKey'] !== inputs.TurnstileSiteKey) {
      await updateOption('TurnstileSiteKey', inputs.TurnstileSiteKey);
    }
    if (originInputs['TurnstileSecretKey'] !== inputs.TurnstileSecretKey && inputs.TurnstileSecretKey !== '') {
      await updateOption('TurnstileSecretKey', inputs.TurnstileSecretKey);
    }
  };

  const submitMessagePusher = async () => {
    if (originInputs['MessagePusherAddress'] !== inputs.MessagePusherAddress) {
      await updateOption('MessagePusherAddress', removeTrailingSlash(inputs.MessagePusherAddress));
    }
    if (originInputs['MessagePusherToken'] !== inputs.MessagePusherToken && inputs.MessagePusherToken !== '') {
      await updateOption('MessagePusherToken', inputs.MessagePusherToken);
    }
  };

  const submitLarkOAuth = async () => {
    if (originInputs['LarkClientId'] !== inputs.LarkClientId) {
      await updateOption('LarkClientId', inputs.LarkClientId);
    }
    if (originInputs['LarkClientSecret'] !== inputs.LarkClientSecret && inputs.LarkClientSecret !== '') {
      await updateOption('LarkClientSecret', inputs.LarkClientSecret);
    }
  };

  const submitOidc = async () => {
    if (inputs.OidcWellKnown !== '') {
      if (!inputs.OidcWellKnown.startsWith('http://') && !inputs.OidcWellKnown.startsWith('https://')) {
        showError('Well-Known URL 必须以 http:// 或 https:// 开头');
        return;
      }
      try {
        const res = await API.get(inputs.OidcWellKnown);
        inputs.OidcAuthorizationEndpoint = res.data['authorization_endpoint'];
        inputs.OidcTokenEndpoint = res.data['token_endpoint'];
        inputs.OidcUserinfoEndpoint = res.data['userinfo_endpoint'];
        showSuccess('获取 OIDC 配置成功！');
      } catch (err) {
        showError("获取 OIDC 配置失败，请检查网络状况和 Well-Known URL 是否正确");
      }
    }

    if (originInputs['OidcWellKnown'] !== inputs.OidcWellKnown) {
      await updateOption('OidcWellKnown', inputs.OidcWellKnown);
    }
    if (originInputs['OidcClientId'] !== inputs.OidcClientId) {
      await updateOption('OidcClientId', inputs.OidcClientId);
    }
    if (originInputs['OidcClientSecret'] !== inputs.OidcClientSecret && inputs.OidcClientSecret !== '') {
      await updateOption('OidcClientSecret', inputs.OidcClientSecret);
    }
    if (originInputs['OidcAuthorizationEndpoint'] !== inputs.OidcAuthorizationEndpoint) {
      await updateOption('OidcAuthorizationEndpoint', inputs.OidcAuthorizationEndpoint);
    }
    if (originInputs['OidcTokenEndpoint'] !== inputs.OidcTokenEndpoint) {
      await updateOption('OidcTokenEndpoint', inputs.OidcTokenEndpoint);
    }
    if (originInputs['OidcUserinfoEndpoint'] !== inputs.OidcUserinfoEndpoint) {
      await updateOption('OidcUserinfoEndpoint', inputs.OidcUserinfoEndpoint);
    }
  };

  return (
    <>
      <Stack spacing={2}>
        <SubCard title="通用设置">
          <Grid container spacing={{ xs: 3, sm: 2, md: 4 }}>
            <Grid xs={12}>
              <FormControl fullWidth>
                <InputLabel htmlFor="ServerAddress">服务器地址</InputLabel>
                <OutlinedInput
                  id="ServerAddress"
                  name="ServerAddress"
                  value={inputs.ServerAddress || ''}
                  onChange={handleInputChange}
                  label="服务器地址"
                  placeholder="例如：https://yourdomain.com"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12}>
              <Button variant="contained" onClick={submitServerAddress}>
                更新服务器地址
              </Button>
            </Grid>
          </Grid>
        </SubCard>
        <SubCard title="配置登录注册">
          <Grid container spacing={{ xs: 3, sm: 2, md: 4 }}>
            <Grid xs={12} md={3}>
              <FormControlLabel
                label="允许通过密码进行登录"
                control={
                  <Checkbox checked={inputs.PasswordLoginEnabled === 'true'} onChange={handleInputChange} name="PasswordLoginEnabled" />
                }
              />
            </Grid>
            <Grid xs={12} md={3}>
              <FormControlLabel
                label="允许通过密码进行注册"
                control={
                  <Checkbox
                    checked={inputs.PasswordRegisterEnabled === 'true'}
                    onChange={handleInputChange}
                    name="PasswordRegisterEnabled"
                  />
                }
              />
            </Grid>
            <Grid xs={12} md={3}>
              <FormControlLabel
                label="通过密码注册时需要进行邮箱验证"
                control={
                  <Checkbox
                    checked={inputs.EmailVerificationEnabled === 'true'}
                    onChange={handleInputChange}
                    name="EmailVerificationEnabled"
                  />
                }
              />
            </Grid>
            <Grid xs={12} md={3}>
              <FormControlLabel
                label="允许通过 GitHub 账户登录 & 注册"
                control={<Checkbox checked={inputs.GitHubOAuthEnabled === 'true'} onChange={handleInputChange} name="GitHubOAuthEnabled" />}
              />
            </Grid>
            <Grid xs={12} md={3}>
              <FormControlLabel
                label="允许通过 OIDC 登录 & 注册"
                control={<Checkbox checked={inputs.OidcEnabled === 'true'} onChange={handleInputChange} name="OidcEnabled" />}
              />
            </Grid>
            <Grid xs={12} md={3}>
              <FormControlLabel
                label="允许通过微信登录 & 注册"
                control={<Checkbox checked={inputs.WeChatAuthEnabled === 'true'} onChange={handleInputChange} name="WeChatAuthEnabled" />}
              />
            </Grid>
            <Grid xs={12} md={3}>
              <FormControlLabel
                label="允许新用户注册（此项为否时，新用户将无法以任何方式进行注册）"
                control={<Checkbox checked={inputs.RegisterEnabled === 'true'} onChange={handleInputChange} name="RegisterEnabled" />}
              />
            </Grid>
            <Grid xs={12} md={3}>
              <FormControlLabel
                label="启用 Turnstile 用户校验"
                control={
                  <Checkbox checked={inputs.TurnstileCheckEnabled === 'true'} onChange={handleInputChange} name="TurnstileCheckEnabled" />
                }
              />
            </Grid>
          </Grid>
        </SubCard>
        <SubCard title="配置邮箱域名白名单" subTitle="用以防止恶意用户利用临时邮箱批量注册">
          <Grid container spacing={{ xs: 3, sm: 2, md: 4 }}>
            <Grid xs={12}>
              <FormControlLabel
                label="启用邮箱域名白名单"
                control={
                  <Checkbox
                    checked={inputs.EmailDomainRestrictionEnabled === 'true'}
                    onChange={handleInputChange}
                    name="EmailDomainRestrictionEnabled"
                  />
                }
              />
            </Grid>
            <Grid xs={12}>
              <FormControl fullWidth>
                <Autocomplete
                  multiple
                  freeSolo
                  id="EmailDomainWhitelist"
                  options={EmailDomainWhitelist}
                  value={inputs.EmailDomainWhitelist}
                  onChange={(e, value) => {
                    const event = {
                      target: {
                        name: 'EmailDomainWhitelist',
                        value: value
                      }
                    };
                    handleInputChange(event);
                  }}
                  filterSelectedOptions
                  renderInput={(params) => <TextField {...params} name="EmailDomainWhitelist" label="允许的邮箱域名" />}
                  filterOptions={(options, params) => {
                    const filtered = filter(options, params);
                    const { inputValue } = params;
                    const isExisting = options.some((option) => inputValue === option);
                    if (inputValue !== '' && !isExisting) {
                      filtered.push(inputValue);
                    }
                    return filtered;
                  }}
                />
              </FormControl>
            </Grid>
            <Grid xs={12}>
              <Button variant="contained" onClick={submitEmailDomainWhitelist}>
                保存邮箱域名白名单设置
              </Button>
            </Grid>
          </Grid>
        </SubCard>
        <SubCard title="配置 SMTP" subTitle="用以支持系统的邮件发送">
          <Grid container spacing={{ xs: 3, sm: 2, md: 4 }}>
            <Grid xs={12} md={4}>
              <FormControl fullWidth>
                <InputLabel htmlFor="SMTPServer">SMTP 服务器地址</InputLabel>
                <OutlinedInput
                  id="SMTPServer"
                  name="SMTPServer"
                  value={inputs.SMTPServer || ''}
                  onChange={handleInputChange}
                  label="SMTP 服务器地址"
                  placeholder="例如：smtp.qq.com"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12} md={4}>
              <FormControl fullWidth>
                <InputLabel htmlFor="SMTPPort">SMTP 端口</InputLabel>
                <OutlinedInput
                  id="SMTPPort"
                  name="SMTPPort"
                  value={inputs.SMTPPort || ''}
                  onChange={handleInputChange}
                  label="SMTP 端口"
                  placeholder="默认: 587"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12} md={4}>
              <FormControl fullWidth>
                <InputLabel htmlFor="SMTPAccount">SMTP 账户</InputLabel>
                <OutlinedInput
                  id="SMTPAccount"
                  name="SMTPAccount"
                  value={inputs.SMTPAccount || ''}
                  onChange={handleInputChange}
                  label="SMTP 账户"
                  placeholder="通常是邮箱地址"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12} md={4}>
              <FormControl fullWidth>
                <InputLabel htmlFor="SMTPFrom">SMTP 发送者邮箱</InputLabel>
                <OutlinedInput
                  id="SMTPFrom"
                  name="SMTPFrom"
                  value={inputs.SMTPFrom || ''}
                  onChange={handleInputChange}
                  label="SMTP 发送者邮箱"
                  placeholder="通常和邮箱地址保持一致"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12} md={4}>
              <FormControl fullWidth>
                <InputLabel htmlFor="SMTPToken">SMTP 访问凭证</InputLabel>
                <OutlinedInput
                  id="SMTPToken"
                  name="SMTPToken"
                  value={inputs.SMTPToken || ''}
                  onChange={handleInputChange}
                  label="SMTP 访问凭证"
                  placeholder="敏感信息不会发送到前端显示"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12}>
              <Button variant="contained" onClick={submitSMTP}>
                保存 SMTP 设置
              </Button>
            </Grid>
          </Grid>
        </SubCard>
        <SubCard
          title="配置 GitHub OAuth App"
          subTitle={
            <span>
              {' '}
              用以支持通过 GitHub 进行登录注册，
              <a href="https://github.com/settings/developers" target="_blank" rel="noopener noreferrer">
                点击此处
              </a>
              管理你的 GitHub OAuth App
            </span>
          }
        >
          <Grid container spacing={{ xs: 3, sm: 2, md: 4 }}>
            <Grid xs={12}>
              <Alert severity="info" sx={{ wordWrap: 'break-word' }}>
                Homepage URL 填 <b>{inputs.ServerAddress}</b>
                ，Authorization callback URL 填 <b>{`${inputs.ServerAddress}/oauth/github`}</b>
              </Alert>
            </Grid>
            <Grid xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel htmlFor="GitHubClientId">GitHub Client ID</InputLabel>
                <OutlinedInput
                  id="GitHubClientId"
                  name="GitHubClientId"
                  value={inputs.GitHubClientId || ''}
                  onChange={handleInputChange}
                  label="GitHub Client ID"
                  placeholder="输入你注册的 GitHub OAuth APP 的 ID"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel htmlFor="GitHubClientSecret">GitHub Client Secret</InputLabel>
                <OutlinedInput
                  id="GitHubClientSecret"
                  name="GitHubClientSecret"
                  value={inputs.GitHubClientSecret || ''}
                  onChange={handleInputChange}
                  label="GitHub Client Secret"
                  placeholder="敏感信息不会发送到前端显示"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12}>
              <Button variant="contained" onClick={submitGitHubOAuth}>
                保存 GitHub OAuth 设置
              </Button>
            </Grid>
          </Grid>
        </SubCard>
        <SubCard
          title="配置飞书授权登录"
          subTitle={
            <span>
              {' '}
              用以支持通过飞书进行登录注册，
              <a href="https://open.feishu.cn/app" target="_blank" rel="noreferrer">
                点击此处
              </a>
              管理你的飞书应用
            </span>
          }
        >
          <Grid container spacing={{ xs: 3, sm: 2, md: 4 }}>
            <Grid xs={12}>
              <Alert severity="info" sx={{ wordWrap: 'break-word' }}>
                主页链接填 <code>{inputs.ServerAddress}</code>
                ，重定向 URL 填 <code>{`${inputs.ServerAddress}/oauth/lark`}</code>
              </Alert>
            </Grid>
            <Grid xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel htmlFor="LarkClientId">App ID</InputLabel>
                <OutlinedInput
                  id="LarkClientId"
                  name="LarkClientId"
                  value={inputs.LarkClientId || ''}
                  onChange={handleInputChange}
                  label="App ID"
                  placeholder="输入 App ID"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel htmlFor="LarkClientSecret">App Secret</InputLabel>
                <OutlinedInput
                  id="LarkClientSecret"
                  name="LarkClientSecret"
                  value={inputs.LarkClientSecret || ''}
                  onChange={handleInputChange}
                  label="App Secret"
                  placeholder="敏感信息不会发送到前端显示"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12}>
              <Button variant="contained" onClick={submitLarkOAuth}>
                保存飞书 OAuth 设置
              </Button>
            </Grid>
          </Grid>
        </SubCard>
        <SubCard
          title="配置 WeChat Server"
          subTitle={
            <span>
              用以支持通过微信进行登录注册，
              <a href="https://github.com/Laisky/wechat-server" target="_blank" rel="noopener noreferrer">
                点击此处
              </a>
              了解 WeChat Server
            </span>
          }
        >
          <Grid container spacing={{ xs: 3, sm: 2, md: 4 }}>
            <Grid xs={12} md={4}>
              <FormControl fullWidth>
                <InputLabel htmlFor="WeChatServerAddress">WeChat Server 服务器地址</InputLabel>
                <OutlinedInput
                  id="WeChatServerAddress"
                  name="WeChatServerAddress"
                  value={inputs.WeChatServerAddress || ''}
                  onChange={handleInputChange}
                  label="WeChat Server 服务器地址"
                  placeholder="例如：https://yourdomain.com"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12} md={4}>
              <FormControl fullWidth>
                <InputLabel htmlFor="WeChatServerToken">WeChat Server 访问凭证</InputLabel>
                <OutlinedInput
                  id="WeChatServerToken"
                  name="WeChatServerToken"
                  value={inputs.WeChatServerToken || ''}
                  onChange={handleInputChange}
                  label="WeChat Server 访问凭证"
                  placeholder="敏感信息不会发送到前端显示"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12} md={4}>
              <FormControl fullWidth>
                <InputLabel htmlFor="WeChatAccountQRCodeImageURL">微信公众号二维码图片链接</InputLabel>
                <OutlinedInput
                  id="WeChatAccountQRCodeImageURL"
                  name="WeChatAccountQRCodeImageURL"
                  value={inputs.WeChatAccountQRCodeImageURL || ''}
                  onChange={handleInputChange}
                  label="微信公众号二维码图片链接"
                  placeholder="输入一个图片链接"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12}>
              <Button variant="contained" onClick={submitWeChat}>
                保存 WeChat Server 设置
              </Button>
            </Grid>
          </Grid>
        </SubCard>

        <SubCard
          title="配置 OIDC"
          subTitle={
            <span>
              用以支持通过 OIDC 登录，例如 Okta、Auth0 等兼容 OIDC 协议的 IdP
            </span>
          }
        >
          <Grid container spacing={ { xs: 3, sm: 2, md: 4 } }>
            <Grid xs={ 12 } md={ 12 }>
              <Alert severity="info" sx={ { wordWrap: 'break-word' } }>
                主页链接填 <code>{ inputs.ServerAddress }</code>
                ，重定向 URL 填 <code>{ `${ inputs.ServerAddress }/oauth/oidc` }</code>
              </Alert> <br />
              <Alert severity="info" sx={ { wordWrap: 'break-word' } }>
                若你的 OIDC Provider 支持 Discovery Endpoint，你可以仅填写 OIDC Well-Known URL，系统会自动获取 OIDC 配置
              </Alert>
            </Grid>
            <Grid xs={ 12 } md={ 6 }>
              <FormControl fullWidth>
                <InputLabel htmlFor="OidcClientId">Client ID</InputLabel>
                <OutlinedInput
                  id="OidcClientId"
                  name="OidcClientId"
                  value={ inputs.OidcClientId || '' }
                  onChange={ handleInputChange }
                  label="Client ID"
                  placeholder="输入 OIDC 的 Client ID"
                  disabled={ loading }
                />
              </FormControl>
            </Grid>
            <Grid xs={ 12 } md={ 6 }>
              <FormControl fullWidth>
                <InputLabel htmlFor="OidcClientSecret">Client Secret</InputLabel>
                <OutlinedInput
                  id="OidcClientSecret"
                  name="OidcClientSecret"
                  value={ inputs.OidcClientSecret || '' }
                  onChange={ handleInputChange }
                  label="Client Secret"
                  placeholder="敏感信息不会发送到前端显示"
                  disabled={ loading }
                />
              </FormControl>
            </Grid>
            <Grid xs={ 12 } md={ 6 }>
              <FormControl fullWidth>
                <InputLabel htmlFor="OidcWellKnown">Well-Known URL</InputLabel>
                <OutlinedInput
                  id="OidcWellKnown"
                  name="OidcWellKnown"
                  value={ inputs.OidcWellKnown || '' }
                  onChange={ handleInputChange }
                  label="Well-Known URL"
                  placeholder="请输入 OIDC 的 Well-Known URL"
                  disabled={ loading }
                />
              </FormControl>
            </Grid>
            <Grid xs={ 12 } md={ 6 }>
              <FormControl fullWidth>
                <InputLabel htmlFor="OidcAuthorizationEndpoint">Authorization Endpoint</InputLabel>
                <OutlinedInput
                  id="OidcAuthorizationEndpoint"
                  name="OidcAuthorizationEndpoint"
                  value={ inputs.OidcAuthorizationEndpoint || '' }
                  onChange={ handleInputChange }
                  label="Authorization Endpoint"
                  placeholder="输入 OIDC 的 Authorization Endpoint"
                  disabled={ loading }
                />
              </FormControl>
            </Grid>
            <Grid xs={ 12 } md={ 6 }>
              <FormControl fullWidth>
                <InputLabel htmlFor="OidcTokenEndpoint">Token Endpoint</InputLabel>
                <OutlinedInput
                  id="OidcTokenEndpoint"
                  name="OidcTokenEndpoint"
                  value={ inputs.OidcTokenEndpoint || '' }
                  onChange={ handleInputChange }
                  label="Token Endpoint"
                  placeholder="输入 OIDC 的 Token Endpoint"
                  disabled={ loading }
                />
              </FormControl>
            </Grid>
            <Grid xs={ 12 } md={ 6 }>
              <FormControl fullWidth>
                <InputLabel htmlFor="OidcUserinfoEndpoint">Userinfo Endpoint</InputLabel>
                <OutlinedInput
                  id="OidcUserinfoEndpoint"
                  name="OidcUserinfoEndpoint"
                  value={ inputs.OidcUserinfoEndpoint || '' }
                  onChange={ handleInputChange }
                  label="Userinfo Endpoint"
                  placeholder="输入 OIDC 的 Userinfo Endpoint"
                  disabled={ loading }
                />
              </FormControl>
            </Grid>
            <Grid xs={ 12 }>
              <Button variant="contained" onClick={ submitOidc }>
                保存 OIDC 设置
              </Button>
            </Grid>
          </Grid>
        </SubCard>

        <SubCard
          title="配置 Message Pusher"
          subTitle={
            <span>
              用以推送报警信息，
              <a href="https://github.com/Laisky/message-pusher" target="_blank" rel="noreferrer">
                点击此处
              </a>
              了解 Message Pusher
            </span>
          }
        >
          <Grid container spacing={{ xs: 3, sm: 2, md: 4 }}>
            <Grid xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel htmlFor="MessagePusherAddress">Message Pusher 推送地址</InputLabel>
                <OutlinedInput
                  id="MessagePusherAddress"
                  name="MessagePusherAddress"
                  value={inputs.MessagePusherAddress || ''}
                  onChange={handleInputChange}
                  label="Message Pusher 推送地址"
                  placeholder="例如：https://msgpusher.com/push/your_username"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel htmlFor="MessagePusherToken">Message Pusher 访问凭证</InputLabel>
                <OutlinedInput
                  id="MessagePusherToken"
                  name="MessagePusherToken"
                  type="password"
                  value={inputs.MessagePusherToken || ''}
                  onChange={handleInputChange}
                  label="Message Pusher 访问凭证"
                  placeholder="敏感信息不会发送到前端显示"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12}>
              <Button variant="contained" onClick={submitMessagePusher}>
                保存 Message Pusher 设置
              </Button>
            </Grid>
          </Grid>
        </SubCard>
        <SubCard
          title="配置 Turnstile"
          subTitle={
            <span>
              用以支持用户校验，
              <a href="https://dash.cloudflare.com/" target="_blank" rel="noopener noreferrer">
                点击此处
              </a>
              管理你的 Turnstile Sites，推荐选择 Invisible Widget Type
            </span>
          }
        >
          <Grid container spacing={{ xs: 3, sm: 2, md: 4 }}>
            <Grid xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel htmlFor="TurnstileSiteKey">Turnstile Site Key</InputLabel>
                <OutlinedInput
                  id="TurnstileSiteKey"
                  name="TurnstileSiteKey"
                  value={inputs.TurnstileSiteKey || ''}
                  onChange={handleInputChange}
                  label="Turnstile Site Key"
                  placeholder="输入你注册的 Turnstile Site Key"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12} md={6}>
              <FormControl fullWidth>
                <InputLabel htmlFor="TurnstileSecretKey">Turnstile Secret Key</InputLabel>
                <OutlinedInput
                  id="TurnstileSecretKey"
                  name="TurnstileSecretKey"
                  type="password"
                  value={inputs.TurnstileSecretKey || ''}
                  onChange={handleInputChange}
                  label="Turnstile Secret Key"
                  placeholder="敏感信息不会发送到前端显示"
                  disabled={loading}
                />
              </FormControl>
            </Grid>
            <Grid xs={12}>
              <Button variant="contained" onClick={submitTurnstile}>
                保存 Turnstile 设置
              </Button>
            </Grid>
          </Grid>
        </SubCard>
      </Stack>
      <Dialog open={showPasswordWarningModal} onClose={() => setShowPasswordWarningModal(false)} maxWidth={'md'}>
        <DialogTitle sx={{ margin: '0px', fontWeight: 700, lineHeight: '1.55556', padding: '24px', fontSize: '1.125rem' }}>
          警告
        </DialogTitle>
        <Divider />
        <DialogContent>取消密码登录将导致所有未绑定其他登录方式的用户（包括管理员）无法通过密码登录，确认取消？</DialogContent>
        <DialogActions>
          <Button onClick={() => setShowPasswordWarningModal(false)}>取消</Button>
          <Button
            sx={{ color: 'error.main' }}
            onClick={async () => {
              setShowPasswordWarningModal(false);
              await updateOption('PasswordLoginEnabled', 'false');
            }}
          >
            确定
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

export default SystemSetting;
