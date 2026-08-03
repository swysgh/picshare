(function () {
  'use strict';

  const zh = {
    albums: '相册',
    upload: '上传',
    new_album: '新建相册',
    delete: '删除',
    rename: '重命名',
    set_cover: '设为封面',
    selected: '已选 {n} 项',
    confirm_delete: '确认删除？此操作不可恢复。',
    confirm_delete_selected: '确认删除所选 {n} 项？',
    album_name: '相册名称',
    cancel: '取消',
    create: '创建',
    save: '保存',
    new_name: '新名称',
    home: '首页',
    loading: '加载中...',
    empty: '暂无内容',
    dropping: '松开鼠标上传',
    upload_progress: '上传中 {p}%',
    upload_done: '上传完成',
    upload_failed: '上传失败',
    deleted: '已删除',
    created: '已创建',
    cover_set: '封面已设置',
    renamed: '已重命名',
    error: '出错了',
    ok: '确定',
    fill_name: '请填写名称',
    select_at_least_one: '请先选择要删除的项',
    dragging_hint: '点击或拖拽图片到此处上传',
    enter_album: '进入相册',
    cover_badge: '封面',
    photos: '张照片',
    view_public: '前台预览',
    logout: '退出',
    description: '描述',
    description_help: '在此相册目录下创建 description.txt 可显示描述',
    no_cover: '暂无封面',
  };

  function t(key, params) {
    let s = (zh[key] || key);
    if (params) s = s.replace(/\{(\w+)\}/g, (_, k) => (params[k] !== undefined ? params[k] : ''));
    return s;
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
  }

  function toast(msg, type) {
    const el = document.getElementById('toast');
    el.textContent = msg;
    el.className = 'toast show ' + (type || '');
    setTimeout(() => el.classList.remove('show'), 2000);
  }

  function modal({ title, fields, onSubmit, submitLabel }) {
    const backdrop = document.getElementById('modal-backdrop');
    backdrop.innerHTML = `
      <div class="modal">
        <h3>${escapeHtml(title)}</h3>
        <form id="modal-form">
          ${fields.map(f => `
            <div class="field">
              <label>${escapeHtml(f.label)}</label>
              ${f.type === 'textarea'
                ? `<textarea name="${f.name}" rows="4">${escapeHtml(f.value || '')}</textarea>`
                : `<input type="text" name="${f.name}" value="${escapeHtml(f.value || '')}" autocomplete="off">`}
            </div>
          `).join('')}
          <div class="modal-actions">
            <button type="button" class="cancel-btn">${t('cancel')}</button>
            <button type="submit" class="primary">${escapeHtml(submitLabel || t('save'))}</button>
          </div>
        </form>
      </div>
    `;
    backdrop.classList.add('show');
    const form = backdrop.querySelector('form');
    const cancel = backdrop.querySelector('.cancel-btn');
    const firstInput = form.querySelector('input,textarea');
    if (firstInput) firstInput.focus();

    const close = () => backdrop.classList.remove('show');
    cancel.onclick = close;
    backdrop.onclick = e => { if (e.target === backdrop) close(); };

    form.onsubmit = e => {
      e.preventDefault();
      const data = {};
      new FormData(form).forEach((v, k) => data[k] = v);
      close();
      onSubmit(data);
    };
  }

  async function api(url, opts) {
    const r = await fetch(url, {
      ...opts,
      headers: { 'Accept': 'application/json', ...(opts && opts.headers || {}) }
    });
    if (r.status === 401) {
      toast(t('please_login'), 'error');
      throw new Error('unauthorized');
    }
    if (!r.ok) {
      let msg = 'HTTP ' + r.status;
      try { const j = await r.json(); if (j.error) msg = j.error; } catch {}
      throw new Error(msg);
    }
    return r.json();
  }

  function getAlbumSlug() {
    const m = location.pathname.match(/^\/admin\/album\/([^/]+)/);
    return m ? decodeURIComponent(m[1]) : null;
  }

  function getBreadcrumb(currentAlbum) {
    const home = `<a href="/admin/">${t('home')}</a>`;
    if (currentAlbum) {
      return `${home} / <span>${escapeHtml(currentAlbum)}</span>`;
    }
    return `${home}`;
  }

  function setProgress(p) {
    const bar = document.getElementById('progress');
    bar.style.width = (p * 100) + '%';
    if (p >= 1) setTimeout(() => bar.style.width = '0%', 300);
  }

  function uploadFiles(album, files) {
    return new Promise((resolve, reject) => {
      const fd = new FormData();
      for (const f of files) fd.append('files', f, f.name);
      const xhr = new XMLHttpRequest();
      xhr.open('POST', `/api/admin/upload?album=${encodeURIComponent(album)}`);
      xhr.setRequestHeader('Accept', 'application/json');
      xhr.upload.onprogress = e => {
        if (e.lengthComputable) setProgress(e.loaded / e.total);
      };
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try { resolve(JSON.parse(xhr.responseText)); }
          catch { resolve({}); }
        } else {
          reject(new Error('HTTP ' + xhr.status));
        }
      };
      xhr.onerror = () => reject(new Error('network error'));
      xhr.send(fd);
    });
  }

  function initHome() {
    const grid = document.getElementById('grid');
    api('/api/admin/manage').then(data => {
      document.getElementById('breadcrumb').innerHTML = getBreadcrumb();
      const albums = (data && Array.isArray(data.albums)) ? data.albums : [];
      if (!albums.length) {
        grid.innerHTML = `<div class="empty">${t('empty')}</div>`;
        return;
      }
      grid.innerHTML = albums.map(a => `
        <div class="album-card" data-album="${escapeHtml(a.name)}">
          <div class="cover">
            ${a.cover_thumb
              ? `<img src="${a.cover_thumb}" alt="" loading="lazy">`
              : `<div class="placeholder" style="color:#999;font-size:13px">${t('no_cover')}</div>`}
          </div>
          <div class="meta">
            <h3>${escapeHtml(a.name)}</h3>
            <div class="count">${a.photo_count} ${t('photos')}</div>
          </div>
          <div class="row-actions">
            <button data-action="rename">${t('rename')}</button>
            <button data-action="delete">${t('delete')}</button>
          </div>
        </div>
      `).join('');
      wireAlbumClicks();
    }).catch(e => {
      grid.innerHTML = `<div class="empty">${t('error')}: ${e.message}</div>`;
    });
  }

  function wireAlbumClicks() {
    document.querySelectorAll('.album-card').forEach(card => {
      const album = card.dataset.album;
      card.addEventListener('click', e => {
        const action = e.target.dataset.action;
        if (action === 'delete') {
          e.stopPropagation();
          if (confirm(t('confirm_delete'))) {
            api('/api/admin/delete', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ paths: ['.'], album })
            }).then(() => {
              toast(t('deleted'), 'success');
              initHome();
            }).catch(e => toast(e.message, 'error'));
          }
        } else if (action === 'rename') {
          e.stopPropagation();
          modal({
            title: t('rename') + ': ' + album,
            fields: [{ name: 'name', label: t('new_name'), value: album }],
            onSubmit: data => {
              if (!data.name.trim()) return toast(t('fill_name'), 'error');
              api('/api/admin/rename', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ album, from: album, to: data.name.trim() })
              }).then(() => {
                toast(t('renamed'), 'success');
                initHome();
              }).catch(e => toast(e.message, 'error'));
            }
          });
        } else {
          location.href = `/admin/album/${encodeURIComponent(album)}`;
        }
      });
    });
  }

  function initAlbum() {
    const album = getAlbumSlug();
    if (!album) return;
    document.getElementById('breadcrumb').innerHTML = getBreadcrumb(album);

    const uploadBtn = document.getElementById('upload-btn');
    if (uploadBtn) {
      uploadBtn.style.display = '';
      uploadBtn.onclick = () => document.getElementById('file-input').click();
    }

    const grid = document.getElementById('grid');
    api(`/api/admin/manage/${encodeURIComponent(album)}`).then(data => {
      const a = data.album;
      document.getElementById('album-title').textContent = a.name;
      document.getElementById('album-desc').textContent = a.description || '';
      const photos = Array.isArray(a.photos) ? a.photos : [];
      if (!photos.length) {
        grid.innerHTML = `<div class="empty">${t('empty')}</div>`;
        return;
      }
      grid.innerHTML = photos.map(p => `
        <div class="photo-tile" data-name="${escapeHtml(p.name)}" data-cover="${p.is_cover}">
          <img src="${p.thumb_url}" alt="" loading="lazy">
          ${p.is_cover ? `<div class="badge">${t('cover_badge')}</div>` : ''}
          <div class="check">✓</div>
          <div class="row-actions">
            <button data-action="setcover">${t('set_cover')}</button>
            <button data-action="rename">${t('rename')}</button>
            <button data-action="delete">${t('delete')}</button>
          </div>
        </div>
      `).join('');
      wirePhotoClicks(album);
    }).catch(e => {
      grid.innerHTML = `<div class="empty">${t('error')}: ${e.message}</div>`;
    });

    wireDropZone(album);
    wireUpload(album);

    const zone = document.getElementById('drop-zone');
    if (zone) zone.style.display = '';
  }

  function wirePhotoClicks(album) {
    const tiles = document.querySelectorAll('.photo-tile');
    tiles.forEach(tile => {
      const name = tile.dataset.name;
      tile.addEventListener('click', e => {
        const action = e.target.dataset.action;
        if (action === 'setcover') {
          e.stopPropagation();
          api('/api/admin/setcover', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ album, photo: name })
          }).then(() => {
            toast(t('cover_set'), 'success');
            initAlbum();
          }).catch(e => toast(e.message, 'error'));
        } else if (action === 'rename') {
          e.stopPropagation();
          modal({
            title: t('rename') + ': ' + name,
            fields: [{ name: 'name', label: t('new_name'), value: name }],
            onSubmit: data => {
              if (!data.name.trim()) return toast(t('fill_name'), 'error');
              api('/api/admin/rename', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ album, from: name, to: data.name.trim() })
              }).then(() => {
                toast(t('renamed'), 'success');
                initAlbum();
              }).catch(e => toast(e.message, 'error'));
            }
          });
        } else if (action === 'delete') {
          e.stopPropagation();
          if (confirm(t('confirm_delete'))) {
            api('/api/admin/delete', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ paths: [name], album })
            }).then(() => {
              toast(t('deleted'), 'success');
              initAlbum();
            }).catch(e => toast(e.message, 'error'));
          }
        } else {
          tile.classList.toggle('selected');
          updateSelectionBar();
        }
      });
    });
  }

  function updateSelectionBar() {
    const selected = document.querySelectorAll('.photo-tile.selected');
    const bar = document.getElementById('selection-bar');
    if (selected.length === 0) {
      bar.classList.remove('show');
      return;
    }
    bar.classList.add('show');
    bar.querySelector('.count').textContent = t('selected', { n: selected.length });
  }

  function wireDropZone(album) {
    const zone = document.getElementById('drop-zone');
    if (!zone) return;
    zone.addEventListener('click', () => document.getElementById('file-input').click());
    zone.addEventListener('dragover', e => {
      e.preventDefault();
      zone.classList.add('dragover');
    });
    zone.addEventListener('dragleave', () => zone.classList.remove('dragover'));
    zone.addEventListener('drop', e => {
      e.preventDefault();
      zone.classList.remove('dragover');
      const files = Array.from(e.dataTransfer.files).filter(f => /\.(jpe?g|png|webp|gif|bmp|tiff?)$/i.test(f.name));
      if (files.length) doUpload(album, files);
    });
  }

  function wireUpload(album) {
    const input = document.getElementById('file-input');
    if (!input) return;
    input.addEventListener('change', () => {
      const files = Array.from(input.files);
      if (files.length) doUpload(album, files);
      input.value = '';
    });
  }

  function doUpload(album, files) {
    uploadFiles(album, files).then(r => {
      toast(t('upload_done') + ' (' + r.uploaded + ')', 'success');
      initAlbum();
    }).catch(e => toast(t('upload_failed') + ': ' + e.message, 'error'));
  }

  function initGlobal() {
    const newBtn = document.getElementById('new-album-btn');
    if (newBtn) {
      newBtn.addEventListener('click', () => {
        modal({
          title: t('new_album'),
          fields: [{ name: 'name', label: t('album_name') }],
          submitLabel: t('create'),
          onSubmit: data => {
            if (!data.name.trim()) return toast(t('fill_name'), 'error');
            api('/api/admin/mkdir', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ album: data.name.trim() })
            }).then(() => {
              toast(t('created'), 'success');
              initHome();
            }).catch(e => toast(e.message, 'error'));
          }
        });
      });
    }

    const deleteSel = document.getElementById('delete-selected');
    if (deleteSel) {
      deleteSel.addEventListener('click', () => {
        const selected = Array.from(document.querySelectorAll('.photo-tile.selected')).map(t => t.dataset.name);
        if (!selected.length) return toast(t('select_at_least_one'), 'error');
        if (!confirm(t('confirm_delete_selected', { n: selected.length }))) return;
        const album = getAlbumSlug();
        api('/api/admin/delete', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ paths: selected, album })
        }).then(() => {
          toast(t('deleted'), 'success');
          initAlbum();
        }).catch(e => toast(e.message, 'error'));
      });
    }

    const clearSel = document.getElementById('clear-selection');
    if (clearSel) {
      clearSel.addEventListener('click', () => {
        document.querySelectorAll('.photo-tile.selected').forEach(t => t.classList.remove('selected'));
        updateSelectionBar();
      });
    }

    const viewBtn = document.getElementById('view-public');
    if (viewBtn) {
      viewBtn.addEventListener('click', () => {
        window.open('/', '_blank');
      });
    }
  }

  document.addEventListener('DOMContentLoaded', () => {
    initGlobal();
    if (getAlbumSlug()) initAlbum();
    else initHome();
  });
})();
