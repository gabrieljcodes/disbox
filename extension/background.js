// Disbox Extension — Background Service Worker
// Handles context menu registration and right-click "Send to Disbox" actions.

chrome.runtime.onInstalled.addListener(() => {
    chrome.contextMenus.create({
        id: 'disbox-send-link',
        title: 'Send to Disbox',
        contexts: ['link'],
    });
});

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
    if (info.menuItemId !== 'disbox-send-link') return;

    const url = info.linkUrl;
    if (!url) return;

    const config = await chrome.storage.sync.get(['serverUrl', 'apiToken']);
    if (!config.serverUrl || !config.apiToken) {
        chrome.notifications.create({
            type: 'basic',
            iconUrl: 'icons/icon128.png',
            title: 'Disbox — Not Configured',
            message: 'Please open the extension popup and set your server URL and API token.',
        });
        return;
    }

    const serverUrl = config.serverUrl.replace(/\/+$/, '');
    const isMagnet = url.startsWith('magnet:');
    const endpoint = isMagnet ? '/v1/add-torrent' : '/v1/add-webdl';

    try {
        const resp = await fetch(`${serverUrl}${endpoint}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${config.apiToken}`,
            },
            body: JSON.stringify({ link: url }),
        });

        const data = await resp.json();

        if (data.success) {
            const name = data.data?.name || 'Download';
            const status = data.data?.status || 'added';
            chrome.notifications.create({
                type: 'basic',
                iconUrl: 'icons/icon128.png',
                title: 'Disbox — Download Added',
                message: `${name}\nStatus: ${status}`,
            });
        } else {
            chrome.notifications.create({
                type: 'basic',
                iconUrl: 'icons/icon128.png',
                title: 'Disbox — Error',
                message: data.error || 'Failed to add download.',
            });
        }
    } catch (err) {
        chrome.notifications.create({
            type: 'basic',
            iconUrl: 'icons/icon128.png',
            title: 'Disbox — Connection Error',
            message: `Could not reach server: ${err.message}`,
        });
    }
});
