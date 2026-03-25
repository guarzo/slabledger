/**
 * Service Worker for SlabLedger
 *
 * Provides offline support and PWA capabilities.
 *
 * Features:
 * - Cache-first strategy for static assets
 * - Network-first strategy for API calls
 * - Offline fallback page
 * - Automatic cache cleanup
 */

const CACHE_NAME = 'slabledger-v1';
const RUNTIME_CACHE = 'slabledger-runtime';

// Assets to cache on install
const PRECACHE_ASSETS = [
  '/',
  '/index.html',
  '/manifest.json',
  // Add other critical assets here
];

// Cache size limits
const MAX_CACHE_SIZE = 50; // Maximum number of items in runtime cache
const MAX_CACHE_AGE = 7 * 24 * 60 * 60 * 1000; // 7 days in milliseconds

/**
 * Install event - precache critical assets
 */
self.addEventListener('install', (event) => {
  console.warn('[SW] Installing service worker');

  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      console.warn('[SW] Precaching static assets');
      return cache.addAll(PRECACHE_ASSETS);
    })
  );

  // Activate immediately
  self.skipWaiting();
});

/**
 * Activate event - cleanup old caches
 */
self.addEventListener('activate', (event) => {
  console.warn('[SW] Activating service worker');

  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.map((cacheName) => {
          if (cacheName !== CACHE_NAME && cacheName !== RUNTIME_CACHE) {
            console.warn('[SW] Deleting old cache:', cacheName);
            return caches.delete(cacheName);
          }
        })
      );
    })
  );

  // Take control immediately
  return self.clients.claim();
});

/**
 * Fetch event - serve from cache or network
 */
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Skip non-GET requests
  if (request.method !== 'GET') {
    return;
  }

  // Skip chrome-extension and other schemes
  if (!url.protocol.startsWith('http')) {
    return;
  }

  // API requests - Network first, fallback to cache
  if (url.pathname.startsWith('/api/')) {
    event.respondWith(networkFirst(request));
    return;
  }

  // Static assets - Cache first, fallback to network
  event.respondWith(cacheFirst(request));
});

/**
 * Cache-first strategy
 * Best for static assets that rarely change
 */
async function cacheFirst(request) {
  try {
    const cachedResponse = await caches.match(request);
    if (cachedResponse) {
      return cachedResponse;
    }

    const networkResponse = await fetch(request);

    // Cache successful responses
    if (networkResponse.ok) {
      const cache = await caches.open(RUNTIME_CACHE);
      cache.put(request, networkResponse.clone());

      // Cleanup old cache entries
      trimCache(RUNTIME_CACHE, MAX_CACHE_SIZE);
    }

    return networkResponse;
  } catch (error) {
    console.error('[SW] Fetch failed:', error);

    // Return offline fallback
    const cachedResponse = await caches.match('/offline.html');
    if (cachedResponse) {
      return cachedResponse;
    }

    return new Response('Offline', { status: 503 });
  }
}

/**
 * Network-first strategy
 * Best for API calls that need fresh data
 */
async function networkFirst(request) {
  try {
    const networkResponse = await fetch(request);

    // Cache successful responses
    if (networkResponse.ok) {
      const cache = await caches.open(RUNTIME_CACHE);
      cache.put(request, networkResponse.clone());
    }

    return networkResponse;
  } catch (error) {
    console.error('[SW] Network request failed, trying cache:', error);

    const cachedResponse = await caches.match(request);
    if (cachedResponse) {
      return cachedResponse;
    }

    return new Response(JSON.stringify({ error: 'Offline' }), {
      status: 503,
      headers: { 'Content-Type': 'application/json' },
    });
  }
}

/**
 * Trim cache to maximum size
 */
async function trimCache(cacheName, maxItems) {
  const cache = await caches.open(cacheName);
  const keys = await cache.keys();

  if (keys.length > maxItems) {
    // Remove oldest items
    const keysToDelete = keys.slice(0, keys.length - maxItems);
    await Promise.all(keysToDelete.map((key) => cache.delete(key)));
  }
}

/**
 * Clean up expired cache entries
 */
async function cleanExpiredCache() {
  const cache = await caches.open(RUNTIME_CACHE);
  const keys = await cache.keys();
  const now = Date.now();

  for (const request of keys) {
    const response = await cache.match(request);
    if (!response) continue;

    const cachedDate = response.headers.get('date');
    if (!cachedDate) continue;

    const cacheAge = now - new Date(cachedDate).getTime();
    if (cacheAge > MAX_CACHE_AGE) {
      await cache.delete(request);
    }
  }
}

// Run cleanup periodically
setInterval(() => {
  cleanExpiredCache();
}, 60 * 60 * 1000); // Every hour
