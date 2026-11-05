import { appConfig } from './config.js';
import { requestJson } from './api.js';

export const ENDPOINTS = Object.freeze({
  profile: '/api/v1/me/profile',
  session: '/api/v1/auth/session',
  login: '/api/v1/auth/login',
  logout: '/api/v1/auth/logout',
  workspaces: '/api/v1/workspaces',
  leads: '/api/v1/leads',
  events: '/api/v1/events',
  reports: '/api/v1/reports',
});

export async function updateProfile(profile) {
  if (!appConfig.backendEnabled) return profile;
  const response = await requestJson(ENDPOINTS.profile, { method: 'PATCH', body: JSON.stringify(profile) });
  return response?.data || response;
}
