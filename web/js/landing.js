(function () {
  'use strict';

  const { t } = window.i18n;

  // The server injects the whole per-language copy (landing_i18n, with the
  // legacy single-language fields already folded in) into
  // <main data-landing-texts="…">. A data attribute keeps this compatible
  // with the strict CSP: no inline script is needed to hand the JSON over.
  function readTexts() {
    const main = document.querySelector('[data-landing-texts]');
    if (!main) return null;
    const raw = main.getAttribute('data-landing-texts') || '';
    try {
      return JSON.parse(raw);
    } catch (_) {
      return null;
    }
  }

  function pick(texts, group, lang) {
    const g = texts && texts[group];
    if (!g || typeof g[lang] !== 'string') return '';
    return g[lang].trim();
  }

  // Overwrite the server-rendered default_lang copy with the visitor's
  // language. Values that are still empty fall through to the dictionary in
  // fillFallbacks().
  function applyTexts(texts) {
    if (!texts) return;
    const lang = window.i18n.getLang();
    const title = pick(texts, 'title', lang);
    if (title) {
      const h1 = document.getElementById('landing-title');
      if (h1) h1.textContent = title;
      document.title = title;
    }
    // Subtitle and CTA are written even when the value is empty: an empty
    // override for the current language must fall through to the dictionary,
    // not keep the default_language text the server rendered into the element.
    setText('landing-subtitle', pick(texts, 'subtitle', lang));
    setText('landing-cta', pick(texts, 'cta', lang));
  }

  function setText(id, value) {
    const el = document.getElementById(id);
    if (el) el.textContent = value;
  }

  // Server-injected copy wins; only empty elements fall back to the i18n
  // dictionary.
  function fillFallbacks() {
    document.querySelectorAll('[data-i18n-fallback]').forEach(el => {
      if (!el.textContent.trim()) {
        el.textContent = t(el.getAttribute('data-i18n-fallback'));
      }
    });
  }

  function initEnterKey() {
    document.addEventListener('keydown', e => {
      if (e.key !== 'Enter' || e.altKey || e.ctrlKey || e.metaKey) return;
      const el = document.activeElement;
      // Let a focused link/button handle Enter itself.
      if (el && (el.tagName === 'A' || el.tagName === 'BUTTON')) return;
      location.href = '/enter';
    });
  }

  // The whole page is the call to action (there is no button). Clicks on the
  // language switcher must only switch the language, so they are skipped.
  function initWholePageClick() {
    document.addEventListener('click', e => {
      if (e.target.closest('#lang-switch')) return;
      location.href = '/enter';
    });
  }

  applyTexts(readTexts());
  fillFallbacks();
  window.i18n.renderLangSwitch();
  initEnterKey();
  initWholePageClick();
})();
