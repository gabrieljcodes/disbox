// ═══════════════════════════════════════════════
// Disbox Browser Extension — Popup Logic
// ═══════════════════════════════════════════════

(function () {
    // ─── DOM Refs ───
    const viewSettings = document.getElementById('view-settings');
    const viewMain = document.getElementById('view-main');
    const inputServerUrl = document.getElementById('input-server-url');
    const inputApiToken = document.getElementById('input-api-token');
    const btnConnect = document.getElementById('btn-connect');
    const btnRefresh = document.getElementById('btn-refresh');
    const btnSettings = document.getElementById('btn-settings');
    const connectionDot = document.getElementById('connection-dot');
    const downloadsLoading = document.getElementById('downloads-loading');
    const downloadList = document.getElementById('download-list');
    const downloadsEmpty = document.getElementById('downloads-empty');
    const inputAddLink = document.getElementById('input-add-link');
    const linkTypeIndicator = document.getElementById('link-type-indicator');
    const btnAddDownload = document.getElementById('btn-add-download');

    let serverUrl = '';
    let apiToken = '';

    // ─── Helpers ───
    function formatBytes(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    function escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    function showToast(msg, type) {
        const toast = viewMain.style.display !== 'none'
            ? document.getElementById('main-toast')
            : document.getElementById('settings-toast');
        toast.textContent = msg;
        toast.className = 'toast ' + type + ' visible';
        setTimeout(() => { toast.className = 'toast'; }, 3000);
    }

    // ─── API Client ───
    async function apiFetch(endpoint, options = {}) {
        const url = serverUrl.replace(/\/+$/, '') + endpoint;
        const headers = {
            'Authorization': `Bearer ${apiToken}`,
            ...options.headers,
        };
        if (options.body && typeof options.body === 'string') {
            headers['Content-Type'] = 'application/json';
        }
        const resp = await fetch(url, { ...options, headers });
        return resp.json();
    }

    // ─── Init ───
    chrome.storage.sync.get(['serverUrl', 'apiToken'], (config) => {
        if (config.serverUrl && config.apiToken) {
            serverUrl = config.serverUrl;
            apiToken = config.apiToken;
            inputServerUrl.value = serverUrl;
            inputApiToken.value = apiToken;
            showMainView();
        } else {
            showSettingsView();
        }
    });

    function showSettingsView() {
        viewSettings.style.display = '';
        viewMain.style.display = 'none';
    }

    function showMainView() {
        viewSettings.style.display = 'none';
        viewMain.style.display = '';
        loadAll();
    }

    // ─── Connect Button ───
    btnConnect.addEventListener('click', async () => {
        const url = inputServerUrl.value.trim();
        const token = inputApiToken.value.trim();
        if (!url || !token) {
            showToast('Please fill in both fields.', 'error');
            return;
        }

        btnConnect.disabled = true;
        btnConnect.textContent = 'Connecting...';

        try {
            const resp = await fetch(url.replace(/\/+$/, '') + '/v1/me', {
                headers: { 'Authorization': `Bearer ${token}` },
            });
            const data = await resp.json();

            if (data.success || data.id || data.username) {
                serverUrl = url;
                apiToken = token;
                chrome.storage.sync.set({ serverUrl: url, apiToken: token });
                showMainView();
            } else {
                showToast(data.error || 'Authentication failed.', 'error');
            }
        } catch (err) {
            showToast('Could not reach server.', 'error');
        }

        btnConnect.disabled = false;
        btnConnect.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"/><polyline points="10 17 15 12 10 7"/><line x1="15" y1="12" x2="3" y2="12"/></svg> Connect`;
    });

    // ─── Settings & Refresh Buttons ───
    btnSettings.addEventListener('click', () => showSettingsView());
    btnRefresh.addEventListener('click', () => loadAll());

    // ─── Tabs ───
    document.querySelectorAll('.tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
            tab.classList.add('active');
            document.getElementById('panel-' + tab.dataset.tab).classList.add('active');

            if (tab.dataset.tab === 'profile') loadProfile();
        });
    });

    // ─── Load All ───
    async function loadAll() {
        loadStats();
        loadDownloads();
    }

    // ─── Load Stats ───
    async function loadStats() {
        try {
            const data = await apiFetch('/v1/queue-status');
            if (data.success) {
                const d = data.data;
                document.getElementById('stat-active').textContent = d.active_jobs;
                document.getElementById('stat-total').textContent = d.total_capacity;
                document.getElementById('stat-queued').textContent = d.queued_jobs;
                const tb = 1024 * 1024 * 1024 * 1024;
                document.getElementById('stat-bw-used').textContent = d.global_bandwidth_used ? (d.global_bandwidth_used / tb).toFixed(2) : '0';
                document.getElementById('stat-bw-limit').textContent = d.global_bandwidth_limit ? (d.global_bandwidth_limit / tb).toFixed(0) : '0';
            }
            connectionDot.classList.remove('error');
        } catch {
            connectionDot.classList.add('error');
        }
    }

    // ─── Load Downloads ───
    async function loadDownloads() {
        downloadsLoading.style.display = '';
        downloadList.style.display = 'none';
        downloadsEmpty.style.display = 'none';

        try {
            const data = await apiFetch('/v1/history');
            downloadsLoading.style.display = 'none';

            let items = [];
            if (data.success && Array.isArray(data.data)) {
                items = data.data;
            } else if (Array.isArray(data)) {
                items = data;
            }

            if (items.length === 0) {
                downloadsEmpty.style.display = '';
                return;
            }

            downloadList.style.display = '';
            downloadList.innerHTML = '';

            items.forEach((item, i) => {
                const el = document.createElement('div');
                el.className = 'dl-item';
                el.style.animationDelay = (i * 0.04) + 's';

                const icon = item.type === 'torrent' ? '🌊' : '🌐';
                const typeClass = item.type === 'torrent' ? '' : 'webdl';
                const typeLabel = item.type === 'torrent' ? 'Torrent' : 'Web DL';
                const date = new Date(item.created_at).toLocaleDateString('en-US', {
                    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
                });

                const dlUrl = item.download_url || (serverUrl.replace(/\/+$/, '') + '/dl/' + item.token);
                const browseUrl = item.browse_url || (serverUrl.replace(/\/+$/, '') + '/browse/' + item.token);

                el.innerHTML = `
                    <div class="dl-item-header">
                        <span class="dl-icon">${icon}</span>
                        <div class="dl-info">
                            <div class="dl-name" title="${escapeHtml(item.name)}">${escapeHtml(item.name)}</div>
                            <div class="dl-meta">
                                <span class="dl-type ${typeClass}">${typeLabel}</span>
                                <span class="dl-date">${date}</span>
                            </div>
                        </div>
                    </div>
                    <div class="dl-actions">
                        <a href="${browseUrl}" target="_blank" class="dl-btn">
                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
                            Browse
                        </a>
                        <a href="${dlUrl}" target="_blank" class="dl-btn">
                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
                            Download
                        </a>
                        <button class="dl-btn btn-ftp" data-token="${item.token}" data-action="ftp">
                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
                            FTP
                        </button>
                        <button class="dl-btn" data-token="${item.token}" data-url="${dlUrl}" data-action="copy">
                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                            Copy
                        </button>
                        <button class="dl-btn btn-remove" data-token="${item.token}" data-action="remove">
                            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                            Remove
                        </button>
                    </div>
                `;

                downloadList.appendChild(el);
            });

            // Attach action listeners
            downloadList.querySelectorAll('[data-action]').forEach(btn => {
                btn.addEventListener('click', (e) => {
                    e.preventDefault();
                    const action = btn.dataset.action;
                    const token = btn.dataset.token;

                    if (action === 'copy') {
                        navigator.clipboard.writeText(btn.dataset.url).then(() => {
                            showToast('Link copied!', 'success');
                        });
                    } else if (action === 'ftp') {
                        sendToFtp(token, btn);
                    } else if (action === 'remove') {
                        removeDownload(token, btn);
                    }
                });
            });

        } catch (err) {
            downloadsLoading.style.display = 'none';
            downloadsEmpty.style.display = '';
            connectionDot.classList.add('error');
        }
    }

    // ─── Send to FTP ───
    async function sendToFtp(token, btn) {
        btn.disabled = true;
        btn.style.opacity = '0.5';
        try {
            const data = await apiFetch('/v1/ftp/send', {
                method: 'POST',
                body: JSON.stringify({ token }),
            });
            if (data.success) {
                showToast(data.data?.message || 'FTP upload started', 'success');
            } else {
                showToast(data.error || 'FTP upload failed', 'error');
            }
        } catch {
            showToast('Connection error', 'error');
        }
        btn.disabled = false;
        btn.style.opacity = '1';
    }

    // ─── Remove Download ───
    async function removeDownload(token, btn) {
        btn.disabled = true;
        btn.style.opacity = '0.5';
        try {
            const data = await apiFetch('/v1/remove-download', {
                method: 'POST',
                body: JSON.stringify({ token }),
            });
            if (data.success) {
                showToast('Download removed', 'success');
                loadDownloads();
            } else {
                showToast(data.error || 'Failed to remove', 'error');
            }
        } catch {
            showToast('Connection error', 'error');
        }
        btn.disabled = false;
        btn.style.opacity = '1';
    }

    // ─── Add Download ───
    inputAddLink.addEventListener('input', () => {
        const val = inputAddLink.value.trim();
        if (!val) {
            linkTypeIndicator.className = 'link-type-indicator';
            linkTypeIndicator.innerHTML = '<span>Paste a link to detect type</span>';
            btnAddDownload.disabled = true;
        } else if (val.startsWith('magnet:')) {
            linkTypeIndicator.className = 'link-type-indicator magnet';
            linkTypeIndicator.innerHTML = '<span>🌊 Magnet Link — will be added as a Torrent</span>';
            btnAddDownload.disabled = false;
        } else if (val.startsWith('http://') || val.startsWith('https://')) {
            linkTypeIndicator.className = 'link-type-indicator webdl';
            linkTypeIndicator.innerHTML = '<span>🌐 Direct URL — will be added as a Web Download</span>';
            btnAddDownload.disabled = false;
        } else {
            linkTypeIndicator.className = 'link-type-indicator';
            linkTypeIndicator.innerHTML = '<span>⚠️ Unrecognized link format</span>';
            btnAddDownload.disabled = true;
        }
    });

    btnAddDownload.addEventListener('click', async () => {
        const link = inputAddLink.value.trim();
        if (!link) return;

        btnAddDownload.disabled = true;
        btnAddDownload.textContent = 'Adding...';

        const isMagnet = link.startsWith('magnet:');
        const endpoint = isMagnet ? '/v1/add-torrent' : '/v1/add-webdl';

        try {
            const data = await apiFetch(endpoint, {
                method: 'POST',
                body: JSON.stringify({ link }),
            });

            if (data.success) {
                const name = data.data?.name || 'Download';
                showToast(`Added: ${name}`, 'success');
                inputAddLink.value = '';
                inputAddLink.dispatchEvent(new Event('input'));
                // Switch to downloads tab
                document.querySelector('[data-tab="downloads"]').click();
                loadDownloads();
                loadStats();
            } else {
                showToast(data.error || 'Failed to add download', 'error');
            }
        } catch {
            showToast('Connection error', 'error');
        }

        btnAddDownload.disabled = false;
        btnAddDownload.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><polyline points="19 12 12 19 5 12"/></svg> Add Download`;
    });

    // ─── Profile ───
    async function loadProfile() {
        try {
            const data = await apiFetch('/v1/user/profile');
            if (data.success) {
                const d = data.data;
                document.getElementById('profile-total').textContent = formatBytes(d.total_downloaded);
                document.getElementById('profile-monthly').textContent = formatBytes(d.monthly_downloaded);
                document.getElementById('input-ftp-host').value = d.ftp_host || '';
                document.getElementById('input-ftp-user').value = d.ftp_username || '';
                if (d.has_ftp_password) {
                    document.getElementById('input-ftp-pass').placeholder = '•••••••• (Saved)';
                } else {
                    document.getElementById('input-ftp-pass').placeholder = 'FTP Password';
                }
            }
        } catch {
            // Silently fail
        }
    }

    document.getElementById('btn-save-ftp').addEventListener('click', async () => {
        const host = document.getElementById('input-ftp-host').value.trim();
        const user = document.getElementById('input-ftp-user').value.trim();
        const pass = document.getElementById('input-ftp-pass').value;

        try {
            const data = await apiFetch('/v1/user/ftp', {
                method: 'POST',
                body: JSON.stringify({ host, username: user, password: pass }),
            });

            if (data.success) {
                showToast('FTP settings saved', 'success');
                document.getElementById('input-ftp-pass').value = '';
                loadProfile();
            } else {
                showToast(data.error || 'Failed to save', 'error');
            }
        } catch {
            showToast('Connection error', 'error');
        }
    });

})();
