import PropTypes from 'prop-types';
import { Tooltip, Stack, Container } from '@mui/material';
import Label from 'ui-component/Label';
import { styled } from '@mui/material/styles';
import { showSuccess, copy } from 'utils/common';

const TooltipContainer = styled(Container)({
  maxHeight: '250px',
  overflow: 'auto',
  '&::-webkit-scrollbar': {
    width: '0px' // Set the width to 0 to hide the scrollbar
  }
});

const NameLabel = ({ name, models, refId }) => {
  let modelMap = [];
  modelMap = models.split(',');
  modelMap.sort();

  return (
    <Tooltip
      title={
        <TooltipContainer>
          <Stack spacing={1}>
            {refId ? (
              <Label
                variant="ghost"
                onClick={() => {
                  copy(String(refId), 'ID');
                }}
              >
                ID: {refId}
              </Label>
            ) : null}
            {modelMap.map((item, index) => {
              return (
                <Label
                  variant="ghost"
                  key={index}
                  onClick={() => {
                    copy(item, '模型名称');
                  }}
                >
                  {item}
                </Label>
              );
            })}
          </Stack>
        </TooltipContainer>
      }
      placement="top"
    >
      <span className="resource-name-with-id">{name}</span>
    </Tooltip>
  );
};

NameLabel.propTypes = {
  name: PropTypes.string,
  models: PropTypes.string,
  refId: PropTypes.oneOfType([PropTypes.string, PropTypes.number])
};

export default NameLabel;
