// Service worker : rend l'application utilisable sans réseau.
//
// Stratégie « réseau d'abord » pour tout. En ligne, l'agent voit toujours la
// version à jour ; hors ligne, il retombe sur la dernière copie connue. C'est
// l'inverse d'un cache classique, mais c'est le bon choix ici : une interface
// périmée qui affiche un véhicule comme disponible alors qu'il est sorti pose
// plus de problèmes qu'un chargement un peu plus lent.

const CACHE = 'vlpm-v1';

// Ressources nécessaires au démarrage à froid, sans réseau.
const COQUILLE = ['/', '/index.html', '/app.css', '/app.js', '/api.js', '/ui.js'];

self.addEventListener('install', (e) => {
  e.waitUntil(
    caches.open(CACHE)
      .then((c) => c.addAll(COQUILLE))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys()
      .then((cles) => Promise.all(
        cles.filter((c) => c !== CACHE).map((c) => caches.delete(c)),
      ))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);

  // Seules les requêtes de cette instance sont concernées.
  if (url.origin !== self.location.origin) return;

  // L'API n'est jamais servie depuis le cache : une réponse périmée sur l'état
  // du parc induirait l'agent en erreur. C'est l'application qui décide quoi
  // faire d'un échec réseau, à partir de son propre instantané.
  if (url.pathname.startsWith('/api/')) return;

  // Les requêtes autres que GET ne se mettent pas en cache.
  if (e.request.method !== 'GET') return;

  e.respondWith(
    fetch(e.request)
      .then((reponse) => {
        if (reponse.ok) {
          const copie = reponse.clone();
          caches.open(CACHE).then((c) => c.put(e.request, copie));
        }
        return reponse;
      })
      .catch(async () => {
        const enCache = await caches.match(e.request);
        if (enCache) return enCache;
        // Route applicative demandée hors ligne : on sert la coquille, qui
        // saura afficher le bon écran.
        if (e.request.mode === 'navigate') {
          const index = await caches.match('/index.html');
          if (index) return index;
        }
        return new Response('Hors ligne', {
          status: 503,
          headers: { 'Content-Type': 'text/plain; charset=utf-8' },
        });
      }),
  );
});
