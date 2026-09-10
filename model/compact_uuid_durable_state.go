package model

import (
	"context"

	"github.com/Laisky/errors/v2"
)

// compactAuditRequiredManifestKey stores crash-safe evidence that completed compact data must be
// fully revalidated. Completion markers remain immutable; this control row is independent state.
const compactAuditRequiredManifestKey = compactMigrationGeneration + "_audit_required"

// compactFullAuditRequired reports whether an earlier process observed drift that has not yet
// completed two clean full validation passes.
// Parameters:
//   - ctx: context bounding the control-row lookup.
//   - topology: topology whose primary database carries coordinator control state.
//
// Return values:
//   - bool: true when a full audit is durably required.
//   - error: wrapped error when the state cannot be read.
func compactFullAuditRequired(ctx context.Context, topology *databaseTopology) (bool, error) {
	var count int64
	if err := topology.primary.WithContext(ctx).Model(&compactManifestRow{}).
		Where("manifest_key = ?", compactAuditRequiredManifestKey).Count(&count).Error; err != nil {
		return false, errors.Wrap(err, "read compact audit-required state")
	}
	return count > 0, nil
}

// persistFullAuditRequired records drift before the worker performs any recovery mutation.
// Parameters:
//   - ctx: context bounding the control-row write.
//
// Return values:
//   - error: wrapped error when durable state cannot be recorded.
func (coordinator *compactCoordinator) persistFullAuditRequired(ctx context.Context) error {
	coordinator.resetEpoch()
	coordinator.requireFullAudit()
	required, err := compactFullAuditRequired(ctx, coordinator.topology)
	if err != nil {
		return err
	}
	if required {
		return nil
	}
	row := &compactManifestRow{ManifestKey: compactAuditRequiredManifestKey, Payload: "required"}
	if err := coordinator.topology.primary.WithContext(ctx).Create(row).Error; err != nil {
		if !isDuplicateObjectError(err) {
			return errors.Wrap(err, "persist compact audit-required state")
		}
		required, readErr := compactFullAuditRequired(ctx, coordinator.topology)
		if readErr != nil {
			return readErr
		}
		if !required {
			return errors.Wrap(err, "confirm compact audit-required state after insert race")
		}
	}
	return nil
}

// clearFullAuditRequired clears durable drift evidence only after full validation has succeeded.
// Parameters:
//   - ctx: context bounding the control-row deletion.
//   - topology: topology whose primary database carries coordinator control state.
//
// Return values:
//   - error: wrapped error when the state cannot be cleared.
func clearFullAuditRequired(ctx context.Context, topology *databaseTopology) error {
	if err := topology.primary.WithContext(ctx).
		Where("manifest_key = ?", compactAuditRequiredManifestKey).
		Delete(&compactManifestRow{}).Error; err != nil {
		return errors.Wrap(err, "clear compact audit-required state")
	}
	return nil
}
