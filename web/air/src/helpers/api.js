import { reportUnexpectedRequestError } from '../../../legacy/browser-request-errors.mjs';
import { showError } from './utils';
import axios from 'axios';

export const API = axios.create({
  baseURL: process.env.REACT_APP_SERVER ? process.env.REACT_APP_SERVER : '',
});

API.interceptors.response.use(
  (response) => response,
  (error) => {
    if (axios.isAxiosError(error) && !error.response && error.request)
      reportUnexpectedRequestError(error);
    showError(error);
    return Promise.reject(error);
  },
);
