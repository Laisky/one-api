package model

import (
	"context"
	"sync"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// uuidTopologyMode identifies where the authoritative logs table lives.
type uuidTopologyMode string

const (
	// uuidTopologyUnified means logs share the primary database handle.
	uuidTopologyUnified uuidTopologyMode = "unified"
	// uuidTopologySplit means a dedicated LOG_DSN owns the authoritative logs table.
	uuidTopologySplit uuidTopologyMode = "split"
)

// uuidMigrationMode selects the coordinator behaviour for one invocation.
type uuidMigrationMode string

const (
	// uuidMigrationModeCatchUp reconciles data without index promotion or marker writes.
	uuidMigrationModeCatchUp uuidMigrationMode = "catchup"
	// uuidMigrationModeFinalizer reconciles, promotes indexes, validates, and writes markers.
	uuidMigrationModeFinalizer uuidMigrationMode = "finalizer"
)

// uuidDBRole identifies which physical database authoritatively owns a table.
type uuidDBRole string

const (
	// uuidRolePrimary owns every non-log resource table.
	uuidRolePrimary uuidDBRole = "primary"
	// uuidRoleLog owns the authoritative logs table.
	uuidRoleLog uuidDBRole = "log"
)

// databaseTopology is the explicit initialization-time description of database roles.
// It is constructed once by the bootstrap path and passed down the migration call
// chain so migration code never rediscovers physical topology by comparing
// gorm.DB pointers. A session clone or instrumentation wrapper therefore cannot
// change the selected mode.
type databaseTopology struct {
	primary *gorm.DB
	log     *gorm.DB
	mode    uuidTopologyMode
}

var (
	currentTopologyMu sync.RWMutex
	currentTopology   *databaseTopology
)

// newUnifiedTopology builds a topology where the primary database also owns logs.
// Parameters:
//   - primary: initialized primary database handle.
//
// Return values:
//   - *databaseTopology: unified topology value.
//   - error: wrapped error when the handle is nil.
func newUnifiedTopology(primary *gorm.DB) (*databaseTopology, error) {
	if primary == nil {
		return nil, errors.New("primary database handle is nil")
	}
	return &databaseTopology{primary: primary, log: primary, mode: uuidTopologyUnified}, nil
}

// newSplitTopology builds a topology where a dedicated database owns logs.
// The mode is selected by the configuration path, not by handle comparison, so a
// deployment pointing both DSNs at one physical server is still treated as split.
// Parameters:
//   - primary: initialized primary database handle.
//   - log: initialized authoritative log database handle.
//
// Return values:
//   - *databaseTopology: split topology value.
//   - error: wrapped error when either handle is nil.
func newSplitTopology(primary *gorm.DB, log *gorm.DB) (*databaseTopology, error) {
	if primary == nil {
		return nil, errors.New("primary database handle is nil")
	}
	if log == nil {
		return nil, errors.New("log database handle is nil")
	}
	return &databaseTopology{primary: primary, log: log, mode: uuidTopologySplit}, nil
}

// handle returns the database handle that authoritatively owns the supplied role.
// Parameters:
//   - role: registry database role.
//
// Return values:
//   - *gorm.DB: handle for the role, or nil when the topology is not initialized.
func (topology *databaseTopology) handle(role uuidDBRole) *gorm.DB {
	if topology == nil {
		return nil
	}
	if role == uuidRoleLog {
		return topology.log
	}
	return topology.primary
}

// markerRoles returns the physical database roles that must carry completion markers.
// Unified deployments have exactly one marker; split deployments have two.
// Parameters: none.
//
// Return values:
//   - []uuidDBRole: roles requiring a current-generation completion marker.
func (topology *databaseTopology) markerRoles() []uuidDBRole {
	if topology == nil || topology.mode != uuidTopologySplit {
		return []uuidDBRole{uuidRolePrimary}
	}
	return []uuidDBRole{uuidRolePrimary, uuidRoleLog}
}

// validate rejects nil, partially initialized, or unknown-mode topologies.
// It issues no metadata queries, so a completed coordinator invocation can prove it
// performed nothing but its marker lookups. Schema completeness is a separate check
// that only runs once a marker is found absent.
// Parameters: none.
//
// Return values:
//   - error: wrapped error when a handle is missing or the mode is unknown.
func (topology *databaseTopology) validate() error {
	if topology == nil {
		return errors.New("database topology is not initialized")
	}
	switch topology.mode {
	case uuidTopologyUnified:
		if topology.primary == nil {
			return errors.New("unified topology has no primary database handle")
		}
		if topology.log != topology.primary {
			return errors.New("unified topology must reuse the primary handle for logs")
		}
	case uuidTopologySplit:
		if topology.primary == nil {
			return errors.New("split topology has no primary database handle")
		}
		if topology.log == nil {
			return errors.New("split topology has no log database handle")
		}
	default:
		return errors.Errorf("unknown database topology mode %q", topology.mode)
	}
	return nil
}

// validateSchema rejects handles whose migration schema is incomplete.
// It runs only on the incomplete-marker path, before any target access or marker write,
// because it necessarily issues metadata queries.
// Parameters:
//   - ctx: context controlling the metadata reads.
//
// Return values:
//   - error: wrapped error when a marker-carrying database cannot serve data_migrations.
func (topology *databaseTopology) validateSchema(ctx context.Context) error {
	for _, role := range topology.markerRoles() {
		db := topology.handle(role)
		migrator := db.WithContext(ctx).Migrator()
		if migrator == nil {
			return errors.Errorf("%s database migrator is unavailable", role)
		}
		if !migrator.HasTable(&DataMigration{}) {
			return errors.Errorf("%s database is missing the data_migrations table", role)
		}
	}
	return nil
}

// setDatabaseTopology installs the process-wide topology used by bootstrap wrappers.
// Parameters:
//   - topology: explicitly constructed topology value, or nil to clear it.
//
// Return values: none.
func setDatabaseTopology(topology *databaseTopology) {
	currentTopologyMu.Lock()
	currentTopology = topology
	currentTopologyMu.Unlock()
}

// databaseTopologySnapshot returns the process-wide topology installed at initialization.
// Parameters: none.
//
// Return values:
//   - *databaseTopology: current topology, or nil when initialization has not run.
func databaseTopologySnapshot() *databaseTopology {
	currentTopologyMu.RLock()
	defer currentTopologyMu.RUnlock()
	return currentTopology
}

// validateUUIDMigrationMode rejects unknown coordinator modes before any database access.
// Parameters:
//   - mode: requested coordinator mode.
//
// Return values:
//   - error: wrapped error when the mode is not catch-up or finalizer.
func validateUUIDMigrationMode(mode uuidMigrationMode) error {
	switch mode {
	case uuidMigrationModeCatchUp, uuidMigrationModeFinalizer:
		return nil
	default:
		return errors.Errorf("unknown external uuid migration mode %q", mode)
	}
}
