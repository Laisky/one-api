package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// backfillPrimaryFKUUIDs fills denormalized FK UUID columns that live on the primary database.
// Parameters:
//   - ctx: context controlling batched migration writes.
//
// Return values:
//   - error: wrapped migration error when any FK backfill fails.
func backfillPrimaryFKUUIDs(ctx context.Context) error {
	refsByTable := map[string]map[int]string{}
	for _, target := range primaryUUIDFKProbeTargets() {
		needsBackfill, err := hasBackfillableFKUUIDs(ctx, DB, target)
		if err != nil {
			return errors.Wrapf(err, "check %s.%s", target.table, target.uuidColumn)
		}
		if !needsBackfill {
			continue
		}
		refs, ok := refsByTable[target.refTable]
		if !ok {
			refs, err = loadIDUUIDMap(ctx, DB, target.refTable)
			if err != nil {
				return errors.Wrapf(err, "load %s uuid map", target.refTable)
			}
			refsByTable[target.refTable] = refs
		}
		refTarget := uuidRefTarget{
			table:      target.table,
			model:      target.model,
			fkColumn:   target.fkColumn,
			uuidColumn: target.uuidColumn,
			refs:       refs,
		}
		if target.nullableFK {
			if err := backfillNullableFKUUIDs(ctx, DB, refTarget); err != nil {
				return errors.Wrapf(err, "backfill %s.%s", target.table, target.uuidColumn)
			}
			continue
		}
		if err := backfillFKUUIDs(ctx, DB, refTarget); err != nil {
			return errors.Wrapf(err, "backfill %s.%s", target.table, target.uuidColumn)
		}
	}

	logTarget := uuidRefProbeTarget{
		table:      "token_transactions",
		model:      &TokenTransaction{},
		fkColumn:   "log_id",
		uuidColumn: "log_uuid",
		refTable:   "logs",
		nullableFK: true,
	}
	needsLogUUIDs, err := hasBackfillableFKUUIDs(ctx, DB, logTarget)
	if err != nil {
		return errors.Wrap(err, "check token transaction log uuid gaps")
	}
	if needsLogUUIDs {
		logUUIDs, err := loadIDUUIDMap(ctx, DB, "logs")
		if err != nil {
			return errors.Wrap(err, "load log uuid map")
		}
		if err := backfillNullableFKUUIDs(ctx, DB, uuidRefTarget{
			table:      "token_transactions",
			model:      &TokenTransaction{},
			fkColumn:   "log_id",
			uuidColumn: "log_uuid",
			refs:       logUUIDs,
		}); err != nil {
			return errors.Wrap(err, "backfill token_transactions.log_uuid")
		}
	}
	return nil
}

// backfillLogFKUUIDs fills log user and channel UUID columns only when rows need those values.
// Parameters:
//   - ctx: context controlling batched migration writes.
//   - logDB: database handle containing the logs table.
//
// Return values:
//   - error: wrapped migration error when any log FK backfill fails.
func backfillLogFKUUIDs(ctx context.Context, logDB *gorm.DB) error {
	refsByTable := map[string]map[int]string{}
	for _, target := range logUUIDFKProbeTargets() {
		var (
			needsBackfill bool
			err           error
		)
		if logDB == DB {
			needsBackfill, err = hasBackfillableFKUUIDs(ctx, logDB, target)
		} else {
			needsBackfill, err = hasMissingFKUUIDCandidate(ctx, logDB, target)
		}
		if err != nil {
			return errors.Wrapf(err, "check logs.%s", target.uuidColumn)
		}
		if !needsBackfill {
			continue
		}
		refs, ok := refsByTable[target.refTable]
		if !ok {
			refs, err = loadIDUUIDMap(ctx, DB, target.refTable)
			if err != nil {
				return errors.Wrapf(err, "load %s uuid map for log db", target.refTable)
			}
			refsByTable[target.refTable] = refs
		}
		if err := backfillFKUUIDs(ctx, logDB, uuidRefTarget{
			table:      target.table,
			model:      target.model,
			fkColumn:   target.fkColumn,
			uuidColumn: target.uuidColumn,
			refs:       refs,
		}); err != nil {
			return errors.Wrapf(err, "backfill logs.%s", target.uuidColumn)
		}
	}
	return nil
}

// backfillTokenTransactionLogUUIDs fills token transaction log UUIDs from the provided log database.
// Parameters:
//   - ctx: context controlling batched migration writes.
//   - logDB: database handle containing authoritative log rows.
//
// Return values:
//   - error: wrapped migration error when the log map or update fails.
func backfillTokenTransactionLogUUIDs(ctx context.Context, logDB *gorm.DB) error {
	logUUIDs, err := loadIDUUIDMap(ctx, logDB, "logs")
	if err != nil {
		return errors.Wrap(err, "load split log uuid map")
	}
	if err := backfillNullableFKUUIDs(ctx, DB, uuidRefTarget{
		table:      "token_transactions",
		model:      &TokenTransaction{},
		fkColumn:   "log_id",
		uuidColumn: "log_uuid",
		refs:       logUUIDs,
	}); err != nil {
		return errors.Wrap(err, "backfill token_transactions.log_uuid")
	}
	return nil
}
