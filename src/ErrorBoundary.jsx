import React from 'react';
import { reset } from './services/storage.js';

// Jaring pengaman: bila ada error saat render, tampilkan pesan + tombol pemulihan, bukan layar putih kosong.
// "Reset tampilan" menghapus state tampilan tersimpan (localStorage convera:v1) lalu memuat ulang; login tetap terjaga.
export default class ErrorBoundary extends React.Component {
  state = { error: null };
  static getDerivedStateFromError(error) { return { error }; }
  componentDidCatch(error, info) { console.error('Convera render error:', error, info?.componentStack); }
  render() {
    if (!this.state.error) return this.props.children;
    const btn = { height: 42, padding: '0 20px', border: 'none', borderRadius: 10, fontFamily: 'inherit', fontSize: 14, fontWeight: 600, cursor: 'pointer' };
    return (
      <div role="alert" style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20, background: '#faf8f5', color: '#1d1c1a', fontFamily: "'Poppins',sans-serif" }}>
        <div style={{ maxWidth: 420, textAlign: 'center', display: 'flex', flexDirection: 'column', gap: 14 }}>
          <h1 style={{ margin: 0, fontSize: 20 }}>Halaman tidak bisa ditampilkan</h1>
          <p style={{ margin: 0, fontSize: 14, color: '#6b6761' }}>Terjadi kesalahan saat menampilkan aplikasi. Data Anda aman. Coba muat ulang; bila masih sama, reset tampilan.</p>
          <div style={{ display: 'flex', gap: 10, justifyContent: 'center', flexWrap: 'wrap' }}>
            <button type="button" onClick={() => location.reload()} style={{ ...btn, background: '#e8590c', color: '#fff' }}>Muat ulang</button>
            <button type="button" onClick={reset} style={{ ...btn, background: '#ece8e1', color: '#1d1c1a' }}>Reset tampilan</button>
          </div>
        </div>
      </div>
    );
  }
}
