from pathlib import Path

worker = Path('model/quota_refund_worker.go')
old = '''			if updateErr := DB.WithContext(ctx).Model(&QuotaRefund{}).
				Where("id = ? AND status = ?", intent.ID, QuotaRefundPending).
				Updates(map[string]any{"retry_at": time.Now().UTC().Add(30 * time.Second).Unix(),
					"attempts": gorm.Expr("attempts + 1")}).Error; updateErr != nil {
'''
new = '''			// A sweep timeout must not strand this row at the queue head.
			// Preserve trace values, but allow at most one extra second for
			// deferral after the sweep deadline or worker cancellation.
			deferCtx, cancelDefer := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			updateErr := DB.WithContext(deferCtx).Model(&QuotaRefund{}).
				Where("id = ? AND status = ?", intent.ID, QuotaRefundPending).
				Updates(map[string]any{"retry_at": time.Now().UTC().Add(30 * time.Second).Unix(),
					"attempts": gorm.Expr("attempts + 1")}).Error
			cancelDefer()
			if updateErr != nil {
'''
text = worker.read_text()
assert text.count(old) == 1, 'worker source drift'
worker.write_text(text.replace(old, new))
doc = Path('docs/model-protocol-support-20260922.md')
text = doc.read_text()
old_pattern = 'Test.*(ProtocolAudit|Audio|Video|Pricing|ModelPrice|ModelConfig)'
new_pattern = 'Test.*(ProtocolAudit|Audio|Video|Pricing|ModelPrice|ModelConfig|QuotaRefund)'
assert text.count(old_pattern) == 1, 'qualification command drift'
text = text.replace(old_pattern, new_pattern)
text += '\nA timed-out refund is deferred on a fresh one-second context, preserving trace\nvalues without inheriting the expired sweep deadline. This prevents a blocked\nhead row from starving later refunds. Shutdown may wait up to this extra second;\na failed deferral still leaves the refund pending without a financial movement.\n'
doc.write_text(text)
