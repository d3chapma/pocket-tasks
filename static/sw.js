const CACHE = 'flowment-v1';

const PRECACHE = [
  '/',
  '/static/styles.css',
  '/static/keyhandler.js',
  '/static/datastar.js',
  '/static/icon.svg',
  '/manifest.json',
];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE)
      .then(cache => cache.addAll(PRECACHE))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(keys => Promise.all(
        keys.filter(k => k !== CACHE).map(k => caches.delete(k))
      ))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', event => {
  const url = new URL(event.request.url);

  // Only handle same-origin requests
  if (url.origin !== self.location.origin) return;

  // Static assets: cache-first, refresh in background
  if (url.pathname.startsWith('/static/') || url.pathname === '/manifest.json') {
    event.respondWith(
      caches.match(event.request).then(cached => {
        const network = fetch(event.request).then(response => {
          caches.open(CACHE).then(cache => cache.put(event.request, response.clone()));
          return response;
        });
        return cached || network;
      })
    );
    return;
  }

  // GET requests (page, history fragments): network-first, fall back to cache
  if (event.request.method === 'GET') {
    event.respondWith(
      fetch(event.request)
        .then(response => {
          caches.open(CACHE).then(cache => cache.put(event.request, response.clone()));
          return response;
        })
        .catch(() => caches.match(event.request))
    );
    return;
  }

  // Mutations (POST, DELETE): network-only
  // These will fail naturally when offline; the offline banner warns the user.
});
