import auth from "./auth.json";
import billing from "./billing.json";
import common from "./common.json";
import dashboard from "./dashboard.json";
import logs from "./logs.json";
import management from "./management.json";
import mcp from "./mcp.json";
import models from "./models.json";
import playground from "./playground.json";
import settings from "./settings.json";

const translations = {
	...common,
	...auth,
	...dashboard,
	...settings,
	...management,
	...playground,
	...models,
	...billing,
	...logs,
	...mcp,
};

export default translations;
