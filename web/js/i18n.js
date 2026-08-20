(function (global) {
  'use strict';

  const dict = {
    zh: {
      site_title: '产品图库',
      albums: '相册',
      photos: '张照片',
      back: '← 返回',
      lang: '语言',
      empty: '暂无相册',
      album_empty: '此相册暂无图片',
      no_cover: '暂无封面',
      loading: '加载中...',
      error: '出错了',
      admin: '管理',
      admin_login: '请登录后台',
      admin_login_help: '首次访问后台会弹出浏览器登录框',
      upload: '上传',
      upload_help: '点击或拖拽图片到此处',
      uploading: '上传中...',
      new_album: '新建相册',
      album_name: '相册名',
      cancel: '取消',
      create: '创建',
      delete: '删除',
      rename: '重命名',
      set_cover: '设为封面',
      confirm_delete: '确认删除？此操作不可恢复。',
      confirm_delete_selected: '确认删除所选 {n} 项？',
      save: '保存',
      description: '描述',
      breadcrumb_home: '首页',
      view: '查看',
      selected: '已选 {n} 项',
      clear: '清空',
      rename_to: '新名称',
      cover_set: '封面已设置',
      deleted: '已删除',
      created: '已创建',
      upload_done: '上传完成',
      upload_failed: '上传失败',
      login_required: '请先登录',
      forbidden: '无权访问',
      not_found: '未找到',
      language_zh: '中文',
      language_ko: '한국어',
      language_en: 'English',
      photo_total: '共 {n} 张',
      folder: '文件夹',
      subfolders: '{n} 个子分类',
      cover_badge: '封面',
      hint_cover: '提示：把图片命名为 cover.jpg 可作为封面',
      hint_desc: '提示：在相册目录放 description.txt 可显示描述',
      hint_sort: '提示：目录名前缀数字可控制排序，如 01.主力产品'
    },
    ko: {
      site_title: '제품 갤러리',
      albums: '앨범',
      photos: '장',
      back: '← 뒤로',
      lang: '언어',
      empty: '앨범이 없습니다',
      album_empty: '이 앨범에 사진이 없습니다',
      no_cover: '표지가 없습니다',
      loading: '로딩 중...',
      error: '오류가 발생했습니다',
      admin: '관리',
      admin_login: '관리자 로그인',
      admin_login_help: '처음 접속 시 브라우저 로그인 창이 표시됩니다',
      upload: '업로드',
      upload_help: '클릭하거나 이미지를 여기로 드래그하세요',
      uploading: '업로드 중...',
      new_album: '새 앨범',
      album_name: '앨범 이름',
      cancel: '취소',
      create: '생성',
      delete: '삭제',
      rename: '이름 변경',
      set_cover: '표지로 설정',
      confirm_delete: '삭제하시겠습니까? 이 작업은 되돌릴 수 없습니다.',
      confirm_delete_selected: '선택한 {n}개 항목을 삭제하시겠습니까?',
      save: '저장',
      description: '설명',
      breadcrumb_home: '홈',
      view: '보기',
      selected: '{n}개 선택됨',
      clear: '선택 해제',
      rename_to: '새 이름',
      cover_set: '표지가 설정되었습니다',
      deleted: '삭제되었습니다',
      created: '생성되었습니다',
      upload_done: '업로드 완료',
      upload_failed: '업로드 실패',
      login_required: '로그인이 필요합니다',
      forbidden: '권한이 없습니다',
      not_found: '찾을 수 없습니다',
      language_zh: '中文',
      language_ko: '한국어',
      language_en: 'English',
      photo_total: '총 {n}장',
      folder: '폴더',
      subfolders: '하위 {n}개',
      cover_badge: '표지',
      hint_cover: '팁: 이미지 이름을 cover.jpg로 하면 표지가 됩니다',
      hint_desc: '팁: 앨범 디렉토리에 description.txt를 두면 설명이 표시됩니다',
      hint_sort: '팁: 디렉토리 이름 앞에 숫자를 붙이면 정렬됩니다 (예: 01.주력제품)'
    },
    en: {
      site_title: 'Product Gallery',
      albums: 'Albums',
      photos: 'photos',
      back: '← Back',
      lang: 'Language',
      empty: 'No albums yet',
      album_empty: 'This album has no photos',
      no_cover: 'No cover',
      loading: 'Loading...',
      error: 'Something went wrong',
      admin: 'Admin',
      admin_login: 'Admin Login',
      admin_login_help: 'A browser login dialog will appear on first access',
      upload: 'Upload',
      upload_help: 'Click or drag images here',
      uploading: 'Uploading...',
      new_album: 'New Album',
      album_name: 'Album name',
      cancel: 'Cancel',
      create: 'Create',
      delete: 'Delete',
      rename: 'Rename',
      set_cover: 'Set as cover',
      confirm_delete: 'Delete? This cannot be undone.',
      confirm_delete_selected: 'Delete selected {n} items?',
      save: 'Save',
      description: 'Description',
      breadcrumb_home: 'Home',
      view: 'View',
      selected: '{n} selected',
      clear: 'Clear',
      rename_to: 'New name',
      cover_set: 'Cover set',
      deleted: 'Deleted',
      created: 'Created',
      upload_done: 'Upload complete',
      upload_failed: 'Upload failed',
      login_required: 'Login required',
      forbidden: 'Forbidden',
      not_found: 'Not found',
      language_zh: '中文',
      language_ko: '한국어',
      language_en: 'English',
      photo_total: '{n} total',
      folder: 'Folder',
      subfolders: '{n} subfolders',
      cover_badge: 'Cover',
      hint_cover: 'Tip: name an image cover.jpg to use as the cover',
      hint_desc: 'Tip: put description.txt in an album directory to show a description',
      hint_sort: 'Tip: prefix a directory name with a number to sort, e.g. 01.Main'
    }
  };

  const STORAGE_KEY = 'picshare.lang';
  const COOKIE_KEY = 'picshare.lang';
  let memLang = null;

  function readCookieLang() {
    try {
      const m = document.cookie.match(new RegExp('(?:^|;\\s*)' + COOKIE_KEY + '=([^;]*)'));
      return m ? decodeURIComponent(m[1]) : null;
    } catch (_) {
      return null;
    }
  }

  function writeCookieLang(lang) {
    try {
      document.cookie = COOKIE_KEY + '=' + encodeURIComponent(lang) +
        '; path=/; max-age=31536000; SameSite=Lax';
    } catch (_) {}
  }

  function readStoredLang() {
    if (memLang && dict[memLang]) return memLang;
    try {
      const v = localStorage.getItem(STORAGE_KEY);
      if (v) return v;
    } catch (_) {}
    try {
      const v = sessionStorage.getItem(STORAGE_KEY);
      if (v) return v;
    } catch (_) {}
    return readCookieLang();
  }

  function writeStoredLang(lang) {
    memLang = lang;
    try { localStorage.setItem(STORAGE_KEY, lang); } catch (_) {}
    try { sessionStorage.setItem(STORAGE_KEY, lang); } catch (_) {}
    writeCookieLang(lang);
  }

  function detectLang() {
    const saved = readStoredLang();
    if (saved && dict[saved]) return saved;
    const def = (typeof window !== 'undefined' && window.PICSHARE_DEFAULT_LANG) || '';
    if (def && dict[def]) return def;
    const nav = (navigator.language || 'zh').toLowerCase();
    if (nav.startsWith('ko')) return 'ko';
    if (nav.startsWith('en')) return 'en';
    return 'zh';
  }

  function setLang(lang) {
    if (!dict[lang]) return;
    writeStoredLang(lang);
    applyLang(lang);
  }

  function getLang() {
    return readStoredLang() || detectLang();
  }

  function t(key, params) {
    const lang = getLang();
    const s = (dict[lang] && dict[lang][key]) || (dict.zh[key]) || key;
    if (!params) return s;
    return s.replace(/\{(\w+)\}/g, (_, k) => (params[k] !== undefined ? params[k] : ''));
  }

  function applyLang(lang) {
    document.documentElement.lang = lang;
    document.querySelectorAll('[data-i18n]').forEach(el => {
      el.textContent = t(el.getAttribute('data-i18n'));
    });
    document.querySelectorAll('[data-i18n-attr]').forEach(el => {
      const spec = el.getAttribute('data-i18n-attr');
      spec.split(';').forEach(pair => {
        const [attr, key] = pair.split(':').map(s => s.trim());
        if (attr && key) el.setAttribute(attr, t(key));
      });
    });
  }

  global.i18n = { t, setLang, getLang, applyLang };

  applyLang(getLang());
})(window);
