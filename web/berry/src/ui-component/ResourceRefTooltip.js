import PropTypes from 'prop-types';
import { Tooltip } from '@mui/material';
import Label from 'ui-component/Label';
import { copy } from 'utils/common';

/**
 * ResourceRefTooltip renders a resource label whose tooltip exposes the resource
 * reference (the UUID when the backend provides one, otherwise the legacy numeric
 * id). Clicking the reference copies it to the clipboard so an operator can paste
 * it straight into a list page search box.
 *
 * Parameters:
 *   - refId: string|number, the resource reference to display and copy.
 *   - children: node, the visible label (usually the resource name).
 *   - label: string, optional prefix shown before the reference. Defaults to 'ID'.
 *
 * Return value: a node wrapping children with the reference tooltip, or children
 * untouched when no reference is available.
 */
const ResourceRefTooltip = ({ refId, children, label = 'ID' }) => {
  if (refId === undefined || refId === null || refId === '') {
    return <>{children}</>;
  }

  const text = String(refId);

  return (
    <Tooltip
      placement="top"
      title={
        <Label
          variant="ghost"
          onClick={(event) => {
            // Keep row-level click handlers (e.g. the logs tracing modal) from firing.
            event.stopPropagation();
            copy(text, label);
          }}
        >
          {label}: {text}
        </Label>
      }
    >
      <span className="resource-name-with-id">{children}</span>
    </Tooltip>
  );
};

ResourceRefTooltip.propTypes = {
  refId: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
  children: PropTypes.node,
  label: PropTypes.string
};

export default ResourceRefTooltip;
