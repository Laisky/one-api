import React from 'react';
import { Typography, Tooltip } from '@douyinfe/semi-ui';
import { IconHelpCircle } from '@douyinfe/semi-icons';

/**
 * LabelWithTooltip renders a strong field label followed by an optional help icon tooltip.
 *
 * @param {object} props The component props.
 * @param {React.ReactNode} props.label The label text.
 * @param {React.ReactNode} [props.helpText] The tooltip content; no help icon is rendered when it is empty.
 * @param {React.ReactNode} [props.children] Extra content rendered after the label and icon.
 * @returns {JSX.Element} The label row.
 */
const LabelWithTooltip = ({ label, helpText, children, ...props }) => (
    <div style={{ display: 'flex', alignItems: 'center', marginBottom: '8px' }}>
        <Typography.Text strong {...props}>
            {label}
        </Typography.Text>
        {helpText && (
            <Tooltip content={helpText} position="top">
                <IconHelpCircle
                    style={{
                        marginLeft: '6px',
                        color: '#999',
                        cursor: 'help',
                        fontSize: '14px'
                    }}
                />
            </Tooltip>
        )}
        {children}
    </div>
);

export default LabelWithTooltip;
