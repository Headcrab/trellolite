const api = {
  async me(){ return fetchJSON('/api/auth/me'); },
  async logout(){ return fetchJSON('/api/auth/logout', {method:'POST'}) },
  async getBoards(scope){ return fetchJSON('/api/boards' + (scope? ('?scope='+encodeURIComponent(scope)) : '')) },
  async createBoard(title){ return fetchJSON('/api/boards', {method:'POST', body:{title}}) },
  async listProjects(){ return fetchJSON('/api/projects'); },
  async createProject(name){ return fetchJSON('/api/projects', {method:'POST', body:{name}}); },
  async boardMembers(id){ return fetchJSON(`/api/boards/${id}/members`); },
  async getBoard(id){ return fetchJSON('/api/boards/'+id) },
  async getBoardFull(id){ return fetchJSON(`/api/boards/${id}/full`) },
  async updateBoard(id, title){ return fetchJSON(`/api/boards/${id}`, {method:'PATCH', body:{title}}) },
  async setBoardColor(id, color){ return fetchJSON(`/api/boards/${id}`, {method:'PATCH', body:{color}}) },
  async deleteBoard(id){ return fetchJSON(`/api/boards/${id}`, {method:'DELETE'}) },
  async getLists(bid){ return fetchJSON(`/api/boards/${bid}/lists`) },
  async createList(bid, title){ return fetchJSON(`/api/boards/${bid}/lists`, {method:'POST', body:{title}}) },
  async getCards(lid){ return fetchJSON(`/api/lists/${lid}/cards`) },
  async createCard(lid, title, description){ return fetchJSON(`/api/lists/${lid}/cards`, {method:'POST', body:{title, description}}) },
  async createCardAdvanced(lid, payload){ return fetchJSON(`/api/lists/${lid}/cards`, {method:'POST', body: payload}) },
  async moveCard(id, targetListId, newIndex){ return fetchJSON(`/api/cards/${id}/move`, {method:'POST', body:{target_list_id: targetListId, new_index: newIndex}}) },
  async updateList(id, payload){ return fetchJSON(`/api/lists/${id}`, {method:'PATCH', body:payload}) },
  async moveList(id, newIndex, targetBoardId){ return fetchJSON(`/api/lists/${id}/move`, {method:'POST', body:{new_index: newIndex, target_board_id: targetBoardId||0}}) },
  async deleteList(id){ return fetchJSON(`/api/lists/${id}`, {method:'DELETE'}) },
  async getComments(cardId){ return fetchJSON(`/api/cards/${cardId}/comments`) },
  async addComment(cardId, body){ return fetchJSON(`/api/cards/${cardId}/comments`, {method:'POST', body:{body}}) },
  async updateCardFields(id, payload){ return fetchJSON(`/api/cards/${id}`, {method:'PATCH', body:payload}) },
  async deleteCard(id){ return fetchJSON(`/api/cards/${id}`, {method:'DELETE'}) },
  async moveBoard(id, newIndex){ return fetchJSON(`/api/boards/${id}/move`, {method:'POST', body:{new_index: newIndex}}) },
  async myGroups(){ return fetchJSON('/api/my/groups'); },
  async createMyGroup(name){ return fetchJSON('/api/groups', {method:'POST', body:{name}}); },
  async myGroupUsers(id){ return fetchJSON(`/api/groups/${id}/users`); },
  async myAddUserToGroup(id, user_id){ return fetchJSON(`/api/groups/${id}/users`, {method:'POST', body:{user_id}}); },
  async myRemoveUserFromGroup(id, uid){ return fetchJSON(`/api/groups/${id}/users/${uid}`, {method:'DELETE'}); },
  async myLeaveGroup(id){ return fetchJSON(`/api/groups/${id}/leave`, {method:'POST'}); },
  async myDeleteGroup(id){ return fetchJSON(`/api/groups/${id}`, {method:'DELETE'}); },
  async boardGroups(id){ return fetchJSON(`/api/boards/${id}/groups`); },
  async addBoardGroup(id, group_id){ return fetchJSON(`/api/boards/${id}/groups`, {method:'POST', body:{group_id}}); },
  async removeBoardGroup(id, gid){ return fetchJSON(`/api/boards/${id}/groups/${gid}`, {method:'DELETE'}) },
  // Admin
  async adminListGroups(){ return fetchJSON('/api/admin/groups'); },
  async adminCreateGroup(name){ return fetchJSON('/api/admin/groups', {method:'POST', body:{name}}); },
  async adminDeleteGroup(id){ return fetchJSON(`/api/admin/groups/${id}`, {method:'DELETE'}); },
  async adminGroupUsers(id){ return fetchJSON(`/api/admin/groups/${id}/users`); },
  async adminAddUserToGroup(id, user_id){ return fetchJSON(`/api/admin/groups/${id}/users`, {method:'POST', body:{user_id}}); },
  async adminRemoveUserFromGroup(id, uid){ return fetchJSON(`/api/admin/groups/${id}/users/${uid}`, {method:'DELETE'}); },
  async adminListUsers(q, limit){ const p = new URLSearchParams(); if(q) p.set('q', q); if(limit) p.set('limit', String(limit)); return fetchJSON(`/api/admin/users${p.toString()?('?' + p.toString()):''}`); },
};

async function fetchJSON(url, opts={}){
  const init = {headers:{'Content-Type':'application/json'}};
  if(opts.method) init.method = opts.method;
  if(opts.body) init.body = JSON.stringify(opts.body);
  const res = await fetch(url, init);
  if(!res.ok){
    let msg = res.statusText;
    try { const j = await res.json(); if(j && j.error) msg = j.error } catch{}
    if(res.status === 401){
      // redirect to login page only for non-GET (mutation) requests
      const method = (init.method||'GET').toUpperCase();
      if(method !== 'GET' && !location.pathname.endsWith('/web/login.html')){
        location.href = '/web/login.html';
        return Promise.reject(new Error('unauthorized'));
      }
    }
    throw new Error(msg);
  }
  if(res.status === 204) return null;
  // some backends may send empty body with 200; guard json parse
  const text = await res.text();
  if(!text) return null;
  try { return JSON.parse(text); } catch { return null; }
}

const state = { boards: [], currentBoardId: null, boardMembers: new Map(), lists: [], cards: new Map(), currentCard: null, dragListCrossDrop: false, user: null, searchQuery: '' };
const el = (id) => document.getElementById(id);
const els = { boards: el('boards'), boardTitle: el('boardTitle'), lists: el('lists'),
  dlgBoard: el('dlgBoard'), formBoard: el('formBoard'), dlgList: el('dlgList'),
  formList: el('formList'), dlgCard: el('dlgCard'), formCard: el('formCard'),
  cardTitle: el('cardTitle'), cardDescription: el('cardDescription'), cardDueAt: el('cardDueAt'),
  btnNewBoard: el('btnNewBoard'), btnNewList: el('btnNewList'),
  btnRenameBoard: el('btnRenameBoard'), btnDeleteBoard: el('btnDeleteBoard'),
  dlgCardView: el('dlgCardView'), cvTitle: el('cvTitle'), cvDescription: el('cvDescription'),
  dlgCardView: el('dlgCardView'), cvTitle: el('cvTitle'), cvDescription: el('cvDescription'),
  cvAssignee: el('cvAssignee'), cvDueAt: el('cvDueAt'), cvComments: el('cvComments'), cvCommentText: el('cvCommentText'),
  btnAddComment: el('btnAddComment'), btnCloseCardView: el('btnCloseCardView'), btnSaveCardView: el('btnSaveCardView'),
  dlgConfirm: el('dlgConfirm'), formConfirm: el('formConfirm'), confirmMessage: el('confirmMessage'),
  dlgColor: el('dlgColor'), formColor: el('formColor'), colorCustom: el('colorCustom'),
  dlgInput: el('dlgInput'), formInput: el('formInput'), inputTitle: el('inputTitle'), inputLabel: el('inputLabel'), inputValue: el('inputValue'),
  dlgSelect: el('dlgSelect'), formSelect: el('formSelect'), selectTitle: el('selectTitle'), selectLabel: el('selectLabel'), selectControl: el('selectControl'),
  dlgDateTime: el('dlgDateTime'), dtMonth: el('dtMonth'), dtGrid: el('dtGrid'), dtPrev: el('dtPrev'), dtNext: el('dtNext'), dtHour: el('dtHour'), dtMinute: el('dtMinute'), btnDtOk: el('btnDtOk'), btnDtCancel: el('btnDtCancel'), btnDtClear: el('btnDtClear'), cardDueAt: el('cardDueAt') };

function applyThemeIcon(mode){
  const btn = document.getElementById('btnTheme'); if(!btn) return;
  const use = btn.querySelector('svg use');
  if(!use){
    btn.innerHTML = '<svg><use href="#i-auto"></use></svg>';
  }
  const icon = mode === 'dark' ? '#i-moon' : mode === 'light' ? '#i-sun' : '#i-auto';
  const useEl = btn.querySelector('use');
  if(useEl){
    useEl.setAttribute('href', icon);
    try{ useEl.setAttributeNS('http://www.w3.org/1999/xlink','href', icon); }catch{}
  }
}

function applySidebarState(collapsed){
  const root = document.getElementById('app'); if(!root) return;
  if(collapsed){ root.classList.add('sidebar-collapsed'); }
  else { root.classList.remove('sidebar-collapsed'); }
  const btn = document.getElementById('btnSidebarToggle');
  if(btn){
  btn.title = collapsed ? (typeof t==='function'? t('app.sidebar.expand') : 'Развернуть панель') : (typeof t==='function'? t('app.sidebar.collapse') : 'Свернуть панель');
    btn.setAttribute('aria-label', btn.title);
  }
}

function getPreferredTheme(){
  const saved = localStorage.getItem('theme');
  if(saved === 'light' || saved === 'dark' || saved === 'auto') return saved;
  return 'auto';
}

function setTheme(mode){
  const html = document.documentElement;
  if(mode === 'auto'){
    html.removeAttribute('data-theme');
  } else {
    html.setAttribute('data-theme', mode);
  }
  applyThemeIcon(mode);
}

function confirmDialog(message){
  return new Promise((resolve) => {
  els.confirmMessage.textContent = message || (typeof t==='function'? t('app.dialogs.are_you_sure') : 'Вы уверены?');
    els.dlgConfirm.returnValue = '';
    els.dlgConfirm.showModal();
    const onClose = () => {
      els.dlgConfirm.removeEventListener('close', onClose);
      resolve(els.dlgConfirm.returnValue === 'ok');
    };
    els.dlgConfirm.addEventListener('close', onClose);
  });
}

init();

async function init(){
  // Theme init
  const mode = getPreferredTheme(); setTheme(mode); applyThemeIcon(mode);
  // Sidebar collapsed state init
  try{ const saved = JSON.parse(localStorage.getItem('sidebarCollapsed')||''); if(typeof saved === 'boolean') applySidebarState(saved); }catch{}
  const btnSidebar = document.getElementById('btnSidebarToggle');
  if(btnSidebar){
    btnSidebar.addEventListener('click', () => {
      const root = document.getElementById('app');
      const collapsed = !root.classList.contains('sidebar-collapsed');
      applySidebarState(collapsed);
      try{ localStorage.setItem('sidebarCollapsed', JSON.stringify(collapsed)); }catch{}
    });
  }
  const btnTheme = document.getElementById('btnTheme');
  if(btnTheme){
    btnTheme.innerHTML = '<svg><use href="#i-auto"></use></svg>';
    btnTheme.addEventListener('click', () => {
      const current = getPreferredTheme();
      const next = current === 'auto' ? 'light' : current === 'light' ? 'dark' : 'auto';
      localStorage.setItem('theme', next); setTheme(next);
    });
  }
  const scopeSel = document.getElementById('boardsScope');
  // Новые переключатели фильтра: Мои/Группы
  const scopeMineBtn = document.getElementById('scopeMine');
  const scopeGroupsBtn = document.getElementById('scopeGroups');
  const loadScope = () => {
    try{
      const s = JSON.parse(localStorage.getItem('boardsScopeToggles')||'');
      if(s && typeof s.mine==='boolean' && typeof s.groups==='boolean'){
        if(scopeMineBtn) scopeMineBtn.setAttribute('aria-pressed', String(!!s.mine));
        if(scopeGroupsBtn) scopeGroupsBtn.setAttribute('aria-pressed', String(!!s.groups));
      }
    }catch{}
    // Если оба выключены, включим оба по умолчанию
    const mineOn = scopeMineBtn && scopeMineBtn.getAttribute('aria-pressed')==='true';
    const groupsOn = scopeGroupsBtn && scopeGroupsBtn.getAttribute('aria-pressed')==='true';
    if(scopeMineBtn && scopeGroupsBtn && !mineOn && !groupsOn){ scopeMineBtn.setAttribute('aria-pressed','true'); scopeGroupsBtn.setAttribute('aria-pressed','true'); }
  };
  const saveScope = () => {
    try{ localStorage.setItem('boardsScopeToggles', JSON.stringify({ mine: scopeMineBtn?.getAttribute('aria-pressed')==='true', groups: scopeGroupsBtn?.getAttribute('aria-pressed')==='true' })); }catch{}
  };
  const ensureAtLeastOne = (toggledBtn) => {
    const mineOn = scopeMineBtn?.getAttribute('aria-pressed')==='true';
    const groupsOn = scopeGroupsBtn?.getAttribute('aria-pressed')==='true';
    if(!mineOn && !groupsOn){
      // Включим другой, если пользователь выключил последний
      if(toggledBtn===scopeMineBtn && scopeGroupsBtn) scopeGroupsBtn.setAttribute('aria-pressed','true');
      if(toggledBtn===scopeGroupsBtn && scopeMineBtn) scopeMineBtn.setAttribute('aria-pressed','true');
    }
  };
  const onToggle = (btn) => {
    const cur = btn.getAttribute('aria-pressed')==='true';
    btn.setAttribute('aria-pressed', String(!cur));
    ensureAtLeastOne(btn); saveScope(); refreshBoards();
  };
  if(scopeMineBtn) scopeMineBtn.addEventListener('click', () => onToggle(scopeMineBtn));
  if(scopeGroupsBtn) scopeGroupsBtn.addEventListener('click', () => onToggle(scopeGroupsBtn));
  loadScope();
  const btnGroupsPanel = document.getElementById('btnGroupsPanel');
  if(btnGroupsPanel){ btnGroupsPanel.addEventListener('click', openGroupsPanel); }
  // Topbar search binding (client-side filter)
  const q = document.getElementById('q');
  if(q){
    const apply = () => { state.searchQuery = (q.value||'').trim().toLowerCase(); renderBoardsList(); };
    let t; q.addEventListener('input', () => { clearTimeout(t); t = setTimeout(apply, 150); });
    q.addEventListener('keydown', (e) => { if(e.key === 'Escape'){ q.value=''; apply(); } });
  }

  // Try to fetch current user; if not authorized, redirect to login (no anonymous access)
  try {
    const me = await api.me();
    state.user = me && me.user ? me.user : null;
    if (!state.user) { location.href = '/web/login.html'; return; }
    // Apply per-user language preference if provided by server
    try{
      if(window.i18n && state.user && state.user.lang){
        await i18n.setLang(state.user.lang);
        i18n.apply();
      }
    }catch{}
    updateUserBar();
  } catch {
    state.user = null; updateUserBar(); location.href = '/web/login.html'; return;
  }
  await refreshBoards(); bindUI(); setupContextMenu();
  if(state.boards.length) openBoard(state.boards[0].id);
}

function bindUI(){
  const btnLogout = document.getElementById('btnLogout');
  if(btnLogout){ btnLogout.addEventListener('click', async () => { try { await api.logout(); location.href = '/web/login.html'; } catch(e){ alert((typeof t==='function'? t('app.errors.cant_save',{msg:e.message}) : ('Не удалось выйти: '+e.message))); } }); }
  
  const btnAdmin = document.getElementById('btnAdmin');
  if(btnAdmin){ btnAdmin.addEventListener('click', () => { location.href = '/web/admin.html'; }); }
  // User menu close behaviors (click outside, on item, Esc)
  const userMenu = document.getElementById('userMenu');
  if(userMenu){
    const panel = userMenu.querySelector('.menu-panel');
    if(panel){
      panel.addEventListener('click', (e) => {
        const item = e.target && e.target.closest('[role="menuitem"]');
        if(item){ userMenu.open = false; }
      });
    }
    document.addEventListener('click', (e) => {
      if(!userMenu.contains(e.target)) userMenu.open = false;
    });
    userMenu.addEventListener('keydown', (e) => { if(e.key === 'Escape'){ userMenu.open = false; } });
  }
  els.btnNewBoard.addEventListener('click', async () => {
    els.formBoard.reset();
    // populate projects
    const sel = document.getElementById('projectSelect'); if(sel){ sel.innerHTML=''; try{ const items = await api.listProjects(); const opt0=document.createElement('option'); opt0.value=''; opt0.textContent='(без проекта)'; sel.appendChild(opt0); for(const p of (items||[])){ const o=document.createElement('option'); o.value=String(p.id); o.textContent=p.name; sel.appendChild(o);} } catch{} }
    els.dlgBoard.returnValue=''; els.dlgBoard.showModal();
  });
  els.dlgBoard.addEventListener('close', async () => {
    if(els.dlgBoard.returnValue === 'ok'){
      const fd = new FormData(els.formBoard);
      const title = (fd.get('title')||'').toString().trim(); if(!title) return;
      const project_id = parseInt((fd.get('project_id')||'').toString(), 10) || 0;
      const b = await fetchJSON('/api/boards', {method:'POST', body:{ title, project_id }});
      await refreshBoards(); openBoard(b.id);
    } else { els.formBoard.reset(); }
  });
  // Cancel buttons should behave like Esc (no validation)
  els.dlgBoard.querySelector('button[value="cancel"][type="button"]').addEventListener('click', () => { els.formBoard.reset(); els.dlgBoard.close('cancel'); });
  const btnNewProject = document.getElementById('btnNewProject');
  if(btnNewProject){ btnNewProject.addEventListener('click', async () => {
  const name = await inputDialog({ title:(typeof t==='function'? t('app.dialogs.board_new.new_project') : 'Новый проект…'), label:(typeof t==='function'? t('app.dialogs.board_new.name') : 'Название') }); if(!name) return;
    try { await api.createProject(name.trim()); const sel=document.getElementById('projectSelect'); if(sel){ const items = await api.listProjects(); sel.innerHTML=''; const opt0=document.createElement('option'); opt0.value=''; opt0.textContent='(без проекта)'; sel.appendChild(opt0); for(const p of (items||[])){ const o=document.createElement('option'); o.value=String(p.id); o.textContent=p.name; sel.appendChild(o);} sel.selectedIndex = 1; } }
  catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось создать проект: ' + err.message)); }
  }); }

  els.btnNewList.addEventListener('click', () => { if(!state.currentBoardId) return; els.formList.reset(); els.dlgList.returnValue=''; els.dlgList.showModal(); });
  els.dlgList.addEventListener('close', async () => {
    if(els.dlgList.returnValue === 'ok'){
      const fd = new FormData(els.formList);
      const title = fd.get('title').trim(); if(!title) return;
      try {
        const l = await api.createList(state.currentBoardId, title);
        if(!state.lists.some(x => x.id === l.id)){
          state.lists.push(l); state.cards.set(l.id, []);
          const col = buildListColumn(l);
          els.lists.appendChild(col);
        }
  } catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось создать список: ' + err.message)); }
    } else { els.formList.reset(); }
  });
  els.dlgList.querySelector('button[value="cancel"][type="button"]').addEventListener('click', () => { els.formList.reset(); els.dlgList.close('cancel'); });

  // Card dialog event handlers
  const btnCloseCardDialog = document.getElementById('btnCloseCardDialog');
  const btnCancelCardDialog = document.getElementById('btnCancelCardDialog');
  
  if (btnCloseCardDialog) {
    btnCloseCardDialog.addEventListener('click', () => {
      els.formCard.reset(); 
      delete els.formCard.dataset.listId; 
      els.dlgCard.close();
    });
  }
  
  if (btnCancelCardDialog) {
    btnCancelCardDialog.addEventListener('click', () => {
      els.formCard.reset(); 
      delete els.formCard.dataset.listId; 
      els.dlgCard.close();
    });
  }
  
  els.formCard.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(els.formCard);
    const title = fd.get('title').trim();
    const description = (fd.get('description')||'').trim();
    const description_is_md = !!els.formCard.querySelector('[name="description_is_md"]')?.checked;
    const due_at = (els.cardDueAt && els.cardDueAt.dataset.iso) ? els.cardDueAt.dataset.iso : '';
    const listId = parseInt(els.formCard.dataset.listId, 10);
    if(!title || !listId) return;
    try {
      const c = await api.createCardAdvanced(listId, { title, description, description_is_md });
      if(due_at){ try { await api.updateCardFields(c.id, { due_at }); c.due_at = due_at; } catch{} }
      if(!state.cards.has(listId)) state.cards.set(listId, []);
      const arr = state.cards.get(listId);
      if(!arr.some(x => x.id === c.id)){
        arr.push(c);
        const cardsEl = document.querySelector(`.cards[data-list-id="${listId}"]`);
        if(cardsEl) cardsEl.appendChild(renderCard(c));
      }
      els.formCard.reset(); 
      delete els.formCard.dataset.listId;
      els.dlgCard.close();
    } catch(err){ 
  alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось создать карточку: ' + err.message)); 
    }
  });

  // Board rename/delete
  els.btnRenameBoard.addEventListener('click', async () => {
    if(!state.currentBoardId) return;
    const current = els.boardTitle.textContent.trim();
  const name = await inputDialog({ title:'Переименование доски', label:'Новое название', value: current });
    if(!name || name.trim() === current) return;
    try { await api.updateBoard(state.currentBoardId, name.trim()); els.boardTitle.textContent = name.trim(); await refreshBoards(); }
  catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось переименовать: ' + err.message)); }
  });
  els.btnDeleteBoard.addEventListener('click', async () => {
    if(!state.currentBoardId) return;
    const ok = await confirmDialog('Удалить текущую доску безвозвратно?');
    if(!ok) return;
    try { await api.deleteBoard(state.currentBoardId); await refreshBoards(); state.currentBoardId = null; els.boardTitle.textContent = ''; els.lists.innerHTML=''; }
  catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось удалить доску: ' + err.message)); }
  });

  // Board groups access dialog
  const btnBoardGroups = document.getElementById('btnBoardGroups');
  const dlgGroups = document.getElementById('dlgGroups');
  const groupsList = document.getElementById('groupsList');
  const formGroups = document.getElementById('formGroups');
  if(btnBoardGroups && dlgGroups && groupsList){
    btnBoardGroups.addEventListener('click', async () => {
      if(!state.currentBoardId) return;
      // Only board owner may manage access
      const board = state.boards.find(b => b.id === state.currentBoardId);
      if(!(state.user && board && board.created_by && state.user.id === board.created_by)){
  alert(typeof t==='function'? t('app.errors.failed') : 'Только владелец доски может управлять доступом.');
        return;
      }
      groupsList.innerHTML = 'Загрузка...'; dlgGroups.returnValue=''; dlgGroups.showModal();
      try {
        const [mine, current] = await Promise.all([api.myGroups(), api.boardGroups(state.currentBoardId)]);
        const currentSet = new Set((current||[]).map(g=>g.id));
        groupsList.innerHTML = '';
        if(!mine || mine.length===0){
          groupsList.textContent = 'Вы не состоите ни в одной группе.';
        } else {
          for(const g of mine){
            const id = `g_${g.id}`;
            const row = document.createElement('label'); row.className='group-row';
            row.innerHTML = `<input type="checkbox" id="${id}" data-gid="${g.id}"> <span>${escapeHTML(g.name)}</span>`;
            const cb = row.querySelector('input'); cb.checked = currentSet.has(g.id);
            groupsList.appendChild(row);
          }
        }
      } catch(err){ groupsList.textContent = 'Ошибка загрузки групп: ' + err.message; }
    });
    dlgGroups.addEventListener('close', async () => {
      if(dlgGroups.returnValue !== 'ok') return;
      if(!state.currentBoardId) return;
      try {
        const [mine, current] = await Promise.all([api.myGroups(), api.boardGroups(state.currentBoardId)]);
        const currentSet = new Set((current||[]).map(g=>g.id));
        const rows = [...groupsList.querySelectorAll('input[type="checkbox"][data-gid]')];
        for(const cb of rows){
          const gid = parseInt(cb.dataset.gid, 10);
          const should = cb.checked; const has = currentSet.has(gid);
          if(should && !has){ try { await api.addBoardGroup(state.currentBoardId, gid); } catch(err){ console.warn('add group failed', err.message); } }
          if(!should && has){ try { await api.removeBoardGroup(state.currentBoardId, gid); } catch(err){ console.warn('remove group failed', err.message); } }
        }
      } catch(err){ console.warn('sync groups failed:', err.message); }
    });
    const cancelBtn = formGroups?.querySelector('button[value="cancel"][type="button"]');
    if(cancelBtn){ cancelBtn.addEventListener('click', () => { dlgGroups.close('cancel'); }); }
  }

  // Card view dialog
  els.btnCloseCardView.addEventListener('click', () => els.dlgCardView.close());
  els.btnSaveCardView.addEventListener('click', async () => {
    const c = state.currentCard; if(!c) return;
    const payload = {};
    const title = els.cvTitle.value.trim(); if(title && title !== c.title) payload.title = title;
    const description = els.cvDescription.value.trim(); if(description !== c.description) payload.description = description;
  const isMdToggle = document.getElementById('cvMdToggle');
  if(isMdToggle){ const md = isMdToggle.getAttribute('aria-pressed') === 'true'; if(typeof c.description_is_md === 'boolean'){ if(md !== !!c.description_is_md) payload.description_is_md = md; } else { payload.description_is_md = md; } }
  const dueVal = els.cvDueAt.dataset.iso || '';
  if(dueVal) payload.due_at = dueVal; else if(c.due_at) payload.due_at = '';
  // Assignee (value '0' means clear)
  if(els.cvAssignee){ const v = els.cvAssignee.value || '0'; const cur = c.assignee_id ? String(c.assignee_id) : '0'; if(v !== cur){ payload.assignee_id = Number(v); } }
    if(Object.keys(payload).length===0){ els.dlgCardView.close(); return; }
    try { await api.updateCardFields(c.id, payload); els.dlgCardView.close(); renderBoard(state.currentBoardId); }
  catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось сохранить карточку: ' + err.message)); }
  });
  // Live preview on typing
  if(els.cvDescription){ els.cvDescription.addEventListener('input', () => { renderCvDescriptionPreview(state.currentCard||{}); }); }
  els.btnAddComment.addEventListener('click', async () => {
    const c = state.currentCard; if(!c) return;
    const body = els.cvCommentText.value.trim(); if(!body) return;
    try { await api.addComment(c.id, body); els.cvCommentText.value=''; await loadComments(c.id); }
  catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось добавить комментарий: ' + err.message)); }
  });

  // Color dialog interactions
  if(els.dlgColor){
    els.dlgColor.addEventListener('click', e => {
      const btn = e.target.closest('.swatch'); if(!btn) return;
      els.dlgColor.dataset.selected = btn.dataset.color || '';
      els.dlgColor.close('ok');
    });
    const clearBtn = els.formColor && els.formColor.querySelector('button[value="clear"]');
    if(clearBtn){ clearBtn.addEventListener('click', () => { els.dlgColor.dataset.selected = ''; els.dlgColor.close('ok'); }); }
  }

  // Custom date-time picker bindings
  const bindDtInput = (inputEl) => {
    if(!inputEl) return;
    inputEl.addEventListener('click', async () => {
      const isoInit = inputEl.dataset.iso || '';
      const iso = await openDateTimePicker(isoInit);
      if(iso === undefined) return; // canceled
      if(iso === ''){ inputEl.value=''; delete inputEl.dataset.iso; return; }
      const {text} = toLocalTextAndISO(iso);
      inputEl.value = text; inputEl.dataset.iso = iso;
    });
  };
  bindDtInput(els.cardDueAt);
  bindDtInput(els.cvDueAt);
}


async function openGroupsPanel(){
  const dlg = document.getElementById('dlgGroupsPanel');
  const list = document.getElementById('groupsPanelList');
  
  if(!dlg || !list) return;
  dlg.returnValue=''; dlg.showModal();
  // close button
  const closeBtn = dlg.querySelector('button[value="cancel"]');
  if(closeBtn){ closeBtn.onclick = () => dlg.close('cancel'); }

  const renderMine = async () => {
    list.innerHTML = 'Загрузка…';
    try {
      const mine = await api.myGroups();
      list.innerHTML = '';
      // Блок: создание своей группы
      const tools = document.createElement('div'); tools.className='admin-actions';
      tools.innerHTML = `<input id="gpNewGroupNameUser" placeholder="Название группы"><button id="gpBtnCreateGroupUser" class="btn" type="button">Создать группу</button>`;
      list.appendChild(tools);
      const hint = document.createElement('div'); hint.className='muted'; hint.textContent = 'Создайте свои группы. Создатель — администратор.'; list.appendChild(hint);

      // Разделы: я админ и я участник
      const adminGroups = (mine||[]).filter(g => (g.role||0) >= 2);
      const memberGroups = (mine||[]).filter(g => (g.role||0) < 2);

      const section = (title) => { const h=document.createElement('h4'); h.textContent=title; list.appendChild(h); };
      section('Мои группы (я админ)');
      if(adminGroups.length===0){ const d=document.createElement('div'); d.className='muted'; d.textContent='Администратор ни в одной группе.'; list.appendChild(d); }
      for(const g of adminGroups){
        const row = document.createElement('div'); row.className='group-row';
        row.innerHTML = `<span><svg aria-hidden="true" style="width:14px;height:14px;vertical-align:-2px;margin-right:6px"><use href="#i-users"></use></svg>${escapeHTML(g.name)}</span><div class="spacer"></div><button class="btn" data-act="users" data-id="${g.id}">Пользователи…</button><button class="btn" data-act="del" data-id="${g.id}">Удалить</button>`;
        list.appendChild(row);
      }
      section('Группы, в которых я состою');
      if(memberGroups.length===0){ const d=document.createElement('div'); d.className='muted'; d.textContent='Нет групп.'; list.appendChild(d); }
      for(const g of memberGroups){
        const row = document.createElement('div'); row.className='group-row';
        row.innerHTML = `<span><svg aria-hidden="true" style="width:14px;height:14px;vertical-align:-2px;margin-right:6px"><use href="#i-user"></use></svg>${escapeHTML(g.name)}</span><div class="spacer"></div><button class="btn" data-act="leave" data-id="${g.id}">Выйти</button>`;
        list.appendChild(row);
      }
    } catch(err){ list.textContent = 'Ошибка: ' + err.message; }
  };


  const wireUserCreate = () => {
    const userNameEl = document.getElementById('gpNewGroupNameUser');
    const userBtn = document.getElementById('gpBtnCreateGroupUser');
    if(userBtn){
      userBtn.onclick = async () => {
        const name = (userNameEl.value||'').trim(); if(!name) return;
        try { await api.createMyGroup(name); userNameEl.value=''; await rerender(); }
  catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось создать группу: ' + err.message)); }
      };
    }
  };

  const rerender = async () => { await renderMine(); wireUserCreate(); };

  await rerender();

  // no global admin tools here

  list.onclick = async (e) => {
    const btn = e.target.closest('button'); if(!btn) return;
    const id = parseInt(btn.dataset.id,10);
    if(btn.dataset.act==='del'){
      const ok = await confirmDialog('Удалить группу?'); if(!ok) return;
      try{
        // Если пользователь админ — удаление своей группы, иначе (если админ системы) — админский маршрут
        if(state.user && state.user.is_admin){ try { await api.adminDeleteGroup(id); } catch(e) { /* может не быть прав если это чужая группа */ } }
        await api.myDeleteGroup(id);
        await rerender();
  }catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось удалить: '+err.message)); }
    }
    if(btn.dataset.act==='users'){
      // Открыть диалог управления участниками; если не админ системы — используем self-эндпоинты в обёртке
      openMembersDialogSelf(id);
    }
    if(btn.dataset.act==='leave'){
      const ok = await confirmDialog('Покинуть эту группу?'); if(!ok) return;
      try { await api.myLeaveGroup(id); await rerender(); }
  catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось выйти из группы: ' + err.message)); }
    }
  };
}

function openMembersDialogSelf(groupId){
  const dlg = document.getElementById('dlgMembers'); const list = document.getElementById('membersList'); const inp = document.getElementById('userSearch'); const status = document.getElementById('membersStatus');
  if(!dlg) return;
  const cancelBtn = document.querySelector('#formMembers button[value="cancel"][type="button"]');
  if(cancelBtn){ cancelBtn.onclick = () => dlg.close('cancel'); }
  const renderMembers = async () => {
    list.innerHTML = 'Загрузка...'; if(status) status.textContent='';
    try{
      const users = await api.myGroupUsers(groupId);
      list.innerHTML = '';
      for(const u of (users||[])){
        const row = document.createElement('div'); row.className='group-row';
  row.innerHTML = `<span>${escapeHTML(u.name || ('#'+u.id))}</span><div class="spacer"></div><button class="btn" data-act="rm" data-id="${u.id}">Убрать</button>`;
        list.appendChild(row);
      }
      if(status) status.textContent = `Участников: ${(users||[]).length}`;
      if(list.innerHTML==='') list.textContent = 'Пока пусто.';
    }catch(err){ list.textContent = 'Ошибка: '+err.message; }
  };
  const searchAndRender = async () => {
    const q = (inp.value||'').trim(); if(!q){ renderMembers(); return; }
    list.innerHTML = 'Поиск...'; if(status) status.textContent='';
    try{
      // использовать self-поиск
      const params = new URLSearchParams({ q, limit: '20' });
      const users = await fetchJSON(`/api/groups/${groupId}/users/search?${params.toString()}`);
      const members = await api.myGroupUsers(groupId); const memberSet = new Set((members||[]).map(u=>u.id));
      list.innerHTML = '';
      for(const u of (users||[])){
        const isMember = memberSet.has(u.id);
        const row = document.createElement('div'); row.className='group-row';
  row.innerHTML = `<span>${escapeHTML(u.name || ('#'+u.id))}</span><div class="spacer"></div>` + (isMember? `<button class="btn" data-act="rm" data-id="${u.id}">Убрать</button>`: `<button class="btn" data-act="add" data-id="${u.id}">Добавить</button>`);
        list.appendChild(row);
      }
      if(status) status.textContent = `Найдено: ${(users||[]).length}`;
      if(list.innerHTML==='') list.textContent = 'Ничего не найдено.';
    }catch(err){ list.textContent = 'Ошибка поиска: '+err.message; }
  };
  list.onclick = async (e) => {
    const btn = e.target.closest('button'); if(!btn) return; const uid = parseInt(btn.dataset.id,10);
  if(btn.dataset.act==='add'){ try{ await api.myAddUserToGroup(groupId, uid); await searchAndRender(); }catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось добавить: '+err.message)); } }
  if(btn.dataset.act==='rm'){ try{ await api.myRemoveUserFromGroup(groupId, uid); await searchAndRender(); }catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось убрать: '+err.message)); } }
  };
  // Debounced search
  let t; inp.oninput = () => { clearTimeout(t); t = setTimeout(searchAndRender, 250); };
  dlg.returnValue=''; dlg.showModal();
  renderMembers();
}

function openMembersDialog(groupId){
  const dlg = document.getElementById('dlgMembers'); const list = document.getElementById('membersList'); const inp = document.getElementById('userSearch'); const status = document.getElementById('membersStatus');
  if(!dlg) return;
  const cancelBtn = document.querySelector('#formMembers button[value="cancel"][type="button"]');
  if(cancelBtn){ cancelBtn.onclick = () => dlg.close('cancel'); }
  const renderMembers = async () => {
    list.innerHTML = 'Загрузка...'; if(status) status.textContent='';
    try{
      const users = await api.adminGroupUsers(groupId);
      list.innerHTML = '';
      for(const u of (users||[])){
        const row = document.createElement('div'); row.className='group-row';
  row.innerHTML = `<span>${escapeHTML(u.name || ('#'+u.id))}</span><div class="spacer"></div><button class="btn" data-act="rm" data-id="${u.id}">Убрать</button>`;
        list.appendChild(row);
      }
      if(status) status.textContent = `Участников: ${(users||[]).length}`;
      if(list.innerHTML==='') list.textContent = 'Пока пусто.';
    }catch(err){ list.textContent = 'Ошибка: '+err.message; }
  };
  const searchAndRender = async () => {
    const q = (inp.value||'').trim(); if(!q){ renderMembers(); return; }
    list.innerHTML = 'Поиск...'; if(status) status.textContent='';
    try{
      const users = await api.adminListUsers(q, 20);
      const members = await api.adminGroupUsers(groupId); const memberSet = new Set((members||[]).map(u=>u.id));
      list.innerHTML = '';
      for(const u of (users||[])){
        const isMember = memberSet.has(u.id);
        const row = document.createElement('div'); row.className='group-row';
  row.innerHTML = `<span>${escapeHTML(u.name || ('#'+u.id))}</span><div class="spacer"></div>` + (isMember? `<button class="btn" data-act="rm" data-id="${u.id}">Убрать</button>`: `<button class="btn" data-act="add" data-id="${u.id}">Добавить</button>`);
        list.appendChild(row);
      }
      if(status) status.textContent = `Найдено: ${(users||[]).length}`;
      if(list.innerHTML==='') list.textContent = 'Ничего не найдено.';
    }catch(err){ list.textContent = 'Ошибка поиска: '+err.message; }
  };
  list.onclick = async (e) => {
    const btn = e.target.closest('button'); if(!btn) return; const uid = parseInt(btn.dataset.id,10);
  if(btn.dataset.act==='add'){ try{ await api.adminAddUserToGroup(groupId, uid); await searchAndRender(); }catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось добавить: '+err.message)); } }
  if(btn.dataset.act==='rm'){ try{ await api.adminRemoveUserFromGroup(groupId, uid); await searchAndRender(); }catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось убрать: '+err.message)); } }
  };
  // Debounced search
  let t; inp.oninput = () => { clearTimeout(t); t = setTimeout(searchAndRender, 250); };
  dlg.returnValue=''; dlg.showModal();
  renderMembers();
}

function updateUserBar(){
  // Toggle admin visibility in the new user menu
  const btnAdmin = document.getElementById('btnAdmin');
  if(btnAdmin) { btnAdmin.style.display = (state.user && state.user.is_admin) ? 'block' : 'none'; }
  // Update avatar letter in topbar
  const avEl = document.querySelector('.topbar .avatar');
  if(avEl){
    const src = (state.user && (state.user.name || state.user.email)) || '';
    const letter = (src.trim().charAt(0).toUpperCase() || 'U');
    avEl.textContent = letter;
  }
}

// ---- Context menu ----
let ctxMenuEl;
function setupContextMenu(){
  // Create menu element once
  ctxMenuEl = document.createElement('div');
  ctxMenuEl.className = 'ctx-menu';
  ctxMenuEl.style.display = 'none';
  document.body.appendChild(ctxMenuEl);

  const hide = () => { ctxMenuEl.style.display = 'none'; ctxMenuEl.innerHTML = ''; };
  document.addEventListener('click', hide);
  window.addEventListener('resize', hide);
  window.addEventListener('scroll', hide, true);
  document.addEventListener('keydown', (e) => { if(e.key === 'Escape') hide(); });
  // Keyboard navigation inside menu
  ctxMenuEl.addEventListener('keydown', (e) => {
    const items = [...ctxMenuEl.querySelectorAll('li:not(.disabled):not(.sep)')];
    if(items.length === 0) return;
    const active = document.activeElement && document.activeElement.closest('.ctx-menu li');
    let idx = items.indexOf(active);
    if(e.key === 'ArrowDown'){
      e.preventDefault();
      idx = (idx + 1) % items.length; items[idx].focus();
    } else if(e.key === 'ArrowUp'){
      e.preventDefault();
      idx = (idx - 1 + items.length) % items.length; items[idx].focus();
    } else if(e.key === 'Home'){
      e.preventDefault(); items[0].focus();
    } else if(e.key === 'End'){
      e.preventDefault(); items[items.length - 1].focus();
    } else if(e.key === 'Enter' || e.key === ' '){
      e.preventDefault(); if(active){ active.click(); }
    }
  });

  document.addEventListener('contextmenu', async (e) => {
    e.preventDefault();
    // Build items based on target
    const items = await buildContextMenuItems(e);
    if(!items || items.length === 0){ hide(); return; }
    renderContextMenu(items, e.clientX, e.clientY);
  });
}

function renderContextMenu(items, x, y){
  ctxMenuEl.innerHTML = '';
  const ul = document.createElement('ul');
  ul.setAttribute('role', 'menu');
  for(const it of items){
    if(it.type === 'separator' || it.label === '---'){
      const hr = document.createElement('li'); hr.className = 'sep'; ul.appendChild(hr); continue;
    }
    const li = document.createElement('li');
    li.textContent = it.label;
    li.setAttribute('role', 'menuitem');
    if(it.danger) li.classList.add('danger');
    if(it.disabled){ li.classList.add('disabled'); }
    else if(typeof it.action === 'function'){
      li.addEventListener('click', () => { it.action(); ctxMenuEl.style.display='none'; });
    }
    if(!it.disabled){ li.tabIndex = 0; }
    ul.appendChild(li);
  }
  ctxMenuEl.appendChild(ul);
  ctxMenuEl.style.display = 'block';
  // position with viewport bounds consideration
  const rect = ctxMenuEl.getBoundingClientRect();
  let left = x, top = y;
  if(left + rect.width > window.innerWidth) left = Math.max(4, window.innerWidth - rect.width - 4);
  if(top + rect.height > window.innerHeight) top = Math.max(4, window.innerHeight - rect.height - 4);
  ctxMenuEl.style.left = left + 'px';
  ctxMenuEl.style.top = top + 'px';
  // focus first actionable item
  const first = ctxMenuEl.querySelector('li[role="menuitem"]:not(.disabled):not(.sep)');
  if(first) first.focus();
}

async function buildContextMenuItems(e){
  const targetCard = e.target.closest('.card');
  if(targetCard){
    const id = parseInt(targetCard.dataset.id, 10);
    const listId = parseInt(targetCard.closest('.cards')?.dataset.listId || '0', 10);
    const c = (state.cards.get(listId) || []).find(x => x.id === id);
    return [
  { label: 'Открыть/Редактировать', action: () => openCard(c) },
  { label: 'Дубликат карточки', action: async () => { await duplicateCard(id, listId); } },
  { label: 'Переместить…', action: async () => { await moveCardPrompt(id, listId); } },
      { label: 'Цвет…', action: async () => {
          const color = await pickColor(c?.color || ''); if(color === undefined) return;
          try { await api.updateCardFields(id, { color: color || '' }); if(c){ c.color = color || ''; const el = document.querySelector(`.card[data-id="${id}"]`); if(el){ if(c.color) el.style.setProperty('--clr', c.color); else el.style.removeProperty('--clr'); } } }
          catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось сохранить цвет карточки: ' + err.message)); }
        } },
      { label: '---' },
      { label: 'Удалить карточку', danger: true, action: async () => {
          const ok = await confirmDialog('Удалить карточку безвозвратно?'); if(!ok) return;
          try { await api.deleteCard(id); const el = document.querySelector(`.card[data-id="${id}"]`); if(el) el.remove(); const arr = state.cards.get(listId) || []; state.cards.set(listId, arr.filter(x=>x.id!==id)); }
          catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось удалить карточку: ' + err.message)); }
        } },
    ];
  }

  const targetList = e.target.closest('section.list');
  if(targetList){
    const listId = parseInt(targetList.dataset.id, 10);
    const l = state.lists.find(x => x.id === listId);
    return [
      { label: 'Добавить карточку', action: () => { els.formCard.reset(); els.formCard.dataset.listId = listId; els.dlgCard.returnValue=''; els.dlgCard.showModal(); } },
    { label: 'Переименовать список', action: async () => {
      const current = l?.title || targetList.querySelector('h3')?.textContent?.trim() || '';
      const name = await inputDialog({ title:'Переименование списка', label:'Новое название', value: current }); if(!name || !name.trim() || name.trim() === current) return;
          try { await api.updateList(listId, { title: name.trim() }); if(l){ l.title = name.trim(); } const h3 = targetList.querySelector('h3'); if(h3) h3.textContent = name.trim(); }
          catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось переименовать: ' + err.message)); }
        } },
  { label: 'Дубликат списка', action: async () => { await duplicateList(listId); } },
  { label: 'Переместить…', action: async () => { await moveListPrompt(listId); } },
      { label: 'Цвет…', action: async () => {
          const color = await pickColor(l?.color || ''); if(color === undefined) return;
          try { await api.updateList(listId, { color: color || '' }); if(l){ l.color = color || ''; } if(color) targetList.style.setProperty('--clr', color); else targetList.style.removeProperty('--clr'); }
          catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось сохранить цвет списка: ' + err.message)); }
        } },
      { label: '---' },
      { label: 'Удалить список', danger: true, action: async () => {
          const ok = await confirmDialog('Удалить список и его карточки?'); if(!ok) return;
          try { await api.deleteList(listId); state.lists = state.lists.filter(x => x.id !== listId); state.cards.delete(listId); targetList.remove(); }
          catch(err){ alert(typeof t==='function'? t('app.errors.cant_save',{msg: err.message}) : ('Не удалось удалить список: ' + err.message)); }
        } },
    ];
  }

  const targetBoardLi = e.target.closest('#boards li');
  if(targetBoardLi){
    const boardId = parseInt(targetBoardLi.dataset.id, 10);
    const b = state.boards.find(x => x.id === boardId);
    const isOwner = !!(state.user && b && b.created_by && state.user.id === b.created_by);
    return [
      { label: 'Открыть доску', action: () => openBoard(boardId) },
    { label: 'Переименовать доску', disabled: !isOwner, action: async () => {
      const current = b?.title || targetBoardLi.querySelector('.t')?.textContent?.trim() || '';
      const name = await inputDialog({ title:'Переименование доски', label:'Новое название', value: current }); if(!name || !name.trim() || name.trim() === current) return;
          try { await api.updateBoard(boardId, name.trim()); if(b){ b.title = name.trim(); } const t = targetBoardLi.querySelector('.t'); if(t) t.textContent = name.trim(); await refreshBoards(); }
          catch(err){ alert('Не удалось переименовать: ' + err.message); }
        } },
      { label: 'Цвет…', disabled: !isOwner, action: async () => {
          const color = await pickColor(b?.color || ''); if(color === undefined) return;
          try { await api.setBoardColor(boardId, color || ''); if(b){ b.color = color || ''; } if(color) targetBoardLi.style.setProperty('--clr', color); else targetBoardLi.style.removeProperty('--clr'); }
          catch(err){ alert('Не удалось сохранить цвет доски: ' + err.message); }
        } },
      { label: '---' },
      { label: 'Удалить доску', danger: true, disabled: !isOwner, action: async () => {
          const ok = await confirmDialog('Удалить доску безвозвратно?'); if(!ok) return;
          try { await api.deleteBoard(boardId); await refreshBoards(); if(state.currentBoardId === boardId){ state.currentBoardId = null; els.boardTitle.textContent=''; els.lists.innerHTML=''; }
          } catch(err){ alert('Не удалось удалить доску: ' + err.message); }
        } },
    ];
  }

  // Containers / empty areas
  if(e.target.closest('#boards')){
    return [
      { label: 'Новая доска', action: () => { els.formBoard.reset(); els.dlgBoard.returnValue=''; els.dlgBoard.showModal(); } },
    ];
  }
  if(e.target.closest('#boardHeader') || e.target.closest('.main')){
    const boardId = state.currentBoardId;
    const b = state.boards.find(x => x.id === boardId);
    const base = [];
    if(boardId){
      const isOwner = !!(state.user && b && b.created_by && state.user.id === b.created_by);
      base.push(
        { label: 'Новый список', action: () => { if(!state.currentBoardId) return; els.formList.reset(); els.dlgList.returnValue=''; els.dlgList.showModal(); } },
    { label: 'Переименовать доску', disabled: !isOwner, action: async () => {
      const current = els.boardTitle.textContent.trim();
      const name = await inputDialog({ title:'Переименование доски', label:'Новое название', value: current }); if(!name || !name.trim() || name.trim() === current) return;
            try { await api.updateBoard(boardId, name.trim()); els.boardTitle.textContent = name.trim(); await refreshBoards(); }
            catch(err){ alert('Не удалось переименовать: ' + err.message); }
          } },
        { label: 'Цвет…', disabled: !isOwner, action: async () => {
            const color = await pickColor(b?.color || ''); if(color === undefined) return;
            try { await api.setBoardColor(boardId, color || ''); if(b){ b.color = color || ''; } await refreshBoards(); }
            catch(err){ alert('Не удалось сохранить цвет доски: ' + err.message); }
          } },
        { label: '---' },
        { label: 'Удалить доску', danger: true, disabled: !isOwner, action: async () => {
            const ok = await confirmDialog('Удалить текущую доску безвозвратно?'); if(!ok) return;
            try { await api.deleteBoard(boardId); await refreshBoards(); state.currentBoardId = null; els.boardTitle.textContent = ''; els.lists.innerHTML=''; }
            catch(err){ alert('Не удалось удалить доску: ' + err.message); }
          } },
      );
    } else {
      base.push({ label: 'Новая доска', action: () => { els.formBoard.reset(); els.dlgBoard.returnValue=''; els.dlgBoard.showModal(); } });
    }
    return base;
  }

  const cardsContainer = e.target.closest('.cards');
  if(cardsContainer){
    const listId = parseInt(cardsContainer.dataset.listId || '0', 10);
    return [ { label: 'Добавить карточку', action: () => { els.formCard.reset(); els.formCard.dataset.listId = listId; els.dlgCard.returnValue=''; els.dlgCard.showModal(); } } ];
  }

  // Fallback: generic new board
  return [ { label: 'Новая доска', action: () => { els.formBoard.reset(); els.dlgBoard.returnValue=''; els.dlgBoard.showModal(); } } ];
}

// Duplicate a card in the same list (title, description, due_at, color)
async function duplicateCard(cardId, listId){
  const arr = state.cards.get(listId) || [];
  const src = arr.find(x => x.id === cardId);
  if(!src) return;
  try {
  const created = await api.createCardAdvanced(listId, { title: src.title || '', description: src.description || '', description_is_md: !!src.description_is_md });
    const payload = {};
    if(src.due_at) payload.due_at = src.due_at;
    if(src.color) payload.color = src.color;
    if(Object.keys(payload).length){ try { await api.updateCardFields(created.id, payload); Object.assign(created, payload); } catch{} }
    const cardsEl = document.querySelector(`.cards[data-list-id="${listId}"]`);
    if(cardsEl){ cardsEl.appendChild(renderCard(created)); }
    arr.push(created); state.cards.set(listId, arr);
  } catch(err){ alert('Не удалось дублировать карточку: ' + err.message); }
}

// Duplicate a list (title, color) and copy its cards
async function duplicateList(listId){
  const l = state.lists.find(x => x.id === listId); if(!l) return;
  const title = (l.title || '') + ' (копия)';
  try {
    const nl = await api.createList(state.currentBoardId, title);
    if(l.color){ try { await api.updateList(nl.id, { color: l.color }); nl.color = l.color; } catch{} }
    // Add to state/UI
    state.lists.push(nl); state.cards.set(nl.id, []);
    const col = buildListColumn(nl); els.lists.appendChild(col);
    // Copy cards sequentially
    const cards = state.cards.get(listId) || [];
    for(const c of cards){ try { await duplicateCard(c.id, listId); } catch{} }
  } catch(err){ alert('Не удалось дублировать список: ' + err.message); }
}

// Prompt-driven card move to another list within current board (to end)
async function moveCardPrompt(cardId, currentListId){
  const lists = state.lists || []; if(!lists.length) return;
  const idx = await selectDialog({ title:'Переместить карточку', label:'Список назначения', options: lists.map((l,i)=>({value: String(i), label: l.title})) });
  if(idx === undefined || idx === null) return;
  const target = lists[parseInt(idx,10)]; if(!target) return;
  try { await api.moveCard(cardId, target.id, 1<<30); renderBoard(state.currentBoardId); }
  catch(err){ alert('Не удалось переместить карточку: ' + err.message); }
}

// Prompt-driven list move to another board (to end)
async function moveListPrompt(listId){
  const boards = state.boards || []; if(!boards.length) return;
  const idx = await selectDialog({ title:'Переместить список', label:'Доска назначения', options: boards.map((b,i)=>({value: String(i), label: b.title + (b.id===state.currentBoardId?' (текущая)':'' )})) });
  if(idx === undefined || idx === null) return;
  const target = boards[parseInt(idx,10)]; if(!target) return;
  try { await api.moveList(listId, 1<<30, target.id); renderBoard(state.currentBoardId); }
  catch(err){ alert('Не удалось переместить список: ' + err.message); }
}

// App-level dialogs
function inputDialog({ title, label, value }){
  return new Promise((resolve) => {
    els.inputTitle.textContent = title || 'Ввод';
    els.inputLabel.textContent = label || 'Значение';
    els.inputValue.value = value || '';
    const onClose = () => {
      els.dlgInput.removeEventListener('close', onClose);
      const rv = els.dlgInput.returnValue;
      if(rv !== 'ok'){ resolve(undefined); return; }
      resolve(els.inputValue.value);
    };
    els.dlgInput.addEventListener('close', onClose);
    const cancelBtn = els.formInput.querySelector('button[value="cancel"][type="button"]');
    if(cancelBtn){ cancelBtn.onclick = () => { els.dlgInput.close('cancel'); }; }
    els.dlgInput.returnValue=''; els.dlgInput.showModal();
    setTimeout(()=>{ els.inputValue.focus(); els.inputValue.select(); }, 0);
  });
}

function selectDialog({ title, label, options }){
  return new Promise((resolve) => {
    els.selectTitle.textContent = title || 'Выбор';
    els.selectLabel.textContent = label || 'Выберите';
    els.selectControl.innerHTML = '';
    for(const opt of (options||[])){
      const o = document.createElement('option'); o.value = String(opt.value); o.textContent = String(opt.label); els.selectControl.appendChild(o);
    }
    const onClose = () => {
      els.dlgSelect.removeEventListener('close', onClose);
      const rv = els.dlgSelect.returnValue;
      if(rv !== 'ok'){ resolve(undefined); return; }
      resolve(els.selectControl.value);
    };
    els.dlgSelect.addEventListener('close', onClose);
    const cancelBtn = els.formSelect.querySelector('button[value="cancel"][type="button"]');
    if(cancelBtn){ cancelBtn.onclick = () => { els.dlgSelect.close('cancel'); }; }
    els.dlgSelect.returnValue=''; els.dlgSelect.showModal();
    setTimeout(()=>{ els.selectControl.focus(); }, 0);
  });
}

// Color picker helper: returns string (e.g., #3b82f6), '' for none, or undefined if canceled
function pickColor(current){
  return new Promise((resolve) => {
    if(!els.dlgColor){ resolve(undefined); return; }
  if('selected' in els.dlgColor.dataset){ delete els.dlgColor.dataset.selected; }
    if(els.colorCustom){ els.colorCustom.value = (current && /^#([0-9a-f]{6})$/i.test(current)) ? current : '#3b82f6'; }
    const onClose = () => {
      els.dlgColor.removeEventListener('close', onClose);
      const rv = els.dlgColor.returnValue;
      if(rv !== 'ok'){ resolve(undefined); return; }
      const sel = els.dlgColor.dataset.selected;
      if(sel !== undefined){ resolve(sel); return; }
      // If no swatch was clicked, use custom input value
      resolve(els.colorCustom ? els.colorCustom.value : '');
    };
    els.dlgColor.addEventListener('close', onClose);
    els.dlgColor.returnValue = '';
    els.dlgColor.showModal();
  });
}

function updateBoardHeaderActions(){
  const enabled = !!state.currentBoardId;
  const btns = [document.getElementById('btnBoardGroups'), els.btnRenameBoard, els.btnDeleteBoard, els.btnNewList];
  const board = state.boards.find(b => b.id === state.currentBoardId) || null;
  const isOwner = !!(state.user && board && board.created_by && state.user.id === board.created_by);
  for(const b of btns){ if(!b) continue; b.disabled = !enabled || (!isOwner && (b.id==='btnBoardGroups' || b===els.btnRenameBoard || b===els.btnDeleteBoard)); }
}

function renderBoardsList(){
  els.boards.innerHTML = '';
  const q = (state.searchQuery||'').toLowerCase();
  const list = (state.boards||[]).filter(b => !q || (b.title||'').toLowerCase().includes(q));
  for(const b of list){
    const li = document.createElement('li');
    li.dataset.id = b.id;
    const isOwner = !!(state.user && b.created_by && state.user.id === b.created_by);
    const iconId = (!isOwner && b.via_group) ? '#i-users' : '#i-board';
  const dragTitle = (typeof t==='function'? t('app.sidebar.drag_board') : 'Перетащить доску');
  const colorTitle = (typeof t==='function'? t('app.sidebar.color') : 'Цвет');
  li.innerHTML = `<span class="drag-handle" title="${dragTitle}"><svg aria-hidden="true"><use href="#i-grip"></use></svg></span><span class="ico"><svg aria-hidden="true"><use href="${iconId}"></use></svg></span><span class="t">${escapeHTML(b.title)}</span><button class="btn icon btn-color" title="${colorTitle}" aria-label="${colorTitle}"><svg aria-hidden="true"><use href="#i-palette"></use></svg></button>`;
    li.addEventListener('click', () => openBoard(b.id));
    if(b.color){ li.style.setProperty('--clr', b.color); }
    const colorBtn = li.querySelector('.btn-color');
    if(colorBtn){
      colorBtn.addEventListener('click', async (ev) => {
        ev.stopPropagation();
        if(!(state.user && b.created_by && state.user.id === b.created_by)) return;
        const color = await pickColor(b.color || ''); if(color === undefined) return;
        try { await api.setBoardColor(b.id, color || ''); b.color = color || ''; if(b.color) li.style.setProperty('--clr', b.color); else li.style.removeProperty('--clr'); }
        catch(err){ alert('Не удалось сохранить цвет: ' + err.message); }
      });
      if(!(state.user && b.created_by && state.user.id === b.created_by)){
        colorBtn.disabled = true;
      }
    }
    if(b.id === state.currentBoardId) li.classList.add('active');
    els.boards.appendChild(li);
  }
  enableBoardsDnD();
  updateBoardHeaderActions();
}

async function refreshBoards(){
  // Определяем область на основе переключателей
  const mineOn = (document.getElementById('scopeMine')?.getAttribute('aria-pressed')==='true');
  const groupsOn = (document.getElementById('scopeGroups')?.getAttribute('aria-pressed')==='true');
  const scope = mineOn && groupsOn ? 'all' : mineOn ? 'mine' : groupsOn ? 'groups' : 'mine';
  const data = await api.getBoards(scope).catch(err => { console.warn('getBoards failed:', err.message); return null; });
  state.boards = Array.isArray(data) ? data : (data && Array.isArray(data.boards) ? data.boards : []);
  renderBoardsList();
  // Ensure selection exists
  const ids = new Set((state.boards||[]).map(b=>b.id));
  if(!state.currentBoardId || !ids.has(state.currentBoardId)){
    if(state.boards && state.boards.length){ await openBoard(state.boards[0].id); }
    else {
      state.currentBoardId = null; els.boardTitle.textContent = ''; els.lists.innerHTML = '';
    }
  }
}

async function openBoard(id){ state.currentBoardId = id; updateBoardHeaderActions(); await renderBoard(id);
  [...els.boards.children].forEach(li => li.classList.toggle('active', parseInt(li.dataset.id,10)===id)); }

let sse;
async function renderBoard(id){
  els.boardTitle.textContent = (typeof t==='function'? t('app.board.loading') : 'Загрузка...');
  try {
    const full = await api.getBoardFull(id);
    // Preload board members for assignee select and card badges
    try { const members = await api.boardMembers(full.board.id); state.boardMembers.set(full.board.id, members||[]); } catch(e){ console.warn('board members load failed', e.message); }
    els.boardTitle.textContent = full.board.title;
    state.lists = full.lists || []; state.cards.clear();
    for(const l of state.lists){ state.cards.set(l.id, (full.cards && full.cards[l.id]) || []); }
    renderLists();
    // SSE subscribe
    if(sse) { sse.close(); sse = null; }
    sse = new EventSource(`/api/boards/${id}/events`);
    sse.onmessage = (e) => { try { onEvent(JSON.parse(e.data)); } catch{} };
  } catch(err){ els.boardTitle.textContent = (typeof t==='function'? t('app.board.load_error') : 'Ошибка загрузки'); alert((typeof t==='function'? (t('app.errors.failed')+': ') : 'Ошибка: ') + err.message); }
}

function onEvent(ev){
  if(!ev || ev.board_id !== state.currentBoardId) return;
  switch(ev.type){
    case 'list.created': {
      const l = ev.payload; if(!l) return;
      if(!state.lists.some(x => x.id === l.id)){
        state.lists.push(l); state.cards.set(l.id, []);
        els.lists.appendChild(buildListColumn(l));
      }
      break;
    }
    case 'list.updated': {
      const id = ev.payload?.id; if(!id) return;
      // Запросим актуальные данные списка (минимально)
      // Для простоты обновим всю доску, если список не найден
      renderBoard(state.currentBoardId);
      break;
    }
    case 'list.moved': {
      const id = ev.payload?.id; if(!id) return;
      // Плавная локальная перестановка колонки без полной перерисовки
      const listsContainer = els.lists;
      const cols = [...listsContainer.querySelectorAll('.list')];
      const target = cols.find(x => +x.dataset.id === id);
      if(!target) { renderBoard(state.currentBoardId); break; }
      // Получим текущий порядок из DOM как эталон и оставим его; серверный порядок придёт позднее через /full при следующей загрузке.
      // Здесь переставим элемент на позицию new_index, если прислан.
      const idx = ev.payload?.new_index;
      if(typeof idx === 'number'){
        const siblings = cols.filter(x => x !== target);
        const beforeEl = idx >= siblings.length ? null : siblings[idx];
        if(beforeEl){ listsContainer.insertBefore(target, beforeEl); } else { listsContainer.appendChild(target); }
      }
      break;
    }
    case 'board.moved': {
      // Обновить порядок в сайдбаре без полной перезагрузки
      const id = ev.payload?.id; if(!id) return;
      const li = [...els.boards.children].find(x => +x.dataset.id === id); if(!li) return;
      const idx = ev.payload?.new_index;
      if(typeof idx === 'number'){
        const siblings = [...els.boards.children].filter(x => x !== li);
        const beforeEl = idx >= siblings.length ? null : siblings[idx];
        if(beforeEl){ els.boards.insertBefore(li, beforeEl); } else { els.boards.appendChild(li); }
      }
      break;
    }
    case 'list.deleted': {
      const id = ev.payload?.id; if(!id) return;
      state.lists = state.lists.filter(x => x.id !== id); state.cards.delete(id);
      const col = els.lists.querySelector(`section.list[data-id='${id}']`); if(col) col.remove();
      break;
    }
    case 'card.created': {
      const c = ev.payload; if(!c) return;
      if(!state.cards.has(c.list_id)) state.cards.set(c.list_id, []);
      const arr = state.cards.get(c.list_id);
      if(!arr.some(x => x.id === c.id)){
        arr.push(c);
        const cardsEl = document.querySelector(`.cards[data-list-id="${c.list_id}"]`);
        if(cardsEl) cardsEl.appendChild(renderCard(c));
      }
      break;
    }
    case 'card.updated':
  case 'card.assignee_changed':
    case 'card.moved':
    case 'comment.created': {
      // Для простоты: переотрисовать текущую доску, чтобы синхронизировать позиции и новые данные
      renderBoard(state.currentBoardId);
      break;
    }
    case 'board.updated': {
      refreshBoards();
      break;
    }
  }
}

function buildListColumn(l){
  const col = document.createElement('section'); col.className='list'; col.dataset.id = l.id;
  col.innerHTML = `
    <header>
      <span class="drag-handle" title="Перетащить список" aria-label="Перетащить">
        <svg aria-hidden="true"><use href="#i-grip"></use></svg>
      </span>
      <h3 contenteditable="true" spellcheck="false">${escapeHTML(l.title)}</h3>
      <div class="spacer"></div>
  <button class="btn icon btn-color" title="Цвет списка" aria-label="Цвет"><svg aria-hidden="true"><use href="#i-palette"></use></svg></button>
  <button class="btn icon btn-del-list" title="Удалить список" aria-label="Удалить список">
        <svg aria-hidden="true"><use href="#i-trash" xlink:href="#i-trash"></use></svg>
      </button>
    </header>
    <div class="cards" data-list-id="${l.id}"></div>
    <div class="add-card"><button class="btn btn-add-card">Добавить карточку</button></div>`;

  const titleEl = col.querySelector('h3');
  titleEl.addEventListener('blur', async () => {
    const title = titleEl.textContent.trim(); const old = l.title;
    if(title && title !== old){
      try { await api.updateList(l.id, {title}); l.title = title; }
      catch(err){ alert('Не удалось сохранить заголовок: ' + err.message); titleEl.textContent = old; }
    } else { titleEl.textContent = old; }
  });

  col.querySelector('.btn-del-list').addEventListener('click', async () => {
    const ok = await confirmDialog('Удалить список и его карточки?');
    if(!ok) return;
    try {
      await api.deleteList(l.id);
      state.lists = state.lists.filter(x => x.id !== l.id);
      state.cards.delete(l.id);
      col.remove();
    } catch(err){ alert('Не удалось удалить список: ' + err.message); }
  });

  col.querySelector('.btn-add-card').addEventListener('click', () => {
  els.formCard.reset();
  if(els.cardDueAt){ els.cardDueAt.value=''; delete els.cardDueAt.dataset.iso; }
  els.formCard.dataset.listId = l.id; els.dlgCard.returnValue=''; els.dlgCard.showModal();
  });

  // Apply list color and color picker handler
  if(l.color){ col.style.setProperty('--clr', l.color); }
  const listColorBtn = col.querySelector('.btn-color');
  if(listColorBtn){
    listColorBtn.addEventListener('click', async (ev) => {
      ev.stopPropagation();
      const color = await pickColor(l.color || '');
      if(color === undefined) return;
      try { await api.updateList(l.id, { color: color || '' }); l.color = color || ''; if(l.color) col.style.setProperty('--clr', l.color); else col.style.removeProperty('--clr'); }
      catch(err){ alert('Не удалось сохранить цвет списка: ' + err.message); }
    });
  }

  const cardsEl = col.querySelector('.cards');
  for(const c of (state.cards.get(l.id)||[])){ cardsEl.appendChild(renderCard(c)); }
  enableCardsDnD(cardsEl);
  enableListsDnD(col);
  return col;
}

function renderLists(){
  els.lists.innerHTML = '';
  for(const l of state.lists){
    els.lists.appendChild(buildListColumn(l));
  }
}

function renderCard(c){
  const el = document.createElement('article');
  el.className = 'card'; el.draggable = true; el.dataset.id = c.id;
  // Assignee badge (if known)
  let assigneeHTML = '';
  if(c.assignee_id && state.currentBoardId && state.boardMembers.has(state.currentBoardId)){
    const u = (state.boardMembers.get(state.currentBoardId)||[]).find(x => x.id === c.assignee_id);
    if(u){ const initials = (u.name||u.email||'?').trim().slice(0,1).toUpperCase(); assigneeHTML = `<span class="assignee" title="Исполнитель: ${escapeHTML(u.name||u.email)}">${escapeHTML(initials)}</span>`; }
  }
  el.innerHTML = `<span class="ico"><svg aria-hidden="true"><use href="#i-card"></use></svg></span><div class="title">${escapeHTML(c.title)}</div><div class="spacer"></div>${assigneeHTML}<button class="btn icon btn-color" title="Цвет карточки" aria-label="Цвет"><svg aria-hidden="true"><use href="#i-palette"></use></svg></button>`;
  el.addEventListener('dblclick', () => openCard(c));
  if(c.color){ el.style.setProperty('--clr', c.color); }
  const colorBtn = el.querySelector('.btn-color');
  if(colorBtn){
    colorBtn.addEventListener('click', async (ev) => {
      ev.stopPropagation();
      const color = await pickColor(c.color || '');
      if(color === undefined) return;
      try { await api.updateCardFields(c.id, { color: color || '' }); c.color = color || ''; if(c.color) el.style.setProperty('--clr', c.color); else el.style.removeProperty('--clr'); }
      catch(err){ alert('Не удалось сохранить цвет карточки: ' + err.message); }
    });
  }
  return el;
}

async function openCard(c){
  state.currentCard = c;
  // Assignee select
  await populateAssigneeSelect();
  if(els.cvAssignee){ els.cvAssignee.value = c.assignee_id ? String(c.assignee_id) : '0'; }
  els.cvTitle.value = c.title || '';
  els.cvDescription.value = c.description || '';
  applyMdToggle(c);
  // Ensure UI reflects initial toggle state
  const btn = document.getElementById('cvMdToggle');
  if(btn){ btn.setAttribute('aria-pressed', String(!!c.description_is_md)); }
  renderCvDescriptionPreview(c);
  if(c.due_at){ const {text, iso} = toLocalTextAndISO(c.due_at); els.cvDueAt.value = text; els.cvDueAt.dataset.iso = iso; }
  else { els.cvDueAt.value=''; delete els.cvDueAt.dataset.iso; }
  // Открываем диалог сразу, чтобы сбой загрузки комментариев не блокировал редактирование
  els.cvComments.innerHTML = '';
  els.dlgCardView.showModal();
  try {
    await loadComments(c.id);
  } catch(err){
    // Тихо игнорируем ошибку, чтобы пользователь мог редактировать поля
    console.warn('Не удалось загрузить комментарии:', err);
  }
}

async function populateAssigneeSelect(){
  const wrap = document.getElementById('assigneeField');
  if(!els.cvAssignee){ return; }
  els.cvAssignee.innerHTML = '';
  const optNone = document.createElement('option'); optNone.value = '0'; optNone.textContent = (typeof t==='function'? t('app.dialogs.card.not_assigned') : '— Не назначен —'); els.cvAssignee.appendChild(optNone);
  const bid = state.currentBoardId;
  if(!bid){ if(wrap) wrap.hidden = true; return; }
  if(wrap) wrap.hidden = false;
  let members = state.boardMembers.get(bid);
  if(!members){ try{ members = await api.boardMembers(bid); state.boardMembers.set(bid, members||[]); }catch{ members = []; } }
  for(const u of members||[]){ const o = document.createElement('option'); o.value = String(u.id); o.textContent = u.name || u.email || ('#'+u.id); els.cvAssignee.appendChild(o); }
}

function applyMdToggle(c){
  // Create or update a toggle button next to description label
  const wrap = els.cvDescription.closest('label'); if(!wrap) return;
  let btn = document.getElementById('cvMdToggle');
  if(!btn){
    btn = document.createElement('button');
    btn.id = 'cvMdToggle'; btn.type = 'button'; btn.className = 'btn icon';
    btn.title = 'Markdown'; btn.setAttribute('aria-label','Markdown');
    btn.innerHTML = '<svg aria-hidden="true"><use href="#i-markdown"></use></svg>';
    wrap.appendChild(btn);
    btn.addEventListener('click', () => {
      const cur = btn.getAttribute('aria-pressed')==='true';
      btn.setAttribute('aria-pressed', String(!cur));
      renderCvDescriptionPreview(state.currentCard || c);
    });
  }
  btn.setAttribute('aria-pressed', String(!!c.description_is_md));
}

function renderCvDescriptionPreview(c){
  // Render markdown preview if toggle is on
  const btn = document.getElementById('cvMdToggle');
  const on = btn && btn.getAttribute('aria-pressed')==='true';
  // Ensure preview container exists
  let prev = document.getElementById('cvDescriptionPreview');
  if(!prev){ prev = document.createElement('div'); prev.id = 'cvDescriptionPreview'; prev.className='md-preview'; els.cvDescription.parentElement.appendChild(prev); }
  prev.hidden = !on;
  prev.style.display = on ? '' : 'none';
  els.cvDescription.style.display = on ? 'none' : '';
  if(on){ prev.innerHTML = renderMarkdownSafe(els.cvDescription.value || ''); }
}

function renderMarkdownSafe(src){
  // Basic Markdown renderer: safe HTML escaping, supports headings, paragraphs, lists, blockquotes,
  // inline code, bold/italic, links, and fenced code blocks. Not full CommonMark but practical.
  const escape = (t) => (t||'').replace(/[&<>]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]));
  const inline = (t) => {
    const codeSlots = [];
    let s = (t||'');
    // Extract code spans to placeholders so we don't alter inside
    s = s.replace(/`([^`]+)`/g, (m, g1) => {
      const idx = codeSlots.push(escape(g1)) - 1;
      return `\u0001CODE${idx}\u0002`;
    });
    // Escape the rest
    s = escape(s);
    // Links [text](http...)
    s = s.replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, (m, txt, url) => '<a href="'+url+'" target="_blank" rel="noopener noreferrer">'+txt+'</a>');
    // Bold (strong) then italic (em): only * delimiters (not _)
    s = s.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    s = s.replace(/(^|[^*])\*([^*]+?)\*(?=[^*]|$)/g, '$1<em>$2</em>');
    // Restore code spans
    s = s.replace(/\u0001CODE(\d+)\u0002/g, (m, n) => `<code>${codeSlots[Number(n)]||''}</code>`);
    return s;
  };

  const lines = (src||'').replace(/\r\n?/g,'\n').split('\n');
  let out = [];
  let i = 0, inCode = false, codeBuf = [], codeLang = '';
  let listType = null, listBuf = [];
  let bqBuf = [];

  const flushList = () => {
    if(!listType) return;
    const tag = listType === 'ol' ? 'ol' : 'ul';
    out.push('<'+tag+'>'+listBuf.join('')+'</'+tag+'>');
    listType = null; listBuf = [];
  };
  const flushBlockquote = () => {
    if(!bqBuf.length) return;
    const html = bqBuf.map(p => '<p>'+inline(escape(p))+'</p>').join('');
    out.push('<blockquote>'+html+'</blockquote>');
    bqBuf = [];
  };
  const flushParagraph = (buf) => {
    if(!buf.length) return;
    out.push('<p>'+inline(escape(buf.join(' ')))+'</p>');
  };

  while(i < lines.length){
    const line = lines[i];
    // Fenced code blocks
    const fence = line.match(/^\s*```\s*([\w-]*)\s*$/);
    if(fence){
      if(!inCode){ inCode = true; codeLang = fence[1]||''; codeBuf = []; i++; continue; }
      // closing fence
      out.push('<pre><code'+(codeLang?(' class="lang-'+codeLang+'"'):'')+'>'+escape(codeBuf.join('\n'))+'</code></pre>');
      inCode = false; codeBuf = []; codeLang=''; i++; continue;
    }
    if(inCode){ codeBuf.push(line); i++; continue; }

    // Horizontal rule
    if(/^\s{0,3}(?:-{3,}|\*{3,}|_{3,})\s*$/.test(line)){ flushList(); flushBlockquote(); out.push('<hr>'); i++; continue; }

    // Headings
    const h = line.match(/^\s{0,3}(#{1,6})\s+(.+)$/);
  if(h){ flushList(); flushBlockquote(); const lvl = h[1].length; out.push('<h'+lvl+'>'+inline(h[2].trim())+'</h'+lvl+'>'); i++; continue; }

    // Blockquote
    const bq = line.match(/^\s*>\s?(.*)$/);
    if(bq){ flushList(); bqBuf.push(bq[1]); i++; // accumulate
      // lookahead to collapse consecutive > lines
      while(i<lines.length && /^\s*>\s?/.test(lines[i])){ bqBuf.push(lines[i].replace(/^\s*>\s?/,'')); i++; }
      flushBlockquote();
      continue;
    }

    // Lists
    let m;
    if((m = line.match(/^\s*[-*]\s+(.+)$/))){
  if(listType !== 'ul'){ flushList(); listType = 'ul'; }
  listBuf.push('<li>'+inline(m[1])+'</li>'); i++;
      // absorb following list items of same kind
      while(i<lines.length){
        const m2 = lines[i].match(/^\s*[-*]\s+(.+)$/);
        if(!m2) break; listBuf.push('<li>'+inline(escape(m2[1]))+'</li>'); i++;
      }
      continue;
    }
    if((m = line.match(/^\s*\d+[\.)]\s+(.+)$/))){
  if(listType !== 'ol'){ flushList(); listType = 'ol'; }
  listBuf.push('<li>'+inline(m[1])+'</li>'); i++;
      while(i<lines.length){
        const m2 = lines[i].match(/^\s*\d+[\.)]\s+(.+)$/);
        if(!m2) break; listBuf.push('<li>'+inline(escape(m2[1]))+'</li>'); i++;
      }
      continue;
    }

    // Blank line separates paragraphs
    if(/^\s*$/.test(line)){ flushList(); flushBlockquote(); i++; continue; }

    // Paragraph (collect until blank line)
    const pbuf = [line.trim()]; i++;
    while(i<lines.length && !/^\s*$/.test(lines[i])){ pbuf.push(lines[i].trim()); i++; }
  flushList(); flushBlockquote(); flushParagraph(pbuf);
  }
  // flush leftovers
  flushList(); flushBlockquote();
  return out.join('\n');
}

async function loadComments(cardId){
  const comments = await api.getComments(cardId);
  els.cvComments.innerHTML = '';
  for(const cm of comments){
    const li = document.createElement('li');
    const when = new Date(cm.created_at).toLocaleString();
    li.textContent = `${when}: ${cm.body}`;
    els.cvComments.appendChild(li);
  }
}

function toLocalDatetimeInput(iso){
  const d = new Date(iso);
  const pad = (n) => n.toString().padStart(2,'0');
  const yyyy = d.getFullYear(); const MM = pad(d.getMonth()+1); const dd = pad(d.getDate());
  const hh = pad(d.getHours()); const mm = pad(d.getMinutes());
  return `${yyyy}-${MM}-${dd}T${hh}:${mm}`;
}

function toLocalTextAndISO(iso){
  const d = new Date(iso);
  const pad = (n) => n.toString().padStart(2,'0');
  const text = `${pad(d.getDate())}.${pad(d.getMonth()+1)}.${d.getFullYear()}  ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  return { text, iso: d.toISOString() };
}

async function openDateTimePicker(initialISO){
  return new Promise((resolve) => {
    const now = initialISO ? new Date(initialISO) : new Date();
    let view = new Date(now.getFullYear(), now.getMonth(), 1);
    let selected = initialISO ? new Date(initialISO) : null;
    // fill hours/minutes
    els.dtHour.innerHTML = ''; els.dtMinute.innerHTML='';
    for(let h=0; h<24; h++){ const o=document.createElement('option'); o.value=String(h); o.textContent=h.toString().padStart(2,'0'); els.dtHour.appendChild(o); }
    for(let m=0; m<60; m+=1){ const o=document.createElement('option'); o.value=String(m); o.textContent=m.toString().padStart(2,'0'); els.dtMinute.appendChild(o); }
    if(selected){ els.dtHour.value=String(selected.getHours()); els.dtMinute.value=String(selected.getMinutes()); }
    else { els.dtHour.value=String(now.getHours()); els.dtMinute.value=String(Math.floor(now.getMinutes()/5)*5); }

    const renderMonth = () => {
      const monthNames = (typeof t==='function'? t('app.dialogs.datetime.month_names', null, true) : null) || ['Январь','Февраль','Март','Апрель','Май','Июнь','Июль','Август','Сентябрь','Октябрь','Ноябрь','Декабрь'];
      els.dtMonth.textContent = `${monthNames[view.getMonth()]} ${view.getFullYear()}`;
      const start = new Date(view.getFullYear(), view.getMonth(), 1);
      const end = new Date(view.getFullYear(), view.getMonth()+1, 0);
      const startDay = (start.getDay()+6)%7; // Mon=0
      els.dtGrid.innerHTML = '';
      const dows = (typeof t==='function'? t('app.dialogs.datetime.dow', null, true) : null) || ['Пн','Вт','Ср','Чт','Пт','Сб','Вс'];
      for(const n of dows){ const s=document.createElement('div'); s.className='dow'; s.textContent=n; els.dtGrid.appendChild(s); }
      for(let i=0;i<startDay;i++){ const sp=document.createElement('div'); els.dtGrid.appendChild(sp); }
      const today = new Date(); today.setHours(0,0,0,0);
      for(let day=1; day<=end.getDate(); day++){
        const btn = document.createElement('button'); btn.type='button'; btn.textContent=String(day);
        const cur = new Date(view.getFullYear(), view.getMonth(), day);
        if(selected){ const s=new Date(selected); s.setHours(0,0,0,0); if(s.getTime()===cur.getTime()) btn.classList.add('selected'); }
        const cur0 = new Date(cur); cur0.setHours(0,0,0,0);
        if(cur0.getTime()===today.getTime()) btn.classList.add('today');
        btn.addEventListener('click', () => {
          selected = new Date(cur.getFullYear(), cur.getMonth(), cur.getDate(), parseInt(els.dtHour.value,10)||0, parseInt(els.dtMinute.value,10)||0);
          // Update selection highlight
          [...els.dtGrid.querySelectorAll('button')].forEach(b=>b.classList.remove('selected'));
          btn.classList.add('selected');
        });
        els.dtGrid.appendChild(btn);
      }
    };
    renderMonth();
    const onPrev = () => { view = new Date(view.getFullYear(), view.getMonth()-1, 1); renderMonth(); };
    const onNext = () => { view = new Date(view.getFullYear(), view.getMonth()+1, 1); renderMonth(); };
    els.dtPrev.onclick = onPrev; els.dtNext.onclick = onNext;
    els.btnDtOk.onclick = () => {
      if(!selected){ const base = initialISO ? new Date(initialISO) : new Date(); selected = new Date(view.getFullYear(), view.getMonth(), 1, parseInt(els.dtHour.value,10)||0, parseInt(els.dtMinute.value,10)||0); }
      els.dlgDateTime.close('ok'); resolve(selected.toISOString());
    };
    els.btnDtCancel.onclick = () => { els.dlgDateTime.close('cancel'); resolve(undefined); };
    els.btnDtClear.onclick = () => { els.dlgDateTime.close('ok'); resolve(''); };
    els.dlgDateTime.returnValue=''; els.dlgDateTime.showModal();
  });
}

function enableCardsDnD(container){
  container.addEventListener('dragstart', e => {
    const card = e.target.closest('.card'); if(!card) return; card.classList.add('dragging');
    e.dataTransfer.setData('text/plain', card.dataset.id);
  });
  container.addEventListener('dragend', e => { const card = e.target.closest('.card'); if(card) card.classList.remove('dragging'); });
  container.addEventListener('dragover', e => {
    e.preventDefault();
    const afterEl = getDragAfterElement(container, e.clientY);
    const dragging = document.querySelector('.card.dragging'); if(!dragging) return;
    if(afterEl == null) container.appendChild(dragging); else container.insertBefore(dragging, afterEl);
  });
  container.addEventListener('drop', async e => {
    e.preventDefault();
    const dragging = document.querySelector('.card.dragging'); if(!dragging) return; dragging.classList.remove('dragging');
    const cardId = parseInt(dragging.dataset.id, 10);
    const listId = parseInt(container.dataset.listId, 10);
    const newIndex = [...container.querySelectorAll('.card')].findIndex(x => x.dataset.id == dragging.dataset.id);
    try { await api.moveCard(cardId, listId, newIndex); } catch(err){ alert('Не удалось переместить: ' + err.message); }
  });
}

function getDragAfterElement(container, y){
  const els = [...container.querySelectorAll('.card:not(.dragging)')];
  let closest = {offset: Number.NEGATIVE_INFINITY, el: null};
  for(const el of els){
    const box = el.getBoundingClientRect();
    const offset = y - box.top - box.height / 2;
    if(offset < 0 && offset > closest.offset){ closest = {offset, el}; }
  }
  return closest.el;
}

function enableListsDnD(listColumn){
  // Разрешаем перетаскивание только за заголовок или хэндл
  const header = listColumn.querySelector('header');
  const handle = listColumn.querySelector('.drag-handle');
  listColumn.draggable = false;
  if(handle) handle.style.cursor = 'grab';
  // Старт перетаскивания только от иконки-хэндла
  if(handle){
    handle.addEventListener('click', e => e.stopPropagation());
    handle.addEventListener('mousedown', () => { listColumn.draggable = true; listColumn.dataset.dragAllowed = '1'; });
  }
  // Разрешаем dragstart только если инициирован с хэндла (и не карточка)
  listColumn.addEventListener('dragstart', e => {
    if(e.target && e.target.closest && e.target.closest('.card')){
      // это перетаскивание карточки — не трогаем
      return;
    }
    if(listColumn.dataset.dragAllowed !== '1'){ e.preventDefault(); listColumn.draggable = false; return; }
    delete listColumn.dataset.dragAllowed;
    listColumn.classList.add('dragging');
    e.dataTransfer.setData('text/plain', listColumn.dataset.id);
  });
  listColumn.addEventListener('dragend', async e => {
    if(state.dragListCrossDrop){
      // already handled by sidebar drop; reset and skip in-board move
      state.dragListCrossDrop = false; listColumn.classList.remove('dragging'); listColumn.draggable = false; return;
    }
    listColumn.classList.remove('dragging');
    // compute new index
    const siblings = [...document.querySelectorAll('.lists .list')];
    const newIndex = siblings.findIndex(x => x.dataset.id == listColumn.dataset.id);
    const id = parseInt(listColumn.dataset.id, 10);
    try { await api.moveList(id, newIndex, state.currentBoardId); } catch(err){ console.warn('Не удалось переместить список:', err.message); }
    listColumn.draggable = false;
  });
  const container = document.querySelector('.lists');
  if(!container.dataset.dndListsAttached){
    container.addEventListener('dragover', e => {
      const dragging = document.querySelector('.list.dragging'); if(!dragging) return;
      e.preventDefault();
      const after = getDragAfterList(container, e.clientX);
      if(after == null) container.appendChild(dragging); else container.insertBefore(dragging, after);
    });
    container.dataset.dndListsAttached = '1';
  }
}

function getDragAfterList(container, x){
  const els = [...container.querySelectorAll('.list:not(.dragging)')];
  let closest = {offset: Number.NEGATIVE_INFINITY, el: null};
  for(const el of els){
    const box = el.getBoundingClientRect();
    const offset = x - box.left - box.width / 2;
    if(offset < 0 && offset > closest.offset){ closest = {offset, el}; }
  }
  return closest.el;
}

function enableBoardsDnD(){
  const container = els.boards;
  [...container.children].forEach(li => {
    li.draggable = false; li.classList.add('board-item');
    const handle = li.querySelector('.drag-handle');
    if(handle){
      handle.style.cursor = 'grab';
      handle.addEventListener('click', e => e.stopPropagation());
      handle.addEventListener('mousedown', () => { li.draggable = true; li.dataset.dragAllowed = '1'; });
    }
    li.addEventListener('dragstart', e => {
      if(li.dataset.dragAllowed !== '1'){ e.preventDefault(); li.draggable = false; return; }
      delete li.dataset.dragAllowed;
      li.classList.add('dragging');
      e.dataTransfer.setData('text/plain', li.dataset.id);
  if(e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
    });
    li.addEventListener('dragend', async () => {
      li.classList.remove('dragging');
      const siblings = [...container.querySelectorAll('li')];
      const newIndex = siblings.findIndex(x => x.dataset.id == li.dataset.id);
      const id = parseInt(li.dataset.id, 10);
      try { await api.moveBoard(id, newIndex); } catch(err){ console.warn('Не удалось переместить доску:', err.message); }
      li.draggable = false;
    });
  });
  container.addEventListener('dragover', e => {
    e.preventDefault();
  if(e.dataTransfer) e.dataTransfer.dropEffect = 'move';
    const dragging = container.querySelector('li.dragging'); if(!dragging) return;
    const after = getDragAfterBoard(container, e.clientY);
    if(after == null) container.appendChild(dragging); else container.insertBefore(dragging, after);
  });

  // Accept dropping list headers onto boards to move list between boards
  container.addEventListener('dragover', e => {
    const draggingList = document.querySelector('.lists .list.dragging');
    if(!draggingList) return; e.preventDefault();
  });
  container.addEventListener('drop', async e => {
    e.preventDefault(); e.stopPropagation();
    const draggingList = document.querySelector('.lists .list.dragging'); if(!draggingList) return;
    const li = e.target.closest('li'); if(!li) return;
    const targetBoardId = parseInt(li.dataset.id, 10);
    const listId = parseInt(draggingList.dataset.id, 10);
    try {
      // move to end by default when moved to another board
      await api.moveList(listId, 1<<30, targetBoardId);
      state.dragListCrossDrop = true;
    } catch(err){ console.warn('Не удалось переместить список между досками:', err.message); }
  });
}

function getDragAfterBoard(container, y){
  const els = [...container.querySelectorAll('li:not(.dragging)')];
  let closest = {offset: Number.NEGATIVE_INFINITY, el: null};
  for(const el of els){
    const box = el.getBoundingClientRect();
    const offset = y - box.top - box.height / 2;
    if(offset < 0 && offset > closest.offset){ closest = {offset, el}; }
  }
  return closest.el;
}
function escapeHTML(s){ return s.replace(/[&<>'"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c])); }
