// Lightweight i18n loader and applier
// Usage: include before page scripts. Mark elements with data-t (textContent) and/or
// data-t-title, data-t-placeholder, data-t-aria-label, data-t-value, data-t-html.
// In JS, call t('key') to get a string. Arrays supported via t('key', null, true).

(function(){
  const I18N_NS = '/web/i18n/';
  const FALLBACK = 'en';

  const store = {
    lang: 'auto',
    dict: {},
    ready: false,
    listeners: [],
  };

  function getSavedLang(){
    try{ const v = localStorage.getItem('lang'); if(v) return v; }catch{}
    return 'auto';
  }

  function detectLang(){
    const saved = getSavedLang();
    if(saved && saved !== 'auto') return saved;
    const nav = (navigator.language || navigator.userLanguage || 'en').toLowerCase();
    if(nav.startsWith('ru')) return 'ru';
    return 'en';
  }

  function deepGet(obj, path){
    if(!obj) return undefined;
    const parts = path.split('.');
    let cur = obj;
    for(const p of parts){
      if(cur && Object.prototype.hasOwnProperty.call(cur, p)) cur = cur[p]; else return undefined;
    }
    return cur;
  }

  function format(str, params){
    if(!params) return String(str);
    return String(str).replace(/\{(\w+)\}/g, (m, k) => (k in params) ? String(params[k]) : m);
  }

  function setLang(lang){
    try{ localStorage.setItem('lang', lang); }catch{}
  // Important: call applyAll() without passing the loaded dict as an argument,
  // otherwise applyAll receives an object and fails with
  // "scope.querySelectorAll is not a function".
  return load(lang).then(() => { applyAll(); });
  }

  async function fetchJSON(url){
    const res = await fetch(url, {cache:'no-store'});
    if(!res.ok) throw new Error('i18n load failed '+res.status);
    return res.json();
  }

  async function load(lang){
    let L = (lang||detectLang()).toLowerCase();
    if(L === 'auto') {
      L = detectLang();
    }
    let dict = {};
    try{ dict = await fetchJSON(I18N_NS + L + '.json'); }
    catch{
      if(L !== FALLBACK){ try{ dict = await fetchJSON(I18N_NS + FALLBACK + '.json'); } catch{} }
    }
    store.lang = L; store.dict = dict || {}; store.ready = true;
    store.listeners.forEach(fn => { try{ fn(); }catch{} });
    return store.dict;
  }

  function t(key, params, asIs){
    // asIs=true: return non-string (arrays/objects) as-is
    const v = deepGet(store.dict, key);
    if(v === undefined || v === null){ return asIs ? undefined : key; }
    if(asIs){ return v; }
    if(typeof v === 'string') return format(v, params);
    return String(v);
  }

  function applyTo(el){
    if(!el) return;
    const set = (attr, key) => {
      const val = t(key);
      if(attr === 'text') el.textContent = val;
      else if(attr === 'html') el.innerHTML = val;
      else el.setAttribute(attr, val);
    };
    // data-t="key" => textContent
    const key = el.getAttribute('data-t'); if(key) set('text', key);
    // attribute mappings
    for(const a of ['title','placeholder','aria-label','value']){
      const k = el.getAttribute('data-t-' + a);
      if(k) set(a, k);
    }
    // data-t-html allows simple HTML (use sparingly)
    const kh = el.getAttribute('data-t-html'); if(kh) set('html', kh);
    // for <option data-t-option="key"> preserve value
    const ko = el.getAttribute('data-t-option'); if(ko){ el.textContent = t(ko); }
  }

  function applyAll(root){
    const scope = root || document;
    // elements with any data-t*
    const nodes = scope.querySelectorAll('[data-t], [data-t-title], [data-t-placeholder], [data-t-aria-label], [data-t-value], [data-t-html], [data-t-option]');
    nodes.forEach(applyTo);
  }

  // Expose globally
  window.i18n = {
    get lang(){ return store.lang; },
    setLang, load, t, apply: applyAll,
    onReady(fn){ if(typeof fn==='function'){ if(store.ready) fn(); else store.listeners.push(fn); } }
  };
  window.t = t;

  // Auto-load and apply
  load(detectLang()).then(() => {
    if(document.readyState === 'loading'){
      document.addEventListener('DOMContentLoaded', () => applyAll());
    } else applyAll();
  });
})();
