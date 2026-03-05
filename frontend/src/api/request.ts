import axios from 'axios';
import type { AxiosInstance, AxiosResponse, InternalAxiosRequestConfig } from 'axios';
import { ElMessage } from 'element-plus';

const service: AxiosInstance = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 10000,
});

service.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    return config;
  },
  (error: any) => {
    return Promise.reject(error);
  }
);

service.interceptors.response.use(
  (response: AxiosResponse) => {
    const res = response.data;
    
    if (res && typeof res === 'object' && 'code' in res) {
      if (res.code !== 200) {
        console.error('API Error:', res.message);
        ElMessage({
          message: res.message || 'Error',
          type: 'error',
          duration: 5000
        });
        return Promise.reject(new Error(res.message || 'Error'));
      }
      return res.data;
    }
    
    return res;
  },
  (error: any) => {
    console.error('HTTP Error:', error.message);
    
    let message = error.message || 'Unknown Error';
    if (error.response) {
      if (error.response.status === 401) {
        message = 'Unauthorized, please login';
      } else if (error.response.data && error.response.data.message) {
        message = error.response.data.message;
      } else {
        message = `HTTP Error ${error.response.status}`;
      }
    }
    
    ElMessage({
      message: message,
      type: 'error',
      duration: 5000
    });
    
    return Promise.reject(error);
  }
);

export default service;
