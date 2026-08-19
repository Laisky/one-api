import React from 'react';
import { Tooltip } from '@douyinfe/semi-ui';
import { copy, showError, showSuccess } from '../helpers';

/**
 * ResourceRefTooltip renders a resource label whose tooltip exposes the resource
 * reference (the UUID when the backend provides one, otherwise the legacy numeric
 * id). Clicking the tooltip body copies the reference to the clipboard so an
 * operator can paste it straight into a list page search box.
 *
 * Parameters:
 *   - refId: string|number, the resource reference to display and copy.
 *   - children: ReactNode, the visible label (usually the resource name).
 *   - label: string, optional prefix shown before the reference. Defaults to 'ID'.
 *
 * Return value: a ReactNode wrapping children with the reference tooltip, or
 * children untouched when no reference is available.
 */
const ResourceRefTooltip = ({ refId, children, label = 'ID' }) => {
  if (refId === undefined || refId === null || refId === '') {
    return <>{children}</>;
  }

  const text = String(refId);

  const handleCopy = async (e) => {
    // Keep row-level click handlers (e.g. the logs tracing modal) from firing.
    e.stopPropagation();
    if (await copy(text)) {
      showSuccess('已复制：' + text);
    } else {
      showError('无法复制到剪贴板，请手动复制：' + text);
    }
  };

  return (
    <Tooltip
      content={
        <span
          style={{ cursor: 'pointer', wordBreak: 'break-all' }}
          onClick={handleCopy}
          title="点击复制"
        >
          {label}: {text}
        </span>
      }
    >
      <span className="resource-name-with-id">{children}</span>
    </Tooltip>
  );
};

export default ResourceRefTooltip;
