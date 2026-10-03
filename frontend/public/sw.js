// Service worker do Pocket — escrito à mão (CONTRATO §6), sem Workbox.
// Responsabilidades: cache do shell para abrir instantâneo, mostrar push,
// reportar recebimento e tratar as ações da notificação (Feito/Adiar).
//
// Estratégia de cache (CONTRATO §6): navegação (documento) é network-first
// com fallback pro index.html cacheado (offline); /assets/* (JS/CSS com
// hash do build) é cache-first, já que o nome muda a cada build — uma vez
// buscado, nunca mais precisa de rede.
const CACHE = 'pocket-shell-v1'
const PRAZO_NAVEGACAO_MS = 4000
const SHELL_URLS = ['/', '/index.html', '/manifest.webmanifest', '/icon-192.png', '/icon-512.png']

self.addEventListener('install', (e) => {
  self.skipWaiting()
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL_URLS)).catch(() => {}))
})

self.addEventListener('activate', (e) => {
  e.waitUntil(
    Promise.all([
      self.clients.claim(),
      caches.keys().then((chaves) => Promise.all(chaves.filter((k) => k !== CACHE).map((k) => caches.delete(k)))),
    ]),
  )
})

self.addEventListener('fetch', (e) => {
  const req = e.request
  const url = new URL(req.url)

  // Dados (API/health) sempre da rede — nunca cacheados (§6: "dados sempre
  // da rede").
  if (url.pathname.startsWith('/api/') || url.pathname === '/health') return

  if (req.mode === 'navigate') {
    // Network-first com prazo (aceite V10 da fase 1.5): com o Tailscale
    // ligado em modo avião o fetch não falha, fica pendurado — sem prazo o
    // app "não abre". Passados PRAZO_NAVEGACAO_MS, serve o shell cacheado; a
    // busca segue em segundo plano e atualiza o cache se completar.
    const daRede = fetch(req).then((resp) => {
      const copia = resp.clone()
      caches.open(CACHE).then((c) => c.put('/index.html', copia)).catch(() => {})
      return resp
    })
    const doCache = () => caches.match('/index.html').then((r) => r || caches.match('/'))
    const prazo = new Promise((resolve) => setTimeout(resolve, PRAZO_NAVEGACAO_MS)).then(doCache)
    e.respondWith(
      Promise.race([daRede.catch(doCache), prazo]).then((r) => r || daRede),
    )
    return
  }

  if (url.pathname.startsWith('/assets/')) {
    e.respondWith(
      caches.match(req).then(
        (cached) =>
          cached ||
          fetch(req).then((resp) => {
            const copia = resp.clone()
            caches.open(CACHE).then((c) => c.put(req, copia)).catch(() => {})
            return resp
          }),
      ),
    )
  }
})

function chamarApi(caminho) {
  return fetch(caminho, { method: 'POST' }).catch(() => {})
}

self.addEventListener('push', (e) => {
  let dado = { notificacao_id: 0, item_id: 0, title: 'Pocket', body: '' }
  try {
    dado = e.data.json()
  } catch (_) {
    // push sem corpo JSON — usa o padrão acima
  }

  // notificacao_id = 0: alerta de certificado (CONTRATO §5/ops), não tem
  // linha em `notificacao` — sem ações e sem chamar a API (README, etapa 5).
  const ehAlertaCert = !dado.notificacao_id || dado.notificacao_id === 0
  // tipo="lista": lembrete de lista de compras (CONTRATO-FASE3 §2.6/§5) —
  // sem ações de Feito/Adiar (não fazem sentido para lista).
  const ehLista = dado.tipo === 'lista'

  const opcoes = {
    body: dado.body,
    tag: ehAlertaCert ? 'pocket-alerta-cert' : `pocket-notificacao-${dado.notificacao_id}`,
    icon: '/icon-192.png',
    badge: '/icon-192.png',
    data: dado,
  }
  if (!ehAlertaCert && !ehLista) {
    opcoes.actions = [
      { action: 'feito', title: 'Feito' },
      { action: 'adiar', title: 'Adiar 30 min' },
    ]
  }

  const tarefas = [self.registration.showNotification(dado.title, opcoes)]
  if (!ehAlertaCert) tarefas.push(chamarApi(`/api/notificacoes/${dado.notificacao_id}/recebida`))
  e.waitUntil(Promise.all(tarefas))
})

// abrirOuFocar abre '/' + caminho numa aba nova, ou foca/navega uma aba já
// aberta do app — evita empilhar abas quando o Pocket já está aberto.
async function abrirOuFocar(caminho) {
  const clientes = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
  for (const c of clientes) {
    if ('focus' in c) {
      await c.focus()
      if ('navigate' in c) {
        try {
          await c.navigate(caminho)
        } catch (_) {
          // navegação recusada (aba de outra origem etc.) — já focou, ok
        }
      }
      return
    }
  }
  return self.clients.openWindow(caminho)
}

self.addEventListener('notificationclick', (e) => {
  e.notification.close()
  const dado = e.notification.data || {}
  const ehAlertaCert = !dado.notificacao_id || dado.notificacao_id === 0
  const ehLista = dado.tipo === 'lista'

  if (ehAlertaCert) {
    e.waitUntil(self.clients.openWindow('/'))
    return
  }

  if (ehLista) {
    // sem ações — toque no corpo abre o app já na lista (item 7 do pedido;
    // App.tsx lê ?lista= na abertura e limpa o parâmetro).
    e.waitUntil(
      Promise.all([
        chamarApi(`/api/notificacoes/${dado.notificacao_id}/abrir`),
        abrirOuFocar(`/?lista=${dado.lista_id}`),
      ]),
    )
    return
  }

  if (e.action === 'feito') {
    e.waitUntil(chamarApi(`/api/notificacoes/${dado.notificacao_id}/feito`))
    return
  }
  if (e.action === 'adiar') {
    e.waitUntil(chamarApi(`/api/notificacoes/${dado.notificacao_id}/adiar`))
    return
  }
  // toque no corpo: abre pelo item (decisão A5) e registra a ação "abrir".
  e.waitUntil(
    Promise.all([chamarApi(`/api/notificacoes/${dado.notificacao_id}/abrir`), self.clients.openWindow('/')]),
  )
})
