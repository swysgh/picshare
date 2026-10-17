(function () {
  'use strict';

  const { t } = window.i18n;

  // Server-injected copy (landing_subtitle / landing_cta_text) wins; only
  // empty elements fall back to the i18n dictionary.
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

  fillFallbacks();
  window.i18n.renderLangSwitch();
  initEnterKey();
})();
