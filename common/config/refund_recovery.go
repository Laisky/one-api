package config

import "github.com/Laisky/one-api/common/env"

// RefundCostRecoveryEnabled permits delayed cost reconciliation only after every
// cost writer has been upgraded to increment revisions. Disabled by default for
// mixed-version rollouts; enabling it never qualifies previously captured intents.
var RefundCostRecoveryEnabled = env.Bool("REFUND_COST_RECOVERY_ENABLED", false)
