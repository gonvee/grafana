/**
 * Contains all handlers that are required for test rendering of components within Alerting
 */

import alertmanagerHandlers from 'app/features/alerting/unified/mocks/server/handlers/alertmanagers';
import datasourcesHandlers from 'app/features/alerting/unified/mocks/server/handlers/datasources';
import evalHandlers from 'app/features/alerting/unified/mocks/server/handlers/eval';
import folderHandlers from 'app/features/alerting/unified/mocks/server/handlers/folders';
import pluginsHandlers from 'app/features/alerting/unified/mocks/server/handlers/plugins';
import silenceHandlers from 'app/features/alerting/unified/mocks/server/handlers/silences';

import { alertRuleHandlers } from './handlers/alertRule';

/**
 * Array of all mock handlers that are required across Alerting tests
 */
const allHandlers = [
  ...alertmanagerHandlers,
  ...datasourcesHandlers,
  ...evalHandlers,
  ...folderHandlers,
  ...pluginsHandlers,
  ...silenceHandlers,
  ...alertRuleHandlers,
];

export default allHandlers;
