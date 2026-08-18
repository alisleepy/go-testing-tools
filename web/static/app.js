// 测试工具集管理面板 - 前端逻辑
(function () {
    'use strict';

    const state = {
        currentTab: 'list',
        loggedIn: false,
        username: '',
        tools: [],
    };

    // ---------- 工具函数 ----------
    const $ = (sel) => document.querySelector(sel);
    const $$ = (sel) => document.querySelectorAll(sel);

    function toast(msg, type = 'info', duration = 2200) {
        const box = document.createElement('div');
        box.className = `toast ${type}`;
        box.textContent = msg;
        $('#toastContainer').appendChild(box);
        setTimeout(() => box.remove(), duration);
    }

    function escapeHtml(s) {
        if (s == null) return '';
        return String(s).replace(/[&<>"']/g, (c) => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
        }[c]));
    }

    function isValidURL(u) {
        try {
            const url = new URL(u);
            return url.protocol === 'http:' || url.protocol === 'https:';
        } catch { return false; }
    }

    async function api(path, options = {}) {
        const opts = { credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, ...options };
        if (opts.body && typeof opts.body !== 'string') opts.body = JSON.stringify(opts.body);
        const res = await fetch(path, opts);
        let data = null;
        const ct = res.headers.get('content-type') || '';
        if (ct.includes('application/json')) data = await res.json().catch(() => null);
        if (!res.ok) {
            const err = new Error((data && data.error) || `HTTP ${res.status}`);
            err.status = res.status;
            err.data = data;
            throw err;
        }
        return data;
    }

    // ---------- Tab 切换 ----------
    function switchTab(name) {
        state.currentTab = name;
        $$('.tab-btn').forEach(b => b.classList.toggle('active', b.dataset.tab === name));
        $$('.tab-panel').forEach(p => p.classList.toggle('active', p.id === `tab-${name}`));
        if (name === 'list') loadListTable();
        if (name === 'manage') {
            if (state.loggedIn) {
                $('#loginPanel').style.display = 'none';
                $('#managePanel').style.display = 'block';
                loadManageTable();
            } else {
                $('#loginPanel').style.display = 'flex';
                $('#managePanel').style.display = 'none';
            }
        }
    }

    // ---------- 登录相关 ----------
    async function checkAuth() {
        try {
            const r = await api('/api/check-auth');
            state.loggedIn = !!(r && r.logged_in);
            state.username = (r && r.username) || '';
        } catch {
            state.loggedIn = false;
        }
        renderUserArea();
    }

    function renderUserArea() {
        const el = $('#userArea');
        if (state.loggedIn) {
            el.innerHTML = `<span class="username">👤 ${escapeHtml(state.username)}</span><button class="btn btn-sm" id="btnLogout">退出登录</button>`;
            $('#btnLogout').addEventListener('click', doLogout);
        } else {
            el.innerHTML = '';
        }
    }

    async function doLogin(e) {
        e.preventDefault();
        const username = $('#loginUser').value.trim();
        const password = $('#loginPass').value;
        const errBox = $('#loginError');
        errBox.textContent = '';
        if (!username || !password) { errBox.textContent = '请输入账号和密码'; return; }
        try {
            await api('/api/login', { method: 'POST', body: { username, password } });
            state.loggedIn = true;
            state.username = username;
            renderUserArea();
            toast('登录成功', 'success');
            $('#loginForm').reset();
            switchTab('manage');
        } catch (err) {
            errBox.textContent = err.message || '登录失败';
        }
    }

    async function doLogout() {
        try { await api('/api/logout', { method: 'POST' }); } catch { /* ignore */ }
        state.loggedIn = false;
        state.username = '';
        renderUserArea();
        toast('已退出登录', 'info');
        switchTab('list');
    }

    // ---------- 工具列表 ----------
    async function loadListTable() {
        try {
            const data = await api('/api/tools');
            state.tools = data || [];
            renderListTable();
        } catch (err) {
            toast('加载失败：' + err.message, 'error');
        }
    }

    function renderListTable() {
        const tbody = $('#listTable tbody');
        tbody.innerHTML = '';
        if (!state.tools.length) { $('#listEmpty').style.display = 'block'; return; }
        $('#listEmpty').style.display = 'none';
        state.tools.forEach((t, i) => {
            const tr = document.createElement('tr');
            tr.innerHTML = `
                <td>${i + 1}</td>
                <td>${escapeHtml(t.name)}</td>
                <td>${escapeHtml(t.purpose)}</td>
                <td><a href="${escapeHtml(t.url)}" target="_blank" rel="noopener noreferrer">${escapeHtml(t.url)}</a></td>
                <td>${escapeHtml(t.remark || '')}</td>
            `;
            tbody.appendChild(tr);
        });
    }

    // ---------- 工具管理 ----------
    async function loadManageTable() {
        try {
            const data = await api('/api/tools');
            state.tools = data || [];
            renderManageTable();
        } catch (err) {
            if (err.status === 401) {
                state.loggedIn = false;
                renderUserArea();
                switchTab('manage');
                toast('会话已过期，请重新登录', 'error');
                return;
            }
            toast('加载失败：' + err.message, 'error');
        }
    }

    function renderManageTable() {
        const tbody = $('#manageTable tbody');
        tbody.innerHTML = '';
        if (!state.tools.length) { $('#manageEmpty').style.display = 'block'; return; }
        $('#manageEmpty').style.display = 'none';
        state.tools.forEach((t) => {
            const tr = document.createElement('tr');
            tr.dataset.id = t.id;
            tr.draggable = true;
            tr.innerHTML = `
                <td class="drag-handle" title="拖动排序">☰ ${t.id}</td>
                <td>${escapeHtml(t.name)}</td>
                <td>${escapeHtml(t.purpose)}</td>
                <td><a href="${escapeHtml(t.url)}" target="_blank" rel="noopener noreferrer">${escapeHtml(t.url)}</a></td>
                <td>${escapeHtml(t.remark || '')}</td>
                <td class="op-cell">
                    <button class="btn-link" data-act="edit" data-id="${t.id}">编辑</button>
                    <button class="btn-link danger" data-act="del" data-id="${t.id}">删除</button>
                </td>
            `;
            tbody.appendChild(tr);
        });
        bindManageRowEvents();
        bindDragEvents();
    }

    function bindManageRowEvents() {
        $$('#manageTable tbody button[data-act]').forEach(btn => {
            btn.addEventListener('click', () => {
                const id = Number(btn.dataset.id);
                const act = btn.dataset.act;
                if (act === 'edit') openEditModal(id);
                if (act === 'del') openDeleteModal(id);
            });
        });
    }

    // ---------- 新增 / 编辑 弹窗 ----------
    function openAddModal() {
        $('#editModalTitle').textContent = '新增工具';
        $('#editId').value = '';
        $('#editForm').reset();
        $('#editError').textContent = '';
        showModal('editModal');
        setTimeout(() => $('#editName').focus(), 50);
    }

    function openEditModal(id) {
        const t = state.tools.find(x => x.id === id);
        if (!t) return;
        $('#editModalTitle').textContent = '编辑工具';
        $('#editId').value = t.id;
        $('#editName').value = t.name;
        $('#editPurpose').value = t.purpose;
        $('#editURL').value = t.url;
        $('#editRemark').value = t.remark || '';
        $('#editError').textContent = '';
        showModal('editModal');
    }

    async function submitEditForm(e) {
        e.preventDefault();
        const id = $('#editId').value;
        const payload = {
            name: $('#editName').value.trim(),
            purpose: $('#editPurpose').value.trim(),
            url: $('#editURL').value.trim(),
            remark: $('#editRemark').value.trim(),
        };
        const errBox = $('#editError');
        errBox.textContent = '';
        if (!payload.name) { errBox.textContent = '工具名称不能为空'; return; }
        if (!payload.purpose) { errBox.textContent = '工具作用不能为空'; return; }
        if (!payload.url) { errBox.textContent = '工具地址不能为空'; return; }
        if (!isValidURL(payload.url)) { errBox.textContent = '请输入有效的URL地址（需以 http:// 或 https:// 开头）'; return; }

        try {
            if (id) {
                await api(`/api/tools/${id}`, { method: 'PUT', body: payload });
                toast('保存成功', 'success');
            } else {
                await api('/api/tools', { method: 'POST', body: payload });
                toast('新增成功', 'success');
            }
            hideModal('editModal');
            loadManageTable();
        } catch (err) {
            if (err.status === 401) { handleUnauthorized(); return; }
            errBox.textContent = err.message || '操作失败';
        }
    }

    // ---------- 删除 ----------
    let pendingDeleteId = null;
    function openDeleteModal(id) {
        const t = state.tools.find(x => x.id === id);
        if (!t) return;
        pendingDeleteId = id;
        $('#confirmText').textContent = `确定要删除「${t.name}」吗？此操作不可恢复。`;
        showModal('confirmModal');
    }

    async function doDelete() {
        if (!pendingDeleteId) return;
        try {
            await api(`/api/tools/${pendingDeleteId}`, { method: 'DELETE' });
            toast('删除成功', 'success');
            hideModal('confirmModal');
            loadManageTable();
        } catch (err) {
            if (err.status === 401) { handleUnauthorized(); return; }
            toast('删除失败：' + err.message, 'error');
        } finally {
            pendingDeleteId = null;
        }
    }

    function handleUnauthorized() {
        state.loggedIn = false;
        renderUserArea();
        toast('会话已过期，请重新登录', 'error');
        switchTab('manage');
    }

    // ---------- 拖动排序 ----------
    let dragSrc = null;
    function bindDragEvents() {
        const rows = $$('#manageTable tbody tr');
        rows.forEach(row => {
            row.addEventListener('dragstart', (e) => {
                dragSrc = row;
                row.classList.add('dragging');
                e.dataTransfer.effectAllowed = 'move';
                e.dataTransfer.setData('text/plain', row.dataset.id);
            });
            row.addEventListener('dragend', () => {
                row.classList.remove('dragging');
                $$('#manageTable tbody tr').forEach(r => r.classList.remove('drag-over-top', 'drag-over-bottom'));
            });
            row.addEventListener('dragover', (e) => {
                e.preventDefault();
                if (!dragSrc || dragSrc === row) return;
                const rect = row.getBoundingClientRect();
                const isTop = (e.clientY - rect.top) < rect.height / 2;
                row.classList.toggle('drag-over-top', isTop);
                row.classList.toggle('drag-over-bottom', !isTop);
            });
            row.addEventListener('dragleave', () => {
                row.classList.remove('drag-over-top', 'drag-over-bottom');
            });
            row.addEventListener('drop', async (e) => {
                e.preventDefault();
                if (!dragSrc || dragSrc === row) return;
                const rect = row.getBoundingClientRect();
                const isTop = (e.clientY - rect.top) < rect.height / 2;
                const tbody = row.parentNode;
                if (isTop) tbody.insertBefore(dragSrc, row);
                else tbody.insertBefore(dragSrc, row.nextSibling);
                row.classList.remove('drag-over-top', 'drag-over-bottom');
                await persistOrder();
            });
        });
    }

    async function persistOrder() {
        const ids = [...$$('#manageTable tbody tr')].map(r => Number(r.dataset.id));
        try {
            await api('/api/tools/sort', { method: 'PUT', body: { ids } });
            toast('排序已保存', 'success', 1500);
            // 更新本地 state 顺序
            const map = new Map(state.tools.map(t => [t.id, t]));
            state.tools = ids.map(id => map.get(id)).filter(Boolean);
        } catch (err) {
            if (err.status === 401) { handleUnauthorized(); return; }
            toast('排序保存失败：' + err.message, 'error');
            loadManageTable();
        }
    }

    // ---------- 弹窗辅助 ----------
    function showModal(id) { $('#' + id).style.display = 'flex'; }
    function hideModal(id) { $('#' + id).style.display = 'none'; }

    // ---------- 事件绑定 ----------
    function bindGlobalEvents() {
        $$('.tab-btn').forEach(b => b.addEventListener('click', () => switchTab(b.dataset.tab)));
        $('#loginForm').addEventListener('submit', doLogin);
        $('#pwdToggle').addEventListener('click', () => {
            const input = $('#loginPass');
            input.type = input.type === 'password' ? 'text' : 'password';
        });
        $('#btnAdd').addEventListener('click', openAddModal);
        $('#editForm').addEventListener('submit', submitEditForm);
        $('#confirmOk').addEventListener('click', doDelete);
        $$('[data-close]').forEach(el => el.addEventListener('click', () => hideModal(el.dataset.close)));
        $$('.modal-mask').forEach(m => m.addEventListener('click', () => {
            const modal = m.parentElement;
            if (modal && modal.id) hideModal(modal.id);
        }));
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                $$('.modal').forEach(m => { if (m.style.display !== 'none') hideModal(m.id); });
            }
        });
    }

    // ---------- 启动 ----------
    async function init() {
        bindGlobalEvents();
        await checkAuth();
        switchTab('list');
    }

    document.addEventListener('DOMContentLoaded', init);
})();
