import tableSelection from './table-selection.json';
import { channelResetTranslations } from '../channel-reset';
import { tokenDuplicateTranslations } from '../token-duplicate';
import { duplicateActionTranslations } from '../duplicate-action';
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
  channel_reset: channelResetTranslations.ja,
  token_duplicate: tokenDuplicateTranslations.ja,
  duplicate_action: duplicateActionTranslations.ja,
};

export default translations;
