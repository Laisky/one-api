package model

// This registry is the single compile-time source of truth for every external
// UUID target. Catch-up phases, validation, and index management all derive
// their target lists from it (UUID-045/UUID-046), so a new UUID column cannot be
// added to only one phase.

// uuidResolverKind selects the specialized helper that resolves a reference.
type uuidResolverKind string

const (
	// uuidResolverIntFK resolves a non-nullable positive integer foreign key.
	uuidResolverIntFK uuidResolverKind = "int_fk"
	// uuidResolverNullableFK resolves a nullable positive integer foreign key.
	uuidResolverNullableFK uuidResolverKind = "nullable_fk"
	// uuidResolverTokenName resolves a historical (user_id, token_name) composite key.
	uuidResolverTokenName uuidResolverKind = "token_name"
)

// uuidOwnedTarget describes one table that owns an external UUID column.
type uuidOwnedTarget struct {
	// role is the database that authoritatively owns the table.
	role uuidDBRole
	// table is the trusted table name.
	table string
	// model is the GORM model used for schema introspection.
	model any
}

// uuidFKTarget describes one denormalized UUID column copied from a referenced row.
type uuidFKTarget struct {
	// role is the database that authoritatively owns the target table.
	role uuidDBRole
	// table is the trusted target table name.
	table string
	// model is the GORM model used for schema introspection.
	model any
	// fkColumn is the observed integer reference column; empty for composite resolvers.
	fkColumn string
	// uuidColumn is the denormalized UUID column to fill.
	uuidColumn string
	// refRole is the database that authoritatively owns the referenced table.
	refRole uuidDBRole
	// refTable is the trusted referenced owner table name.
	refTable string
	// resolver selects the specialized reference resolution helper.
	resolver uuidResolverKind
}

// uuidOwnedRegistry returns every table that owns an external UUID, keyed by database role.
// In split mode the logs entry is served exclusively by the log database handle, so a
// stale logs table on the primary database is never scanned or mutated (UUID-045).
// Parameters: none.
//
// Return values:
//   - []uuidOwnedTarget: all 12 owned UUID tables in dependency-friendly order.
func uuidOwnedRegistry() []uuidOwnedTarget {
	return []uuidOwnedTarget{
		{role: uuidRolePrimary, table: "users", model: &User{}},
		{role: uuidRolePrimary, table: "tokens", model: &Token{}},
		{role: uuidRolePrimary, table: "channels", model: &Channel{}},
		{role: uuidRolePrimary, table: "redemptions", model: &Redemption{}},
		{role: uuidRolePrimary, table: "token_transactions", model: &TokenTransaction{}},
		{role: uuidRolePrimary, table: "user_request_costs", model: &UserRequestCost{}},
		{role: uuidRolePrimary, table: "traces", model: &Trace{}},
		{role: uuidRolePrimary, table: "async_task_bindings", model: &AsyncTaskBinding{}},
		{role: uuidRolePrimary, table: "mcp_servers", model: &MCPServer{}},
		{role: uuidRolePrimary, table: "mcp_tools", model: &MCPTool{}},
		{role: uuidRolePrimary, table: "passkey_credentials", model: &PasskeyCredential{}},
		{role: uuidRoleLog, table: "logs", model: &Log{}},
	}
}

// uuidFKRegistry returns every denormalized UUID column and its authoritative reference.
// Parameters: none.
//
// Return values:
//   - []uuidFKTarget: all 15 denormalized FK UUID columns.
func uuidFKRegistry() []uuidFKTarget {
	return []uuidFKTarget{
		{role: uuidRolePrimary, table: "users", model: &User{}, fkColumn: "inviter_id", uuidColumn: "inviter_uuid",
			refRole: uuidRolePrimary, refTable: "users", resolver: uuidResolverIntFK},
		{role: uuidRolePrimary, table: "tokens", model: &Token{}, fkColumn: "user_id", uuidColumn: "user_uuid",
			refRole: uuidRolePrimary, refTable: "users", resolver: uuidResolverIntFK},
		{role: uuidRolePrimary, table: "redemptions", model: &Redemption{}, fkColumn: "user_id", uuidColumn: "user_uuid",
			refRole: uuidRolePrimary, refTable: "users", resolver: uuidResolverIntFK},
		{role: uuidRolePrimary, table: "token_transactions", model: &TokenTransaction{}, fkColumn: "token_id", uuidColumn: "token_uuid",
			refRole: uuidRolePrimary, refTable: "tokens", resolver: uuidResolverIntFK},
		{role: uuidRolePrimary, table: "token_transactions", model: &TokenTransaction{}, fkColumn: "user_id", uuidColumn: "user_uuid",
			refRole: uuidRolePrimary, refTable: "users", resolver: uuidResolverIntFK},
		{role: uuidRolePrimary, table: "user_request_costs", model: &UserRequestCost{}, fkColumn: "user_id", uuidColumn: "user_uuid",
			refRole: uuidRolePrimary, refTable: "users", resolver: uuidResolverIntFK},
		{role: uuidRolePrimary, table: "async_task_bindings", model: &AsyncTaskBinding{}, fkColumn: "user_id", uuidColumn: "user_uuid",
			refRole: uuidRolePrimary, refTable: "users", resolver: uuidResolverIntFK},
		{role: uuidRolePrimary, table: "async_task_bindings", model: &AsyncTaskBinding{}, fkColumn: "token_id", uuidColumn: "token_uuid",
			refRole: uuidRolePrimary, refTable: "tokens", resolver: uuidResolverIntFK},
		{role: uuidRolePrimary, table: "async_task_bindings", model: &AsyncTaskBinding{}, fkColumn: "channel_id", uuidColumn: "channel_uuid",
			refRole: uuidRolePrimary, refTable: "channels", resolver: uuidResolverIntFK},
		{role: uuidRolePrimary, table: "mcp_tools", model: &MCPTool{}, fkColumn: "server_id", uuidColumn: "server_uuid",
			refRole: uuidRolePrimary, refTable: "mcp_servers", resolver: uuidResolverIntFK},
		{role: uuidRolePrimary, table: "passkey_credentials", model: &PasskeyCredential{}, fkColumn: "user_id", uuidColumn: "user_uuid",
			refRole: uuidRolePrimary, refTable: "users", resolver: uuidResolverIntFK},

		// Authoritative log targets resolved from primary owners.
		{role: uuidRoleLog, table: "logs", model: &Log{}, fkColumn: "user_id", uuidColumn: "user_uuid",
			refRole: uuidRolePrimary, refTable: "users", resolver: uuidResolverIntFK},
		{role: uuidRoleLog, table: "logs", model: &Log{}, fkColumn: "channel_id", uuidColumn: "channel_uuid",
			refRole: uuidRolePrimary, refTable: "channels", resolver: uuidResolverIntFK},
		{role: uuidRoleLog, table: "logs", model: &Log{}, uuidColumn: "token_uuid",
			refRole: uuidRolePrimary, refTable: "tokens", resolver: uuidResolverTokenName},

		// Primary target resolved only from authoritative log owners.
		{role: uuidRolePrimary, table: "token_transactions", model: &TokenTransaction{}, fkColumn: "log_id", uuidColumn: "log_uuid",
			refRole: uuidRoleLog, refTable: "logs", resolver: uuidResolverNullableFK},
	}
}

// ownedTargetsForRole returns the registry's owned UUID tables for one database role.
// Parameters:
//   - role: database role to filter by.
//
// Return values:
//   - []uuidOwnedTarget: owned targets whose authoritative owner is role.
func ownedTargetsForRole(role uuidDBRole) []uuidOwnedTarget {
	targets := make([]uuidOwnedTarget, 0, len(uuidOwnedRegistry()))
	for _, target := range uuidOwnedRegistry() {
		if target.role == role {
			targets = append(targets, target)
		}
	}
	return targets
}

// fkTargetsForRoles returns the registry's FK UUID columns for one target/reference role pair.
// Phase ordering is expressed by calling this with the role pair a phase is allowed
// to touch, which keeps a single dependency-correct code path for both topologies.
// Parameters:
//   - role: database role owning the target table.
//   - refRole: database role owning the referenced table.
//
// Return values:
//   - []uuidFKTarget: matching FK targets in registry order.
func fkTargetsForRoles(role uuidDBRole, refRole uuidDBRole) []uuidFKTarget {
	targets := make([]uuidFKTarget, 0, len(uuidFKRegistry()))
	for _, target := range uuidFKRegistry() {
		if target.role == role && target.refRole == refRole {
			targets = append(targets, target)
		}
	}
	return targets
}

// Phase names label progress logs and metrics. They are compile-time constants, so they are
// safe as bounded metric labels.
const (
	// uuidPhaseOwned generates the UUID that identifies a row itself.
	uuidPhaseOwned = "owned"
	// uuidPhaseFK copies a UUID from a row referenced by an integer foreign key.
	uuidPhaseFK = "fk"
	// uuidPhaseTokenName resolves a UUID from a historical (user_id, token_name) key.
	uuidPhaseTokenName = "token_name"
)

// uuidFKPhase names one FK reconciliation phase and the role pair it may touch.
// Naming the phase, rather than relying on its position in a slice, keeps the coordinator's
// dependency order and its error messages from silently disagreeing if the list is reordered.
type uuidFKPhase struct {
	// name identifies the phase in errors and logs.
	name string
	// role is the database owning the target tables of this phase.
	role uuidDBRole
	// refRole is the database owning the referenced tables of this phase.
	refRole uuidDBRole
}

// FK phase names, ordered by the dependency graph they encode.
const (
	// uuidFKPhasePrimaryLocal resolves primary references from primary owners.
	uuidFKPhasePrimaryLocal = "primary-local fk uuids"
	// uuidFKPhaseLogFromPrimary resolves authoritative log references from primary owners.
	uuidFKPhaseLogFromPrimary = "log fk uuids from primary owners"
	// uuidFKPhasePrimaryFromLog resolves primary references from authoritative log owners.
	uuidFKPhasePrimaryFromLog = "primary fk uuids from authoritative log owners"
)

// uuidFKPhaseOrder returns the ordered FK reconciliation phases.
// The order encodes the dependency graph: primary owners are populated before any reference
// to them is resolved, and authoritative log owners are populated before
// token_transactions.log_uuid is resolved from them. The coordinator interleaves the
// owned-UUID phases between these entries; this list also lets a completeness test prove the
// union of the FK phases equals the whole FK registry.
// Parameters: none.
//
// Return values:
//   - []uuidFKPhase: ordered FK phases.
func uuidFKPhaseOrder() []uuidFKPhase {
	return []uuidFKPhase{
		{name: uuidFKPhasePrimaryLocal, role: uuidRolePrimary, refRole: uuidRolePrimary},
		{name: uuidFKPhaseLogFromPrimary, role: uuidRoleLog, refRole: uuidRolePrimary},
		{name: uuidFKPhasePrimaryFromLog, role: uuidRolePrimary, refRole: uuidRoleLog},
	}
}
