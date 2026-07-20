import PropTypes from "prop-types";
import { useTheme } from "@mui/material/styles";
import { useState } from "react";
import {
  IconUser,
  IconKey,
  IconBrandGithubCopilot,
  IconSitemap,
  IconSearch,
} from "@tabler/icons-react";
import {
  InputAdornment,
  OutlinedInput,
  Stack,
  FormControl,
  FormHelperText,
  InputLabel,
  Select,
  MenuItem,
  Autocomplete,
  TextField,
} from "@mui/material";
import { API } from "utils/api";
import { LocalizationProvider, DateTimePicker } from "@mui/x-date-pickers";
import { AdapterDayjs } from "@mui/x-date-pickers/AdapterDayjs";
import dayjs from "dayjs";
import LogType from "../type/LogType";
require("dayjs/locale/zh-cn");
// ----------------------------------------------------------------------

export default function TableToolBar({
  filterName,
  handleFilterName,
  userIsAdmin,
}) {
  const theme = useTheme();
  const grey500 = theme.palette.grey[500];
  // The keyword search endpoint only understands the keyword itself, so the
  // structured filters below are greyed out while a keyword is present rather
  // than silently ignored.
  const keywordActive = (filterName.keyword || "").trim() !== "";
  const [userOptions, setUserOptions] = useState([]);
  const [userSearchLoading, setUserSearchLoading] = useState(false);

  const searchUsers = async (searchQuery) => {
    if (!searchQuery.trim()) {
      setUserOptions([]);
      return;
    }

    setUserSearchLoading(true);
    try {
      const res = await API.get(`/api/user/search?keyword=${searchQuery}`);
      const { success, data } = res.data;
      if (success) {
        const options = data.map(user => ({
          id: user.id,
          username: user.username,
          label: `${user.display_name || user.username} (@${user.username})`,
          display_name: user.display_name
        }));
        setUserOptions(options);
      }
    } catch (error) {
      console.error('Failed to search users:', error);
    } finally {
      setUserSearchLoading(false);
    }
  };

  return (
    <>
      <Stack
        direction={{ xs: "column", sm: "row" }}
        spacing={{ xs: 3, sm: 2, md: 4 }}
        padding={"24px"}
        paddingBottom={"0px"}
      >
        <FormControl>
          <InputLabel htmlFor="log-keyword-label">关键字</InputLabel>
          <OutlinedInput
            id="keyword"
            name="keyword"
            sx={{
              minWidth: "100%",
            }}
            label="关键字"
            value={filterName.keyword}
            onChange={handleFilterName}
            placeholder="按 UUID 或详情搜索 ..."
            startAdornment={
              <InputAdornment position="start">
                <IconSearch stroke={1.5} size="20px" color={grey500} />
              </InputAdornment>
            }
          />
          {keywordActive && (
            <FormHelperText>关键字搜索会忽略其他筛选条件。</FormHelperText>
          )}
        </FormControl>
        <FormControl>
          <InputLabel htmlFor="channel-token_name-label">令牌名称</InputLabel>
          <OutlinedInput
            id="token_name"
            name="token_name"
            disabled={keywordActive}
            sx={{
              minWidth: "100%",
            }}
            label="令牌名称"
            value={filterName.token_name}
            onChange={handleFilterName}
            placeholder="令牌名称"
            startAdornment={
              <InputAdornment position="start">
                <IconKey stroke={1.5} size="20px" color={grey500} />
              </InputAdornment>
            }
          />
        </FormControl>
        <FormControl>
          <InputLabel htmlFor="channel-model_name-label">模型名称</InputLabel>
          <OutlinedInput
            id="model_name"
            name="model_name"
            disabled={keywordActive}
            sx={{
              minWidth: "100%",
            }}
            label="模型名称"
            value={filterName.model_name}
            onChange={handleFilterName}
            placeholder="模型名称"
            startAdornment={
              <InputAdornment position="start">
                <IconBrandGithubCopilot
                  stroke={1.5}
                  size="20px"
                  color={grey500}
                />
              </InputAdornment>
            }
          />
        </FormControl>

        <FormControl>
          <LocalizationProvider
            dateAdapter={AdapterDayjs}
            adapterLocale={"zh-cn"}
          >
            <DateTimePicker
              label="起始时间"
              ampm={false}
              name="start_timestamp"
              disabled={keywordActive}
              value={
                filterName.start_timestamp === 0
                  ? null
                  : dayjs.unix(filterName.start_timestamp)
              }
              onChange={(value) => {
                if (value === null) {
                  handleFilterName({
                    target: { name: "start_timestamp", value: 0 },
                  });
                  return;
                }
                handleFilterName({
                  target: { name: "start_timestamp", value: value.unix() },
                });
              }}
              slotProps={{
                actionBar: {
                  actions: ["clear", "today", "accept"],
                },
              }}
            />
          </LocalizationProvider>
        </FormControl>

        <FormControl>
          <LocalizationProvider
            dateAdapter={AdapterDayjs}
            adapterLocale={"zh-cn"}
          >
            <DateTimePicker
              label="结束时间"
              name="end_timestamp"
              disabled={keywordActive}
              ampm={false}
              value={
                filterName.end_timestamp === 0
                  ? null
                  : dayjs.unix(filterName.end_timestamp)
              }
              onChange={(value) => {
                if (value === null) {
                  handleFilterName({
                    target: { name: "end_timestamp", value: 0 },
                  });
                  return;
                }
                handleFilterName({
                  target: { name: "end_timestamp", value: value.unix() },
                });
              }}
              slotProps={{
                actionBar: {
                  actions: ["clear", "today", "accept"],
                },
              }}
            />
          </LocalizationProvider>
        </FormControl>
      </Stack>

      <Stack
        direction={{ xs: "column", sm: "row" }}
        spacing={{ xs: 3, sm: 2, md: 4 }}
        padding={"24px"}
      >
        {userIsAdmin && (
          <FormControl>
            <InputLabel htmlFor="channel-channel-label">渠道ID</InputLabel>
            <OutlinedInput
              id="channel"
              name="channel"
              disabled={keywordActive}
              sx={{
                minWidth: "100%",
              }}
              label="渠道ID"
              value={filterName.channel}
              onChange={handleFilterName}
              placeholder="渠道ID"
              startAdornment={
                <InputAdornment position="start">
                  <IconSitemap stroke={1.5} size="20px" color={grey500} />
                </InputAdornment>
              }
            />
          </FormControl>
        )}

        {userIsAdmin && (
          <FormControl sx={{ minWidth: "100%" }}>
            <Autocomplete
              freeSolo
              disabled={keywordActive}
              options={userOptions}
              getOptionLabel={(option) => typeof option === 'string' ? option : option.username}
              value={filterName.username}
              onInputChange={(_, newInputValue) => {
                searchUsers(newInputValue);
                handleFilterName({
                  target: { name: 'username', value: newInputValue }
                });
              }}
              onChange={(_, newValue) => {
                const username = typeof newValue === 'string' ? newValue : (newValue?.username || '');
                handleFilterName({
                  target: { name: 'username', value: username }
                });
              }}
              loading={userSearchLoading}
              renderInput={(params) => (
                <TextField
                  {...params}
                  label="用户名称"
                  placeholder="搜索用户名称"
                  InputProps={{
                    ...params.InputProps,
                    startAdornment: (
                      <InputAdornment position="start">
                        <IconUser stroke={1.5} size="20px" color={grey500} />
                      </InputAdornment>
                    ),
                  }}
                />
              )}
              renderOption={(props, option) => (
                <li {...props}>
                  <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}>
                    <div style={{ fontWeight: 'bold' }}>
                      {option.display_name || option.username}
                    </div>
                    <div style={{ fontSize: '0.9em', color: '#666' }}>
                      @{option.username} • ID: {option.id}
                    </div>
                  </div>
                </li>
              )}
              noOptionsText="未找到用户"
            />
          </FormControl>
        )}

        <FormControl sx={{ minWidth: "22%" }}>
          <InputLabel htmlFor="channel-type-label">类型</InputLabel>
          <Select
            id="channel-type-label"
            label="类型"
            disabled={keywordActive}
            value={filterName.type}
            name="type"
            onChange={handleFilterName}
            sx={{
              minWidth: "100%",
            }}
            MenuProps={{
              PaperProps: {
                style: {
                  maxHeight: 200,
                },
              },
            }}
          >
            {Object.values(LogType).map((option) => {
              return (
                <MenuItem key={option.value} value={option.value}>
                  {option.text}
                </MenuItem>
              );
            })}
          </Select>
        </FormControl>
      </Stack>
    </>
  );
}

TableToolBar.propTypes = {
  filterName: PropTypes.object,
  handleFilterName: PropTypes.func,
  userIsAdmin: PropTypes.bool,
};
