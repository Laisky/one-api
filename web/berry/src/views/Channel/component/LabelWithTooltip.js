import { Box, InputLabel, Tooltip, IconButton } from '@mui/material';
import { HelpOutline } from '@mui/icons-material';

/**
 * LabelWithTooltip renders a field label followed by an optional help icon tooltip.
 *
 * @param {object} props The component props; any other props (htmlFor, sx, ...) are passed to the InputLabel, and sx also styles the row.
 * @param {React.ReactNode} props.label The label text.
 * @param {React.ReactNode} [props.helpText] The tooltip content; no help icon is rendered when it is empty.
 * @param {React.ReactNode} [props.children] Extra content rendered after the label and icon.
 * @returns {JSX.Element} The label row.
 */
const LabelWithTooltip = ({ label, helpText, children, ...props }) => (
  <Box sx={{ display: 'flex', alignItems: 'center', ...props.sx }}>
    <InputLabel {...props}>
      {label}
    </InputLabel>
    {helpText && (
      <Tooltip title={helpText} placement="top" arrow>
        <IconButton size="small" sx={{ ml: 0.5, p: 0.25 }}>
          <HelpOutline sx={{ fontSize: 16, color: 'text.secondary' }} />
        </IconButton>
      </Tooltip>
    )}
    {children}
  </Box>
);

export default LabelWithTooltip;
