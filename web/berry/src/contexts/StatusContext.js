import { showError as reportUIError } from '../utils/common';
import { useEffect, useCallback, createContext } from "react";
import { API } from "utils/api";
import { showNotice, showError } from "utils/common";
import { SET_SITE_INFO } from "store/actions";
import { useDispatch } from "react-redux";

export const LoadStatusContext = createContext();

// eslint-disable-next-line
const StatusProvider = ({ children }) => {
  const dispatch = useDispatch();

  const loadStatus = useCallback(async () => {
    const res = await API.get("/api/status");
    const { success, data } = res.data;
    let system_name = "";
    if (success) {
      const previousSiteInfoRaw = localStorage.getItem("siteInfo");
      let previousVersion = "";
      if (previousSiteInfoRaw) {
        try {
          previousVersion = JSON.parse(previousSiteInfoRaw).version || "";
        } catch (error) {
          console.warn("Failed to parse cached site info:", error);
        }
      }

      if (!data.chat_link) {
        delete data.chat_link;
      }
      localStorage.setItem("siteInfo", JSON.stringify(data));
      localStorage.setItem("quota_per_unit", data.quota_per_unit);
      localStorage.setItem("display_in_currency", data.display_in_currency);
      dispatch({ type: SET_SITE_INFO, payload: data });
      const backendVersion = data.version || "";
      if (
        backendVersion !== "" &&
        backendVersion !== "v0.0.0" &&
        backendVersion !== "0.0.0" &&
        previousVersion !== "" &&
        previousVersion !== backendVersion
      ) {
        showNotice(
          `新版本可用：${backendVersion}，请使用快捷键 Shift + F5 刷新页面`
        );
      }
      if (data.system_name) {
        system_name = data.system_name;
      }
    } else {
      const backupSiteInfo = localStorage.getItem("siteInfo");
      if (backupSiteInfo) {
        const data = JSON.parse(backupSiteInfo);
        if (data.system_name) {
          system_name = data.system_name;
        }
        dispatch({
          type: SET_SITE_INFO,
          payload: data,
        });
      }
      showError("无法正常连接至服务器！");
    }

    if (system_name) {
      document.title = system_name;
    }
  }, [dispatch]);

  useEffect(() => {
    loadStatus().then().catch(reportUIError);
  }, [loadStatus]);

  return (
    <LoadStatusContext.Provider value={loadStatus}>
      {" "}
      {children}{" "}
    </LoadStatusContext.Provider>
  );
};

export default StatusProvider;
