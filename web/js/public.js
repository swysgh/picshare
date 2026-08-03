(function () {
  'use strict';

  const { t, getLang, setLang } = window.i18n;

  function initLangSwitch() {
    const lang = getLang();
    document.querySelectorAll('.lang-switch button').forEach(btn => {
      btn.classList.toggle('active', btn.dataset.lang === lang);
      btn.addEventListener('click', () => {
        setLang(btn.dataset.lang);
        location.reload();
      });
    });
  }

  function renderLangSwitch() {
    const html = `
      <div class="lang-switch">
        <button data-lang="zh">${t('language_zh')}</button>
        <button data-lang="ko">${t('language_ko')}</button>
        <button data-lang="en">${t('language_en')}</button>
      </div>
    `;
    const el = document.getElementById('lang-switch');
    if (el) el.innerHTML = html;
  }

  async function fetchJSON(url) {
    const r = await fetch(url);
    if (!r.ok) throw new Error('HTTP ' + r.status);
    return r.json();
  }

  function getAlbumSlug() {
    const m = location.pathname.match(/^\/album\/([^/]+)/);
    return m ? decodeURIComponent(m[1]) : null;
  }

  function getPageType() {
    if (location.pathname.startsWith('/album/')) return 'album';
    if (location.pathname.startsWith('/admin')) return 'admin';
    return 'home';
  }

  function initHome() {
    const listEl = document.getElementById('album-list');
    fetchJSON('/api/albums').then(albums => {
      const list = Array.isArray(albums) ? albums : [];
      if (!list.length) {
        listEl.innerHTML = `<div class="empty">${t('empty')}</div>`;
        return;
      }
      listEl.innerHTML = list.map(a => `
        <a class="album-card" href="/album/${encodeURIComponent(a.name)}">
          <div class="cover">
            ${a.cover_thumb
              ? `<img src="${a.cover_thumb}" alt="${escapeHtml(a.name)}" loading="lazy">`
              : `<div class="placeholder">${t('no_cover')}</div>`}
          </div>
          <div class="meta">
            <h3>${escapeHtml(a.name)}</h3>
            <div class="count">${t('photo_total', { n: a.photo_count })}</div>
            ${a.description ? `<div class="desc">${escapeHtml(a.description)}</div>` : ''}
          </div>
        </a>
      `).join('');
    }).catch(e => {
      listEl.innerHTML = `<div class="empty">${t('error')}: ${escapeHtml(e.message)}</div>`;
    });
  }

  function initAlbum() {
    const slug = getAlbumSlug();
    if (!slug) return;
    const grid = document.getElementById('photo-grid');
    const crumb = document.getElementById('breadcrumb');
    const title = document.getElementById('album-title');
    const desc = document.getElementById('album-desc');

    fetchJSON(`/api/albums/${encodeURIComponent(slug)}`).then(a => {
      if (!a || !a.name) {
        grid.innerHTML = `<div class="empty">${t('not_found')}</div>`;
        return;
      }
      document.title = `${a.name} - ${t('site_title')}`;
      title.textContent = a.name;
      crumb.innerHTML = `
        <a href="/">${t('breadcrumb_home')}</a> / <span>${escapeHtml(a.name)}</span>
      `;
      if (a.description) desc.textContent = a.description;
      const photos = a.photos || [];
      if (!photos.length) {
        grid.innerHTML = `<div class="empty">${t('album_empty')}</div>`;
        return;
      }
      grid.innerHTML = photos.map(p => `
        <a href="${p.url}" data-pswp-width="${p.width || 1600}" data-pswp-height="${p.height || 1067}" target="_blank">
          <img src="${p.thumb_url}" alt="${escapeHtml(p.name)}" loading="lazy">
        </a>
      `).join('');
      initPhotoSwipe();
    }).catch(e => {
      grid.innerHTML = `<div class="empty">${t('error')}: ${escapeHtml(e.message)}</div>`;
    });
  }

  function initPhotoSwipe() {
    if (typeof PhotoSwipeLightbox === 'undefined') return;
    const lightbox = new PhotoSwipeLightbox({
      gallery: '#photo-grid',
      children: 'a',
      pswpModule: window.PhotoSwipe,
      bgOpacity: 0.95,
      showHideOpacity: true
    });
    lightbox.init();
  }

  function openLightbox(startIndex) {
    if (typeof PhotoSwipe === 'undefined') return;
    const links = document.querySelectorAll('#photo-grid a');
    const items = Array.from(links).map(a => {
      const img = a.querySelector('img');
      return {
        src: a.getAttribute('href'),
        msrc: img.getAttribute('src'),
        width: parseInt(a.dataset.pswpWidth) || 1600,
        height: parseInt(a.dataset.pswpHeight) || 1067,
        alt: img.getAttribute('alt') || ''
      };
    });
    const pswp = document.querySelector('.pswp');
    if (!pswp) return;
    const g = new PhotoSwipe({
      dataSource: items,
      index: startIndex,
      bgOpacity: 0.95,
      showHideOpacity: true,
      pswpModule: PhotoSwipe
    });
    g.init();
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
  }

  document.addEventListener('DOMContentLoaded', () => {
    renderLangSwitch();
    initLangSwitch();
    if (getPageType() === 'home') initHome();
    if (getPageType() === 'album') initAlbum();
  });
})();
