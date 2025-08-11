// Admin Panel JavaScript
const adminApi = {
  // Users
  async listUsers(q, limit) {
    const p = new URLSearchParams();
    if (q) p.set('q', q);
    if (limit) p.set('limit', String(limit));
    return fetchJSON(`/api/admin/users${p.toString() ? ('?' + p.toString()) : ''}`);
  },
  async createUser(payload) {
    return fetchJSON('/api/admin/users', { method: 'POST', body: payload });
  },
  async updateUser(id, payload) {
    return fetchJSON(`/api/admin/users/${id}`, { method: 'PATCH', body: payload });
  },
  async deleteUser(id) {
    return fetchJSON(`/api/admin/users/${id}`, { method: 'DELETE' });
  },

  // Groups
  async listGroups() {
    return fetchJSON('/api/admin/groups');
  },
  async createGroup(name) {
    return fetchJSON('/api/admin/groups', { method: 'POST', body: { name } });
  },
  async deleteGroup(id) {
    return fetchJSON(`/api/admin/groups/${id}`, { method: 'DELETE' });
  },
  async getGroupUsers(id) {
    return fetchJSON(`/api/admin/groups/${id}/users`);
  },
  async addUserToGroup(groupId, userId) {
    return fetchJSON(`/api/admin/groups/${groupId}/users`, { method: 'POST', body: { user_id: userId } });
  },
  async removeUserFromGroup(groupId, userId) {
    return fetchJSON(`/api/admin/groups/${groupId}/users/${userId}`, { method: 'DELETE' });
  },

  // Auth & Providers
  async me() {
    return fetchJSON('/api/auth/me');
  },
  async logout() {
    return fetchJSON('/api/auth/logout', { method: 'POST' });
  },
  async getProviders() {
    return fetchJSON('/api/auth/providers');
  }
};

// Shared fetch function
async function fetchJSON(url, opts = {}) {
  const init = { headers: { 'Content-Type': 'application/json' } };
  if (opts.method) init.method = opts.method;
  if (opts.body) init.body = JSON.stringify(opts.body);
  
  const res = await fetch(url, init);
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const j = await res.json();
      if (j && j.error) msg = j.error;
    } catch {}
    if (res.status === 401) {
      location.href = '/web/login.html';
      return Promise.reject(new Error('unauthorized'));
    }
    throw new Error(msg);
  }
  
  if (res.status === 204) return null;
  const text = await res.text();
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

// Global state
const adminState = {
  user: null,
  users: [],
  groups: [],
  currentTab: 'users'
};

// DOM helpers
const $ = (id) => document.getElementById(id);
const $$ = (selector) => document.querySelectorAll(selector);

// Utility functions
function formatDate(dateStr) {
  if (!dateStr) return '—';
  return new Date(dateStr).toLocaleDateString('ru-RU');
}

function showStatus(elementId, message, type = 'info') {
  const el = $(elementId);
  if (!el) return;
  el.textContent = message;
  el.className = `form-status ${type}`;
  setTimeout(() => {
    el.textContent = '';
    el.className = 'form-status';
  }, 3000);
}

function confirmDialog(message) {
  return new Promise((resolve) => {
    $('confirmMessage').textContent = message || 'Вы уверены?';
    $('dlgConfirm').showModal();
    
    const handleResolve = (result) => {
      $('dlgConfirm').close();
      resolve(result);
    };
    
    const btnOk = $('btnConfirmOk');
    const btnCancel = $('btnCancelConfirm');
    
    const onOk = () => {
      btnOk.removeEventListener('click', onOk);
      btnCancel.removeEventListener('click', onCancel);
      handleResolve(true);
    };
    
    const onCancel = () => {
      btnOk.removeEventListener('click', onOk);
      btnCancel.removeEventListener('click', onCancel);
      handleResolve(false);
    };
    
    btnOk.addEventListener('click', onOk);
    btnCancel.addEventListener('click', onCancel);
  });
}

// Tab management
function switchTab(tabName) {
  // Update tab buttons
  $$('.tab-btn').forEach(btn => btn.classList.remove('active'));
  $(`tab${tabName.charAt(0).toUpperCase() + tabName.slice(1)}`).classList.add('active');
  
  // Update tab content
  $$('.tab-content').forEach(content => content.classList.remove('active'));
  $(`${tabName}Tab`).classList.add('active');
  
  adminState.currentTab = tabName;
  
  // Load data for the tab
  switch (tabName) {
    case 'users':
      loadUsers();
      break;
    case 'groups':
      loadGroups();
      break;
    case 'settings':
      loadSettings();
      break;
  }
}

// Users management
async function loadUsers(query = '') {
  try {
    const users = await adminApi.listUsers(query, 100);
    console.log('API response for users:', users);
    adminState.users = Array.isArray(users) ? users : (users.users || []);
    console.log('Parsed users:', adminState.users);
    renderUsers();
  } catch (err) {
    console.error('Failed to load users:', err);
    $('usersTableBody').innerHTML = `<tr><td colspan="6" class="loading">Ошибка загрузки: ${err.message}</td></tr>`;
  }
}

function renderUsers() {
  const tbody = $('usersTableBody');
  if (!adminState.users.length) {
    tbody.innerHTML = '<tr><td colspan="6" class="loading">Пользователи не найдены</td></tr>';
    return;
  }
  
  tbody.innerHTML = adminState.users.map(user => `
    <tr>
      <td>${user.id}</td>
      <td>${escapeHtml(user.name)}</td>
      <td>${escapeHtml(user.email)}</td>
      <td>${formatDate(user.created_at)}</td>
      <td><span class="status ${user.is_admin ? 'active' : 'inactive'}">${user.is_admin ? 'Да' : 'Нет'}</span></td>
      <td>
        <div class="table-actions">
          <button class="btn btn-sm btn-edit" data-action="edit" data-id="${user.id}">Изменить</button>
          <button class="btn btn-sm btn-delete" data-action="delete" data-id="${user.id}">Удалить</button>
        </div>
      </td>
    </tr>
  `).join('');

  // Bind row actions
  tbody.querySelectorAll('button[data-action]').forEach(btn => {
    btn.addEventListener('click', (e) => {
      const id = Number(e.currentTarget.getAttribute('data-id'));
      const act = e.currentTarget.getAttribute('data-action');
      if (act === 'edit') editUser(id);
      if (act === 'delete') deleteUser(id);
    });
  });
}

function openUserDialog(user = null) {
  const isEdit = !!user;
  $('userDialogTitle').textContent = isEdit ? 'Редактировать пользователя' : 'Создать пользователя';
  $('userId').value = isEdit ? user.id : '';
  $('userName').value = isEdit ? user.name : '';
  $('userEmail').value = isEdit ? user.email : '';
  $('userPassword').value = '';
  $('userIsAdmin').checked = isEdit ? user.is_admin : false;
  
  // Hide password field for existing users
  const passwordField = $('passwordField');
  if (passwordField) {
    passwordField.style.display = isEdit ? 'none' : 'block';
  }
  $('userPassword').required = !isEdit;
  
  // Clear any previous status
  const statusEl = $('userFormStatus');
  if (statusEl) {
    statusEl.textContent = '';
    statusEl.className = 'form-status';
  }
  
  $('dlgUser').showModal();
}

async function saveUser() {
  const form = $('formUser');
  const formData = new FormData(form);
  const userId = formData.get('id');
  const isEdit = !!userId;

  const payload = {
    name: formData.get('name'),
    email: formData.get('email'),
    is_admin: formData.has('is_admin')
  };

  if (!isEdit) {
    payload.password = formData.get('password');
  }

  try {
    if (isEdit) {
      await adminApi.updateUser(userId, payload);
      showStatus('userFormStatus', 'Пользователь обновлен', 'success');
    } else {
      await adminApi.createUser(payload);
      showStatus('userFormStatus', 'Пользователь создан', 'success');
    }
    // brief delay to show success then close
    setTimeout(() => {
      $('dlgUser').close();
      loadUsers();
    }, 250);
  } catch (err) {
    showStatus('userFormStatus', `Ошибка: ${err.message}`, 'error');
  }
}

async function editUser(userId) {
  const user = adminState.users.find(u => u.id === userId);
  if (user) {
    openUserDialog(user);
  }
}

async function deleteUser(userId) {
  const user = adminState.users.find(u => u.id === userId);
  if (!user) return;
  
  const confirmed = await confirmDialog(`Удалить пользователя "${user.name}"?`);
  if (!confirmed) return;
  
  try {
    await adminApi.deleteUser(userId);
    showStatus('membersStatus', 'Пользователь удален', 'success');
    loadUsers();
  } catch (err) {
    showStatus('membersStatus', `Ошибка удаления: ${err.message}`, 'error');
  }
}

// Groups management
async function loadGroups() {
  try {
    const groups = await adminApi.listGroups();
    console.log('API response for groups:', groups);
    adminState.groups = Array.isArray(groups) ? groups : (groups.groups || []);
    console.log('Parsed groups:', adminState.groups);
    renderGroups();
  } catch (err) {
    console.error('Failed to load groups:', err);
    $('groupsTableBody').innerHTML = `<tr><td colspan="5" class="loading">Ошибка загрузки: ${err.message}</td></tr>`;
  }
}

function renderGroups() {
  const tbody = $('groupsTableBody');
  if (!adminState.groups.length) {
    tbody.innerHTML = '<tr><td colspan="5" class="loading">Группы не найдены</td></tr>';
    return;
  }
  
  tbody.innerHTML = adminState.groups.map(group => `
    <tr>
      <td>${group.id}</td>
      <td>${escapeHtml(group.name)}</td>
      <td>${group.member_count || 0}</td>
      <td>${formatDate(group.created_at)}</td>
      <td>
        <div class="table-actions">
          <button class="btn btn-sm btn-members" data-action="members" data-id="${group.id}">Участники</button>
          <button class="btn btn-sm btn-delete" data-action="delete" data-id="${group.id}">Удалить</button>
        </div>
      </td>
    </tr>
  `).join('');

  // Bind row actions
  tbody.querySelectorAll('button[data-action]').forEach(btn => {
    btn.addEventListener('click', (e) => {
      const id = Number(e.currentTarget.getAttribute('data-id'));
      const act = e.currentTarget.getAttribute('data-action');
      if (act === 'members') {
        const row = e.currentTarget.closest('tr');
        const nameCell = row && row.children[1] ? row.children[1].textContent : '';
        openGroupMembers(id, nameCell || 'Группа');
      } else if (act === 'delete') {
        deleteGroup(id);
      }
    });
  });
}

function openGroupDialog(group = null) {
  const isEdit = !!group;
  $('groupDialogTitle').textContent = isEdit ? 'Редактировать группу' : 'Создать группу';
  $('groupId').value = isEdit ? group.id : '';
  $('groupName').value = isEdit ? group.name : '';
  
  // Clear any previous status
  const statusEl = $('groupFormStatus');
  if (statusEl) {
    statusEl.textContent = '';
    statusEl.className = 'form-status';
  }
  
  $('dlgGroup').showModal();
}

async function saveGroup() {
  const form = $('formGroup');
  const formData = new FormData(form);
  const name = formData.get('name');
  
  try {
    await adminApi.createGroup(name);
    showStatus('groupFormStatus', 'Группа создана', 'success');
    // brief delay to show success then close
    setTimeout(() => {
      $('dlgGroup').close();
      loadGroups();
    }, 250);
  } catch (err) {
    showStatus('groupFormStatus', `Ошибка: ${err.message}`, 'error');
  }
}

async function deleteGroup(groupId) {
  const group = adminState.groups.find(g => g.id === groupId);
  if (!group) return;
  
  const confirmed = await confirmDialog(`Удалить группу "${group.name}"?`);
  if (!confirmed) return;
  
  try {
    await adminApi.deleteGroup(groupId);
    showStatus('membersStatus', 'Группа удалена', 'success');
    loadGroups();
  } catch (err) {
    showStatus('membersStatus', `Ошибка удаления: ${err.message}`, 'error');
  }
}

// Group members management
async function openGroupMembers(groupId, groupName) {
  $('groupMembersTitle').textContent = groupName;
  $('dlgGroupMembers').showModal();
  
  try {
    const users = await adminApi.getGroupUsers(groupId);
    const members = Array.isArray(users) ? users : (users.users || []);
    renderGroupMembers(members, groupId);
  } catch (err) {
    showStatus('membersStatus', `Ошибка загрузки участников: ${err.message}`, 'error');
  }
}

function renderGroupMembers(members, groupId) {
  const list = $('membersList');
  if (!members.length) {
    list.innerHTML = '<div class="member-item-modern" style="justify-content: center; color: var(--muted);">Участников нет</div>';
    return;
  }
  
  list.innerHTML = members.map(member => `
    <div class="member-item-modern">
      <div class="member-info-modern">
        <div class="member-name-modern">${escapeHtml(member.name)}</div>
        <div class="member-email-modern">${escapeHtml(member.email)}</div>
      </div>
      <button class="btn-remove" onclick="removeFromGroup(${groupId}, ${member.id})">Удалить</button>
    </div>
  `).join('');
}

async function removeFromGroup(groupId, userId) {
  try {
    await adminApi.removeUserFromGroup(groupId, userId);
    showStatus('membersStatus', 'Участник удален', 'success');
    // Reload members
    const groupName = $('groupMembersTitle').textContent;
    openGroupMembers(groupId, groupName);
  } catch (err) {
    showStatus('membersStatus', `Ошибка: ${err.message}`, 'error');
  }
}

// Settings
async function loadSettings() {
  try {
    // Load OAuth providers
    const providers = await adminApi.getProviders();
    const hasGithub = providers.providers && providers.providers.some(p => p.id === 'github');
    $('githubStatus').textContent = hasGithub ? 'Настроен' : 'Не настроен';
    $('githubStatus').className = `status ${hasGithub ? 'active' : 'inactive'}`;
    
    // Load stats
    $('totalUsers').textContent = adminState.users.length;
    $('totalGroups').textContent = adminState.groups.length;
    $('totalBoards').textContent = '—'; // Would need additional API
  } catch (err) {
    console.error('Failed to load settings:', err);
  }
}

// Search functionality
let searchTimeout;
function setupSearch() {
  $('userSearch').addEventListener('input', (e) => {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => {
      loadUsers(e.target.value);
    }, 300);
  });
  
  $('memberSearch').addEventListener('input', (e) => {
    // Search for users to add to group - would need additional implementation
    console.log('Search members:', e.target.value);
  });
}

// HTML escaping
function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

// Event listeners
function setupEventListeners() {
  // Tab switching
  $$('.tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const tab = btn.getAttribute('data-tab');
      switchTab(tab);
    });
  });
  
  // Navigation
  $('btnBackToApp').addEventListener('click', () => {
    location.href = '/';
  });
  
  $('btnAdminLogout').addEventListener('click', async () => {
    try {
      await adminApi.logout();
      location.href = '/web/login.html';
    } catch (err) {
      alert('Ошибка выхода: ' + err.message);
    }
  });
  
  // User management
  $('btnCreateUser').addEventListener('click', () => openUserDialog());
  const formUser = document.getElementById('formUser');
  if (formUser) {
    formUser.addEventListener('submit', (e) => {
      e.preventDefault();
      saveUser();
    });
  }
  const btnCloseUserDialog = document.getElementById('btnCloseUserDialog');
  if (btnCloseUserDialog) {
    btnCloseUserDialog.addEventListener('click', () => $('dlgUser').close());
  }
  const btnCancelUserDialog = document.getElementById('btnCancelUserDialog');
  if (btnCancelUserDialog) {
    btnCancelUserDialog.addEventListener('click', () => $('dlgUser').close());
  }  // Group management
  $('btnCreateGroup').addEventListener('click', () => openGroupDialog());
  const formGroup = document.getElementById('formGroup');
  if (formGroup) {
    formGroup.addEventListener('submit', (e) => {
      e.preventDefault();
      saveGroup();
    });
  }
  const btnCloseGroupDialog = document.getElementById('btnCloseGroupDialog');
  if (btnCloseGroupDialog) {
    btnCloseGroupDialog.addEventListener('click', () => $('dlgGroup').close());
  }
  const btnCancelGroupDialog = document.getElementById('btnCancelGroupDialog');
  if (btnCancelGroupDialog) {
    btnCancelGroupDialog.addEventListener('click', () => $('dlgGroup').close());
  }

  // Group members dialog
  $('btnCloseMembersDialog').addEventListener('click', () => {
    $('dlgGroupMembers').close();
  });

  // Search
  setupSearch();
}

// Initialize admin panel
async function initAdmin() {
  try {
    // Check if user is admin
    const me = await adminApi.me();
    if (!me.user) {
      location.href = '/web/login.html';
      return;
    }
    
    if (!me.user.is_admin) {
      alert('Доступ запрещен. Требуются права администратора.');
      location.href = '/';
      return;
    }
    
    adminState.user = me.user;
    $('adminUserName').textContent = me.user.name;
    
    // Setup event listeners
    setupEventListeners();
    
    // Load initial data
    switchTab('users');
    
  } catch (err) {
    console.error('Admin init failed:', err);
    alert('Ошибка инициализации админки: ' + err.message);
    location.href = '/';
  }
}

// Start the admin panel
document.addEventListener('DOMContentLoaded', initAdmin);
