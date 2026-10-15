/* Static GitHub Pages demo adapter. It replaces the Go API with deterministic,
   local-only data so every tab can be explored without Apple credentials. */
(function () {
  const originalFetch = window.fetch.bind(window);
  let checkJob = null;
  let downloadJob = null;
  const singleDownloads = new Map();
  const names = ['1Password', 'Infuse', 'Documents', 'Pixelmator Photo', 'GoodNotes', 'Procreate', 'Carrot Weather', 'Apollo', 'Overcast', 'Spark'];
  const icon = 'https://is1-ssl.mzstatic.com/image/thumb/Purple126/v4/00/00/00/00000000-0000-0000-0000-000000000000/AppIcon-0-0-1x_U007emarketing-0-7-0-0-85-220.png/512x512bb.jpg';
  const json = (data, status = 200) => Promise.resolve({ ok: status >= 200 && status < 300, status, json: () => Promise.resolve(data) });
  const body = req => { try { return JSON.parse(req.body || '{}'); } catch (_) { return {}; } };
  const app = (i, name = names[i % names.length], platforms = ['iphone', 'ipad']) => ({
    appId: 100000000 + i,
    trackId: 100000000 + i,
    name,
    trackName: name,
    bundleId: `com.demo.${name.toLowerCase().replace(/[^a-z]+/g, '')}`,
    artistName: 'Demo Developer',
    version: `3.${i}.1`,
    price: 0,
    artworkUrl512: icon,
    fileSizeBytes: 78000000 + i * 1200000,
    platforms
  });

  const osPrefixForPlatform = platform => {
    switch (String(platform || '').toLowerCase()) {
      case 'appletv': return 'tvOS';
      case 'ipad': return 'iPadOS';
      case 'macos': return 'macOS';
      case 'watchos': return 'watchOS';
      case 'visionos': return 'visionOS';
      default: return 'iOS';
    }
  };

  let purchasesFetchedAt = Math.floor(Date.now() / 1000) - 3600;

  const responseFor = (url, req) => {
    if (url === '/api/status') return json({
      authenticated: true,
      os: 'windows',
      version: '2.7.0d-webGUI',
      updateInfo: {
        currentVersion: '2.7.0d-webGUI',
        latestVersion: '2.7.0d-webGUI',
        updateAvailable: false,
        releaseUrl: 'https://github.com/pdx15/ipatool-webGUI/releases'
      },
      account: { email: 'demo@example.com', name: 'Demo Apple ID', directoryServicesId: 'demo' }
    });
    if (url === '/api/icloud/status') return json({ installed: true, variant: 'demo' });
    if (url === '/api/downloads/active') return json({ success: true, jobs: Array.from(singleDownloads.values()) });
    if (url.includes('/api/search/all')) return json({
      success: true,
      official: {
        results: [
          app(1, '1Password', ['iphone', 'ipad']),
          app(2, 'Infuse', ['iphone', 'ipad', 'appletv', 'macos']),
          app(3, 'Documents', ['iphone', 'ipad', 'macos'])
        ]
      },
      removed: {
        results: [
          { appId: 568903335, name: '1Password 7' },
          { appId: 448868000, name: 'Okko' }
        ]
      }
    });
    if (url.includes('/api/search?')) return json({
      success: true,
      results: [
        app(1, '1Password', ['iphone', 'ipad']),
        app(2, 'Infuse', ['iphone', 'ipad', 'appletv', 'macos']),
        app(3, 'Documents', ['iphone', 'ipad', 'macos'])
      ]
    });
    if (url.includes('/api/removed-apps')) return json({ success: true, results: [] });
    if (url === '/api/auth/revoke') return json({ success: true });
    if (url.includes('/api/purchases')) {
      const isRefresh = url.includes('refresh=1');
      if (isRefresh) {
        purchasesFetchedAt = Math.floor(Date.now() / 1000);
      }
      const items = [
        {
          appId: 1451612172,
          bundleId: 'ru.mts.tvh',
          name: 'KION – оригинальный кинотеатр',
          version: '5.20.3',
          tvosVersion: '3.12.1',
          purchaseDate: '2026-10-15 10:12:00',
          platforms: ['iphone', 'ipad', 'appletv'],
          hiddenFromStorefront: false
        },
        {
          appId: 448868000,
          bundleId: 'ru.more.play',
          name: 'Okko: Кино и Сериалы',
          version: '14.4.0',
          tvosVersion: '26.1.0',
          purchaseDate: '2026-09-18 19:40:12',
          platforms: ['iphone', 'ipad', 'appletv'],
          hiddenFromStorefront: true
        },
        {
          appId: 1136220934,
          bundleId: 'com.firecore.infuse',
          name: 'Infuse',
          version: '8.1.2',
          purchaseDate: '2026-09-07 09:54:20',
          platforms: ['iphone', 'ipad', 'appletv', 'macos'],
          hiddenFromStorefront: false
        },
        {
          appId: 568903335,
          bundleId: 'com.agilebits.onepassword-ios',
          name: '1Password 7',
          version: '7.9.11',
          purchaseDate: '2026-08-14 14:22:05',
          platforms: ['iphone', 'ipad'],
          hiddenFromStorefront: true
        },
        {
          appId: 1289583905,
          bundleId: 'com.pixelmatorteam.pixelmator.x',
          name: 'Pixelmator Pro',
          version: '3.6.14',
          purchaseDate: '2026-07-03 11:05:44',
          platforms: ['macos'],
          hiddenFromStorefront: false
        },
        {
          appId: 1207581673,
          bundleId: 'com.bigbluefrog.duffy',
          name: 'Duffy - Steps Complication',
          version: '2.4',
          purchaseDate: '2026-06-19 08:30:00',
          platforms: ['iphone', 'watchos'],
          hiddenFromStorefront: false
        }
      ];
      return json({
        success: true,
        count: items.length,
        items,
        cached: !isRefresh,
        fetchedAt: purchasesFetchedAt
      });
    }
    if (url === '/api/save-list' && req.method === 'POST') {
      const b = body(req);
      const filename = b.filename || 'purchases-demo@example.com.txt';
      return json({
        success: true,
        filename,
        path: `C:\\ipatool-webGUI\\${filename}`
      });
    }
    if (url === '/api/download' && req.method === 'POST') {
      const b = body(req);
      const jobId = `demo-dl-${Date.now()}`;
      const ext = String(b.platform || '').toLowerCase() === 'macos' ? 'pkg' : 'ipa';
      const appName = b.appName || b.bundleId || String(b.appId || 'DemoApp');
      singleDownloads.set(jobId, {
        id: jobId,
        appName,
        bundleId: b.bundleId || 'com.demo.app',
        appId: b.appId || 100000001,
        platform: b.platform || 'iphone',
        status: 'downloading',
        progress: 15,
        bytesRead: 12000000,
        totalBytes: 80000000,
        speed: '24.5 MB/s',
        startedAt: Date.now(),
        outputPath: `C:\\ipatool-webGUI\\apps\\${appName}_3.1.0.${ext}`
      });
      return json({ success: true, jobId });
    }
    if (url.includes('/api/download/status')) {
      const parsedUrl = new URL(url, window.location.origin);
      const id = parsedUrl.searchParams.get('id');
      const job = singleDownloads.get(id);
      if (!job) return json({ error: 'job not found' }, 404);
      const elapsed = Date.now() - job.startedAt;
      if (elapsed >= 1200) {
        job.status = 'completed';
        job.progress = 100;
        job.bytesRead = job.totalBytes;
      } else {
        job.progress = Math.min(95, Math.floor((elapsed / 1200) * 100));
        job.bytesRead = Math.floor((job.progress / 100) * job.totalBytes);
      }
      return json(job);
    }
    if (url.includes('/api/install/devices')) return json({ success: true, devices: [], toolsAvailable: false, tools: [], driver: { available: false } });
    if (url.includes('/api/open-folder')) return json({ success: true });
    if (url.includes('/api/purchase')) return json({ success: true, message: 'Лицензия получена (демо)' });
    if (url.includes('/api/versions')) {
      const parsedUrl = new URL(url, window.location.origin);
      const appId = parsedUrl.searchParams.get('appId');
      const bundleId = parsedUrl.searchParams.get('bundleId');
      const platform = parsedUrl.searchParams.get('platform') || 'iphone';
      if (appId === '448868000' || bundleId === 'ru.more.play') {
        return json({
          success: true,
          appID: 448868000,
          name: 'Okko: Кино и Сериалы',
          bundleID: 'ru.more.play',
          externalVersionIdentifiers: platform === 'appletv'
            ? ['865120101', '872340102', '883790103']
            : ['854001100', '861220150', '868324193']
        });
      }
      return json({
        success: true,
        appID: Number(appId) || 1451612172,
        name: bundleId === 'ru.mts.tvh' ? 'KION – оригинальный кинотеатр' : 'Demo App',
        bundleID: bundleId || 'com.demo.demoapp',
        externalVersionIdentifiers: ['2025070101', '2026081201', '2026090701']
      });
    }
    if (url.includes('/api/version-metadata/batch')) {
      const b = body(req);
      const versionIds = b.versionIds || [];
      const osPrefix = osPrefixForPlatform(b.platform);
      const minVer = osPrefix === 'tvOS' ? '15.0' : (osPrefix === 'macOS' ? '13.0' : '16.0');
      const results = Object.fromEntries(versionIds.map((id, index) => [id, {
        success: true,
        displayVersion: id === '883790103' ? '26.1.0' : `3.${versionIds.length - index}.0`,
        releaseDate: '2026-09-07',
        minimumOSVersion: `${osPrefix} ${minVer}`
      }]));
      return json({ success: true, results });
    }
    if (url.includes('/api/version-metadata')) {
      const parsedUrl = new URL(url, window.location.origin);
      const osPrefix = osPrefixForPlatform(parsedUrl.searchParams.get('platform'));
      return json({
        success: true,
        displayVersion: '3.1.0',
        releaseDate: '2026-09-07',
        minimumOSVersion: `${osPrefix} 16.0`
      });
    }
    if (url === '/api/batch/check' && req.method === 'POST') {
      const b = body(req); const items = b.items || [];
      checkJob = { id: 'demo-check', status: 'running', total: items.length, started: Date.now(), items: items.map((x, i) => ({ appId: x.appId, name: x.name || `Demo App ${i + 1}`, status: 'checking' })) };
      return json({ success: true, jobId: checkJob.id, total: items.length });
    }
    if (url.includes('/api/batch/check/status')) {
      if (!checkJob) return json({});
      const done = Math.min(checkJob.total, Math.floor((Date.now() - checkJob.started) / 180));
      checkJob.items.forEach((x, i) => { if (i < done) { x.status = 'available'; x.version = `2.${i}.0`; } });
      checkJob.done = done; checkJob.progress = checkJob.total ? done / checkJob.total * 100 : 100; checkJob.status = done >= checkJob.total ? 'completed' : 'running';
      return json(checkJob);
    }
    if (url === '/api/batch/download' && req.method === 'POST') {
      const b = body(req); const items = b.items || [];
      downloadJob = { id: 'demo-download', status: 'running', total: items.length, started: Date.now(), items: items.map((x, i) => ({ appId: x.appId, name: x.name || `Demo App ${i + 1}`, status: 'queued', progress: 0, totalBytes: 90000000 + i * 1000000, bytesRead: 0 })) };
      return json({ success: true, batchId: downloadJob.id, total: items.length });
    }
    if (url.includes('/api/batch/download/status')) {
      if (!downloadJob) return json({});
      const elapsed = Math.floor((Date.now() - downloadJob.started) / 250);
      downloadJob.items.forEach((x, i) => { if (i < elapsed) { x.status = 'completed'; x.progress = 100; x.outputPath = `demo/downloads/${x.name}.ipa`; } else if (i === elapsed && i < downloadJob.total) { x.status = 'downloading'; x.progress = Math.min(99, ((Date.now() - downloadJob.started) % 9000) / 90); } });
      const done = downloadJob.items.filter(x => x.status === 'completed').length;
      downloadJob.progress = downloadJob.items.reduce((n, x) => n + (x.status === 'completed' ? 100 : x.progress || 0), 0) / Math.max(1, downloadJob.total);
      downloadJob.completedCount = done; downloadJob.errors = 0;
      if (done === downloadJob.total) downloadJob.status = 'completed';
      return json(downloadJob);
    }
    return json({ success: true, devices: [], jobs: [], apps: [] });
  };
  window.fetch = function (input, init) { const url = typeof input === 'string' ? input : input.url; const req = { method: (init && init.method) || 'GET', body: init && init.body }; if (url.startsWith('/api/')) return responseFor(url, req); return originalFetch(input, init); };
})();
