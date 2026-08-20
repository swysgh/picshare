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

  function getAlbumPath() {
    const m = location.pathname.match(/^\/album\/(.+)/);
    if (!m) return null;
    try {
      return decodeURIComponent(m[1]);
    } catch {
      return null;
    }
  }

  function getPageType() {
    if (location.pathname.startsWith('/album/')) return 'album';
    if (location.pathname.startsWith('/admin')) return 'admin';
    return 'home';
  }

  function renderCard(a) {
    const hasChildren = Array.isArray(a.children) && a.children.length > 0;
    const count = hasChildren
      ? `${t('subfolders', { n: a.children.length })} · ${t('photo_total', { n: a.photo_count })}`
      : t('photo_total', { n: a.photo_count });
    return `
      <a class="album-card${hasChildren ? ' folder' : ''}" href="/album/${encodeURIComponent(a.path)}">
        <div class="cover">
          ${a.cover_thumb
            ? `<img src="${a.cover_thumb}" alt="${escapeHtml(a.name)}" loading="lazy">`
            : `<div class="placeholder">${hasChildren ? t('folder') : t('no_cover')}</div>`}
        </div>
        <div class="meta">
          <h3>${escapeHtml(a.name)}</h3>
          <div class="count">${count}</div>
          ${a.description ? `<div class="desc">${escapeHtml(a.description)}</div>` : ''}
        </div>
      </a>
    `;
  }

  function buildBreadcrumb(path) {
    const segs = path.split('/');
    let acc = '';
    let html = `<a href="/">${t('breadcrumb_home')}</a>`;
    segs.forEach((seg, i) => {
      acc = acc ? acc + '/' + seg : seg;
      html += ' / ';
      if (i === segs.length - 1) {
        html += `<span>${escapeHtml(seg)}</span>`;
      } else {
        html += `<a href="/album/${encodeURIComponent(acc)}">${escapeHtml(seg)}</a>`;
      }
    });
    return html;
  }

  function renderPhotos(photos) {
    const grid = document.getElementById('photo-grid');
    if (!photos.length) return false;
    grid.innerHTML = photos.map(p => `
      <a href="${p.url}" data-pswp-width="${p.width || 1600}" data-pswp-height="${p.height || 1067}" target="_blank">
        <img src="${p.thumb_url}" alt="${escapeHtml(p.name)}" loading="lazy">
      </a>
    `).join('');
    initPhotoSwipe();
    return true;
  }

  function initHome() {
    const listEl = document.getElementById('album-list');
    const crumb = document.getElementById('breadcrumb');
    fetchJSON('/api/albums').then(albums => {
      const list = Array.isArray(albums) ? albums : [];
      crumb.innerHTML = '';
      if (!list.length) {
        listEl.innerHTML = `<div class="empty">${t('empty')}</div>`;
        return;
      }
      listEl.innerHTML = list.map(renderCard).join('');
    }).catch(e => {
      listEl.innerHTML = `<div class="empty">${t('error')}: ${escapeHtml(e.message)}</div>`;
    });
  }

  function initAlbum() {
    const path = getAlbumPath();
    if (!path) return;
    const listEl = document.getElementById('album-list');
    const grid = document.getElementById('photo-grid');
    const crumb = document.getElementById('breadcrumb');
    const title = document.getElementById('album-title');
    const desc = document.getElementById('album-desc');

    fetchJSON(`/api/albums/${encodeURIComponent(path)}`).then(a => {
      if (!a || !a.name) {
        grid.innerHTML = `<div class="empty">${t('not_found')}</div>`;
        return;
      }
      document.title = `${a.name} - ${t('site_title')}`;
      title.textContent = a.name;
      crumb.innerHTML = buildBreadcrumb(a.path || path);
      if (a.description) desc.textContent = a.description;

      const children = Array.isArray(a.children) ? a.children : [];
      const photos = Array.isArray(a.photos) ? a.photos : [];

      listEl.innerHTML = children.length ? children.map(renderCard).join('') : '';

      if (photos.length) {
        renderPhotos(photos);
      } else if (!children.length) {
        grid.innerHTML = `<div class="empty">${t('album_empty')}</div>`;
      } else {
        grid.innerHTML = '';
      }
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