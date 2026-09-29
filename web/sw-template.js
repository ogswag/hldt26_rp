// The app without network. One deployment's files live in one cache named after that build, so a new
// deployment never mixes with an old one. The cache of the build before is kept too: a tab still on that build
// opens its lazy pages from it until it reloads itself. API answers are not cached here: the project store and
// the offline reference data keep what they need in IndexedDB themselves.
//
// This file is a template. The build fills VERSION and PRECACHE from the bundle and writes it out as sw.js.

const VERSION = '__VERSION__'
const PRECACHE = __PRECACHE__
const PREFETCH = __PREFETCH__
const CACHE = `robots-${VERSION}`
// BUILDS lists the caches of the builds kept, newest first, in a cache of its own.
const META = 'robots-meta'
const KEPT_BUILDS = 2

// UNHASHED are the files whose names stay the same while their contents change with a deployment: the
// calculation engine the API serves, and, in a demo build, the engine and the reference data it carries. They
// are answered from the cache and replaced behind the answer, so a copy is never stale for more than a visit.
const UNHASHED = ['/api/engine/', '/engine/', '/demo/']

self.addEventListener('install', (e) => {
  e.waitUntil(
    (async () => {
      const c = await caches.open(CACHE)
      await c.addAll(PRECACHE)
      // The pages behind the first screen: worth having, not worth waiting for, and a miss is not a failure.
      void Promise.allSettled(PREFETCH.map((url) => c.add(url)))
      // A new build takes over at once. The page reloads itself when it is safe (see offline/serviceWorker.ts).
      await self.skipWaiting()
    })(),
  )
})

self.addEventListener('activate', (e) => {
  e.waitUntil(
    (async () => {
      const meta = await caches.open(META)
      const known = await meta
        .match('/builds')
        .then((r) => (r ? r.json() : []))
        .catch(() => [])
      const builds = [CACHE, ...known.filter((name) => name !== CACHE)].slice(0, KEPT_BUILDS)
      await meta.put('/builds', new Response(JSON.stringify(builds)))
      for (const name of await caches.keys()) {
        if (name.startsWith('robots-') && name !== META && !builds.includes(name)) {
          await caches.delete(name)
        }
      }
      await self.clients.claim()
    })(),
  )
})

// The page asks which files this build needs, to learn whether it is itself on an older build.
self.addEventListener('message', (e) => {
  if (e.data === 'version') {
    e.source?.postMessage({ version: VERSION, shell: PRECACHE })
  }
})

self.addEventListener('fetch', (e) => {
  const req = e.request
  if (req.method !== 'GET') {
    return
  }
  const url = new URL(req.url)
  if (url.origin !== self.location.origin) {
    return
  }
  if (UNHASHED.some((prefix) => url.pathname.startsWith(prefix))) {
    e.respondWith(staleWhileRevalidate(req))
    return
  }
  // Everything the server answers for the moment: sessions, projects, the catalog, health.
  if (url.pathname.startsWith('/api/') || url.pathname === '/health' || url.pathname === '/version') {
    return
  }
  if (req.mode === 'navigate') {
    e.respondWith(navigation(req))
    return
  }
  e.respondWith(cacheFirst(req))
})

// navigation answers a page load. Every route of the app is index.html, so an address typed without network
// still opens the app.
async function navigation(req) {
  try {
    return await fetch(req)
  } catch (err) {
    const hit = await caches.match('/index.html', { cacheName: CACHE })
    if (hit) {
      return hit
    }
    throw err
  }
}

// cacheFirst answers a built file. Their names carry a content hash, so a hit is never stale; the kept caches
// of earlier builds answer for a page that has not reloaded yet.
async function cacheFirst(req) {
  const hit = (await caches.match(req, { cacheName: CACHE })) ?? (await caches.match(req))
  if (hit) {
    return hit
  }
  const res = await fetch(req)
  if (res.ok && res.type === 'basic') {
    const c = await caches.open(CACHE)
    await c.put(req, res.clone())
  }
  return res
}

// staleWhileRevalidate answers from the cache at once and replaces the copy for the next time.
async function staleWhileRevalidate(req) {
  const c = await caches.open(CACHE)
  const hit = await c.match(req)
  const fresh = fetch(req)
    .then(async (res) => {
      if (res.ok && res.type === 'basic') {
        await c.put(req, res.clone())
      }
      return res
    })
    .catch((err) => {
      if (hit) {
        return hit
      }
      throw err
    })
  return hit ?? fresh
}
