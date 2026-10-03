// Estado de conexão com o redmilab (aceite V10 da fase 1.5). navigator.onLine
// não serve: com o Tailscale ligado, o Android segue "online" em modo avião
// (a interface da VPN continua de pé) e os fetch ficam pendurados. Aqui a
// verdade vem das próprias requisições: falha de rede/prazo estourado marca
// offline; qualquer resposta do servidor marca online. Enquanto offline, uma
// sondagem leve em /health a cada 10 s descobre a volta sozinha.
import { useSyncExternalStore } from 'react'

const SONDAGEM_MS = 10_000
const PRAZO_SONDAGEM_MS = 4_000

let online = navigator.onLine
let sondagem: number | null = null
const ouvintes = new Set<() => void>()

function notificar() {
  ouvintes.forEach((f) => f())
}

async function sondar() {
  const ctrl = new AbortController()
  const t = window.setTimeout(() => ctrl.abort(), PRAZO_SONDAGEM_MS)
  try {
    await fetch('/health', { cache: 'no-store', signal: ctrl.signal })
    marcarOnline()
  } catch {
    // segue offline; a próxima sondagem tenta de novo
  } finally {
    window.clearTimeout(t)
  }
}

export function marcarOffline() {
  if (!online) return
  online = false
  if (sondagem === null) sondagem = window.setInterval(sondar, SONDAGEM_MS)
  notificar()
}

export function marcarOnline() {
  if (sondagem !== null) {
    window.clearInterval(sondagem)
    sondagem = null
  }
  if (online) return
  online = true
  notificar()
}

export function estaOnline(): boolean {
  return online
}

// abriu já sem interface de rede: começa sondando
if (!online) sondagem = window.setInterval(sondar, SONDAGEM_MS)

// Os eventos do navegador ainda valem como dica: "offline" é confiável
// (sem interface nenhuma); "online" só dispara uma sondagem.
window.addEventListener('offline', marcarOffline)
window.addEventListener('online', () => {
  sondar()
})

function inscrever(f: () => void) {
  ouvintes.add(f)
  return () => {
    ouvintes.delete(f)
  }
}

export function useOnline(): boolean {
  return useSyncExternalStore(inscrever, estaOnline)
}
