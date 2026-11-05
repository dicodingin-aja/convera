import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App.jsx';
import { appConfig } from './services/config.js';
import { takeParam } from './services/links.js';
import { verifyEmail, supportLogin } from './services/auth.js';
import '@fontsource/poppins/400.css';
import '@fontsource/poppins/500.css';
import '@fontsource/poppins/600.css';
import './styles.css';

// Props mirror the original design's tweakable options:
//   startScreen: 'app' | 'onboarding'   accent: 'orange' | 'indigo'
//   statusStyle: 'brand' | 'semantic'   dark: boolean
// Tautan khusus (mode backend), token dihapus dari URL begitu dibaca:
//   ?reset=<token>    membuka form password baru
//   ?verify=<token>   memverifikasi email
//   ?support=<kode>   sesi dukungan dari Convera Admin (admin membantu customer; kode sekali pakai)
const notice = (m) => { try { sessionStorage.setItem('convera:notice', m); } catch { /* abaikan */ } };
const start = () => createRoot(document.getElementById('root')).render(<App />);

if (appConfig.backendEnabled) {
  const reset = takeParam('reset');
  if (reset) { try { sessionStorage.setItem('convera:reset', reset); } catch { /* abaikan */ } }
  const verify = takeParam('verify');
  const support = takeParam('support');
  const jobs = [];
  if (verify) {
    jobs.push(verifyEmail(verify).then(() => notice('Email berhasil diverifikasi. Silakan masuk.')).catch((e) => notice(e.message || 'Tautan verifikasi tidak valid.')));
  }
  if (support) {
    jobs.push(supportLogin(support).catch((e) => notice(e.message || 'Tautan dukungan tidak valid.')));
  }
  Promise.all(jobs).finally(start);
} else start();
