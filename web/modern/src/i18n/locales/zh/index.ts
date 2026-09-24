import tableSelection from './table-selection.json';
import { channelResetTranslations } from '../channel-reset';
import auth from './auth.json';
import billing from './billing.json';
import common from './common.json';
import dashboard from './dashboard.json';
import logs from './logs.json';
import management from './management.json';
import mcp from './mcp.json';
import modelApi from './model-api.json';
import models from './models.json';
import playground from './playground.json';
import settings from './settings.json';
import tools from './tools.json';

const translations = {
  ...tableSelection,
  ...common,
  ...auth,
  ...dashboard,
  ...settings,
  ...management,
  ...playground,
  ...models,
  ...modelApi,
  ...billing,
  ...logs,
  ...mcp,
  ...tools,
  channel_reset: channelResetTranslations.zh,
};

export default translations;
