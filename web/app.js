const api = {
  async getBoards(){ return fetchJSON('/api/boards') },
  async createBoard(title){ return fetchJSON('/api/boards', {method:'POST', body:{title}}) },
  async getBoard(id){ return fetchJSON('/api/boards/'+id) },
  async getBoardFull(id){ return fetchJSON(`/api/boards/${id}/full`) },
  async updateBoard(id, title){ return fetchJSON(`/api/boards/${id}`, {method:'PATCH', body:{title}}) },
  async setBoardColor(id, color){ return fetchJSON(`/api/boards/${id}`, {method:'PATCH', body:{color}}) },
  async deleteBoard(id){ return fetchJSON(`/api/boards/${id}`, {method:'DELETE'}) },
  async getLists(bid){ return fetchJSON(`/api/boards/${bid}/lists`) },
  async createList(bid, title){ return fetchJSON(`/api/boards/${bid}/lists`, {method:'POST', body:{title}}) },
  async getCards(lid){ return fetchJSON(`/api/lists/${lid}/cards`) },
  async createCard(lid, title, description){ return fetchJSON(`/api/lists/${lid}/cards`, {method:'POST', body:{title, description}}) },
  async moveCard(id, targetListId, newIndex){ return fetchJSON(`/api/cards/${id}/move`, {method:'POST', body:{target_list_id: targetListId, new_index: newIndex}}) },
  async updateList(id, payload){ return fetchJSON(`/api/lists/${id}`, {method:'PATCH', body:payload}) },
  async moveList(id, newIndex, targetBoardId){ return fetchJSON(`/api/lists/${id}/move`, {method:'POST', body:{new_index: newIndex, target_board_id: targetBoardId||0}}) },
  async deleteList(id){ return fetchJSON(`/api/lists/${id}`, {method:'DELETE'}) },
  async getComments(cardId){ return fetchJSON(`/api/cards/${cardId}/comments`) },
  async addComment(cardId, body){ return fetchJSON(`/api/cards/${cardId}/comments`, {method:'POST', body:{body}}) },
  async updateCardFields(id, payload){ return fetchJSON(`/api/cards/${id}`, {method:'PATCH', body:payload}) },
  async deleteCard(id){ return fetchJSON(`/api/cards/${id}`, {method:'DELETE'}) },
  async moveBoard(id, newIndex){ return fetchJSON(`/api/boards/${id}/move`, {method:'POST', body:{new_index: newIndex}}) },
};

async function fetchJSON(url, opts={}){
  const init = {headers:{'Content-Type':'application/json'}};
  if(opts.method) init.method = opts.method;
  if(opts.body) init.body = JSON.stringify(opts.body);
  const res = await fetch(url, init);
  if(!res.ok){
    let msg = res.statusText;
    try { const j = await res.json(); if(j && j.error) msg = j.error } catch{}
    throw new Error(msg);
  }
  return res.json();
}

const state = { boards: [], currentBoardId: null, lists: [], cards: new Map(), currentCard: null, dragListCrossDrop: false };
const el = (id) => document.getElementById(id);
const els = { boards: el('boards'), boardTitle: el('boardTitle'), lists: el('lists'),
  dlgBoard: el('dlgBoard'), formBoard: el('formBoard'), dlgList: el('dlgList'),
  formList: el('formList'), dlgCard: el('dlgCard'), formCard: el('formCard'),
  btnNewBoard: el('btnNewBoard'), btnNewList: el('btnNewList'),
  btnRenameBoard: el('btnRenameBoard'), btnDeleteBoard: el('btnDeleteBoard'),
  dlgCardView: el('dlgCardView'), cvTitle: el('cvTitle'), cvDescription: el('cvDescription'),
  cvDueAt: el('cvDueAt'), cvComments: el('cvComments'), cvCommentText: el('cvCommentText'),
  btnAddComment: el('btnAddComment'), btnCloseCardView: el('btnCloseCardView'), btnSaveCardView: el('btnSaveCardView'),
  dlgConfirm: el('dlgConfirm'), formConfirm: el('formConfirm'), confirmMessage: el('confirmMessage'),
  dlgColor: el('dlgColor'), formColor: el('formColor'), colorCustom: el('colorCustom'),
  dlgInput: el('dlgInput'), formInput: el('formInput'), inputTitle: el('inputTitle'), inputLabel: el('inputLabel'), inputValue: el('inputValue'),
  dlgSelect: el('dlgSelect'), formSelect: el('formSelect'), selectTitle: el('selectTitle'), selectLabel: el('selectLabel'), selectControl: el('selectControl') };

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
    els.confirmMessage.textContent = message || 'Вы уверены?';
    // Ensure previous returnValue cleared
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
  const btnTheme = document.getElementById('btnTheme');
  if(btnTheme){
    btnTheme.innerHTML = '<svg><use href="#i-auto"></use></svg>';
    btnTheme.addEventListener('click', () => {
      const current = getPreferredTheme();
      const next = current === 'auto' ? 'light' : current === 'light' ? 'dark' : 'auto';
      localStorage.setItem('theme', next); setTheme(next);
    });
  }

  await refreshBoards(); bindUI(); setupContextMenu();
  if(state.boards.length) openBoard(state.boards[0].id);
}

function bindUI(){
  els.btnNewBoard.addEventListener('click', () => { els.formBoard.reset(); els.dlgBoard.returnValue=''; els.dlgBoard.showModal(); });
  els.dlgBoard.addEventListener('close', async () => {
    if(els.dlgBoard.returnValue === 'ok'){
      const fd = new FormData(els.formBoard);
      const title = fd.get('title').trim(); if(!title) return;
      const b = await api.createBoard(title); await refreshBoards(); openBoard(b.id);
    } else { els.formBoard.reset(); }
  });
  // Cancel buttons should behave like Esc (no validation)
  els.dlgBoard.querySelector('button[value="cancel"][type="button"]').addEventListener('click', () => { els.formBoard.reset(); els.dlgBoard.close('cancel'); });

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
      } catch(err){ alert('Не удалось создать список: ' + err.message); }
    } else { els.formList.reset(); }
  });
  els.dlgList.querySelector('button[value="cancel"][type="button"]').addEventListener('click', () => { els.formList.reset(); els.dlgList.close('cancel'); });

  els.dlgCard.addEventListener('close', async () => {
    if(els.dlgCard.returnValue !== 'ok') return;
    const fd = new FormData(els.formCard);
    const title = fd.get('title').trim();
    const description = (fd.get('description')||'').trim();
    const due_at = fd.get('due_at');
    const listId = parseInt(els.formCard.dataset.listId, 10);
    if(!title || !listId) return;
    try {
      const c = await api.createCard(listId, title, description);
      if(due_at){
        try { await api.updateCardFields(c.id, { due_at: new Date(due_at).toISOString() }); c.due_at = new Date(due_at).toISOString(); } catch{}
      }
      if(!state.cards.has(listId)) state.cards.set(listId, []);
      const arr = state.cards.get(listId);
      if(!arr.some(x => x.id === c.id)){
        arr.push(c);
        const cardsEl = document.querySelector(`.cards[data-list-id="${listId}"]`);
        if(cardsEl) cardsEl.appendChild(renderCard(c));
      }
    } catch(err){ alert('Не удалось создать карточку: ' + err.message); }
    els.formCard.reset(); delete els.formCard.dataset.listId;
  });
  els.dlgCard.querySelector('button[value="cancel"][type="button"]').addEventListener('click', () => { els.formCard.reset(); delete els.formCard.dataset.listId; els.dlgCard.close('cancel'); });

  // Board rename/delete
  els.btnRenameBoard.addEventListener('click', async () => {
    if(!state.currentBoardId) return;
    const current = els.boardTitle.textContent.trim();
  const name = await inputDialog({ title:'Переименование доски', label:'Новое название', value: current });
    if(!name || name.trim() === current) return;
    try { await api.updateBoard(state.currentBoardId, name.trim()); els.boardTitle.textContent = name.trim(); await refreshBoards(); }
    catch(err){ alert('Не удалось переименовать: ' + err.message); }
  });
  els.btnDeleteBoard.addEventListener('click', async () => {
    if(!state.currentBoardId) return;
    const ok = await confirmDialog('Удалить текущую доску безвозвратно?');
    if(!ok) return;
    try { await api.deleteBoard(state.currentBoardId); await refreshBoards(); state.currentBoardId = null; els.boardTitle.textContent = ''; els.lists.innerHTML=''; }
    catch(err){ alert('Не удалось удалить доску: ' + err.message); }
  });

  // Card view dialog
  els.btnCloseCardView.addEventListener('click', () => els.dlgCardView.close());
  els.btnSaveCardView.addEventListener('click', async () => {
    const c = state.currentCard; if(!c) return;
    const payload = {};
    const title = els.cvTitle.value.trim(); if(title && title !== c.title) payload.title = title;
    const description = els.cvDescription.value.trim(); if(description !== c.description) payload.description = description;
    const dueVal = els.cvDueAt.value; if(dueVal){ const iso = new Date(dueVal).toISOString(); payload.due_at = iso; }
    if(Object.keys(payload).length===0){ els.dlgCardView.close(); return; }
    try { await api.updateCardFields(c.id, payload); els.dlgCardView.close(); renderBoard(state.currentBoardId); }
    catch(err){ alert('Не удалось сохранить карточку: ' + err.message); }
  });
  els.btnAddComment.addEventListener('click', async () => {
    const c = state.currentCard; if(!c) return;
    const body = els.cvCommentText.value.trim(); if(!body) return;
    try { await api.addComment(c.id, body); els.cvCommentText.value=''; await loadComments(c.id); }
    catch(err){ alert('Не удалось добавить комментарий: ' + err.message); }
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
          catch(err){ alert('Не удалось сохранить цвет карточки: ' + err.message); }
        } },
      { label: '---' },
      { label: 'Удалить карточку', danger: true, action: async () => {
          const ok = await confirmDialog('Удалить карточку безвозвратно?'); if(!ok) return;
          try { await api.deleteCard(id); const el = document.querySelector(`.card[data-id="${id}"]`); if(el) el.remove(); const arr = state.cards.get(listId) || []; state.cards.set(listId, arr.filter(x=>x.id!==id)); }
          catch(err){ alert('Не удалось удалить карточку: ' + err.message); }
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
          catch(err){ alert('Не удалось переименовать: ' + err.message); }
        } },
  { label: 'Дубликат списка', action: async () => { await duplicateList(listId); } },
  { label: 'Переместить…', action: async () => { await moveListPrompt(listId); } },
      { label: 'Цвет…', action: async () => {
          const color = await pickColor(l?.color || ''); if(color === undefined) return;
          try { await api.updateList(listId, { color: color || '' }); if(l){ l.color = color || ''; } if(color) targetList.style.setProperty('--clr', color); else targetList.style.removeProperty('--clr'); }
          catch(err){ alert('Не удалось сохранить цвет списка: ' + err.message); }
        } },
      { label: '---' },
      { label: 'Удалить список', danger: true, action: async () => {
          const ok = await confirmDialog('Удалить список и его карточки?'); if(!ok) return;
          try { await api.deleteList(listId); state.lists = state.lists.filter(x => x.id !== listId); state.cards.delete(listId); targetList.remove(); }
          catch(err){ alert('Не удалось удалить список: ' + err.message); }
        } },
    ];
  }

  const targetBoardLi = e.target.closest('#boards li');
  if(targetBoardLi){
    const boardId = parseInt(targetBoardLi.dataset.id, 10);
    const b = state.boards.find(x => x.id === boardId);
    return [
      { label: 'Открыть доску', action: () => openBoard(boardId) },
    { label: 'Переименовать доску', action: async () => {
      const current = b?.title || targetBoardLi.querySelector('.t')?.textContent?.trim() || '';
      const name = await inputDialog({ title:'Переименование доски', label:'Новое название', value: current }); if(!name || !name.trim() || name.trim() === current) return;
          try { await api.updateBoard(boardId, name.trim()); if(b){ b.title = name.trim(); } const t = targetBoardLi.querySelector('.t'); if(t) t.textContent = name.trim(); await refreshBoards(); }
          catch(err){ alert('Не удалось переименовать: ' + err.message); }
        } },
      { label: 'Цвет…', action: async () => {
          const color = await pickColor(b?.color || ''); if(color === undefined) return;
          try { await api.setBoardColor(boardId, color || ''); if(b){ b.color = color || ''; } if(color) targetBoardLi.style.setProperty('--clr', color); else targetBoardLi.style.removeProperty('--clr'); }
          catch(err){ alert('Не удалось сохранить цвет доски: ' + err.message); }
        } },
      { label: '---' },
      { label: 'Удалить доску', danger: true, action: async () => {
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
      base.push(
        { label: 'Новый список', action: () => { if(!state.currentBoardId) return; els.formList.reset(); els.dlgList.returnValue=''; els.dlgList.showModal(); } },
    { label: 'Переименовать доску', action: async () => {
      const current = els.boardTitle.textContent.trim();
      const name = await inputDialog({ title:'Переименование доски', label:'Новое название', value: current }); if(!name || !name.trim() || name.trim() === current) return;
            try { await api.updateBoard(boardId, name.trim()); els.boardTitle.textContent = name.trim(); await refreshBoards(); }
            catch(err){ alert('Не удалось переименовать: ' + err.message); }
          } },
        { label: 'Цвет…', action: async () => {
            const color = await pickColor(b?.color || ''); if(color === undefined) return;
            try { await api.setBoardColor(boardId, color || ''); if(b){ b.color = color || ''; } await refreshBoards(); }
            catch(err){ alert('Не удалось сохранить цвет доски: ' + err.message); }
          } },
        { label: '---' },
        { label: 'Удалить доску', danger: true, action: async () => {
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
    const created = await api.createCard(listId, src.title || '', src.description || '');
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

async function refreshBoards(){
  state.boards = await api.getBoards();
  els.boards.innerHTML = '';
  for(const b of state.boards){
    const li = document.createElement('li');
  li.dataset.id = b.id;
  li.innerHTML = `<span class="drag-handle" title="Перетащить доску"><svg aria-hidden="true"><use href="#i-grip"></use></svg></span><span class="ico"><svg aria-hidden="true"><use href="#i-board"></use></svg></span><span class="t">${escapeHTML(b.title)}</span><button class="btn icon btn-color" title="Цвет доски" aria-label="Цвет"><svg aria-hidden="true"><use href="#i-palette"></use></svg></button>`;
    li.addEventListener('click', () => openBoard(b.id));
    if(b.color){ li.style.setProperty('--clr', b.color); }
    const colorBtn = li.querySelector('.btn-color');
    if(colorBtn){
      colorBtn.addEventListener('click', async (ev) => {
        ev.stopPropagation();
        const color = await pickColor(b.color || '');
        if(color === undefined) return;
        try { await api.setBoardColor(b.id, color || ''); b.color = color || ''; if(b.color) li.style.setProperty('--clr', b.color); else li.style.removeProperty('--clr'); }
        catch(err){ alert('Не удалось сохранить цвет: ' + err.message); }
      });
    }
    if(b.id === state.currentBoardId) li.classList.add('active');
    els.boards.appendChild(li);
  }
  enableBoardsDnD();
}

async function openBoard(id){ state.currentBoardId = id; await renderBoard(id);
  [...els.boards.children].forEach(li => li.classList.toggle('active', parseInt(li.dataset.id,10)===id)); }

let sse;
async function renderBoard(id){
  els.boardTitle.textContent = 'Загрузка...';
  try {
    const full = await api.getBoardFull(id);
    els.boardTitle.textContent = full.board.title;
    state.lists = full.lists || []; state.cards.clear();
    for(const l of state.lists){ state.cards.set(l.id, (full.cards && full.cards[l.id]) || []); }
    renderLists();
    // SSE subscribe
    if(sse) { sse.close(); sse = null; }
    sse = new EventSource(`/api/boards/${id}/events`);
    sse.onmessage = (e) => { try { onEvent(JSON.parse(e.data)); } catch{} };
  } catch(err){ els.boardTitle.textContent = 'Ошибка загрузки'; alert('Ошибка загрузки доски: ' + err.message); }
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
      <button class="btn icon btn-del-list" title="Удалить" aria-label="Удалить">
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
    els.formCard.reset(); els.formCard.dataset.listId = l.id; els.dlgCard.returnValue=''; els.dlgCard.showModal();
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
  el.innerHTML = `<span class="ico"><svg aria-hidden="true"><use href="#i-card"></use></svg></span><div class="title">${escapeHTML(c.title)}</div><div class="spacer"></div><button class="btn icon btn-color" title="Цвет карточки" aria-label="Цвет"><svg aria-hidden="true"><use href="#i-palette"></use></svg></button>`;
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
  els.cvTitle.value = c.title || '';
  els.cvDescription.value = c.description || '';
  els.cvDueAt.value = c.due_at ? toLocalDatetimeInput(c.due_at) : '';
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
  [...container.children].forEach(li => { li.draggable = false; li.classList.add('board-item'); });
  // drag only via handle
  container.querySelectorAll('.board-item .drag-handle').forEach(h => {
    h.style.cursor = 'grab';
    h.addEventListener('click', e => e.stopPropagation());
    h.addEventListener('mousedown', e => { const li = e.target.closest('li'); if(li){ li.draggable = true; li.dataset.dragAllowed = '1'; } });
  });
  container.addEventListener('dragstart', e => {
    const li = e.target.closest('li'); if(!li) return;
    if(li.dataset.dragAllowed !== '1'){ e.preventDefault(); li.draggable = false; return; }
    delete li.dataset.dragAllowed;
    li.classList.add('dragging');
    e.dataTransfer.setData('text/plain', li.dataset.id);
  });
  container.addEventListener('dragend', async e => {
    const li = e.target.closest('li'); if(!li) return; li.classList.remove('dragging');
    const siblings = [...container.querySelectorAll('li')];
    const newIndex = siblings.findIndex(x => x.dataset.id == li.dataset.id);
    const id = parseInt(li.dataset.id, 10);
    try { await api.moveBoard(id, newIndex); } catch(err){ console.warn('Не удалось переместить доску:', err.message); }
    li.draggable = false;
  });
  container.addEventListener('dragover', e => {
    e.preventDefault();
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
