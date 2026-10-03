// Inscrição de Web Push — chamado em toda abertura do app (se a permissão
// já estiver concedida, CONTRATO §5/§6) e pelo botão "Ativar notificações"
// de Ajustes.
import { api } from './api'

function urlBase64ToUint8Array(base64: string): Uint8Array {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4)
  const base64Safe = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(base64Safe)
  const bytes = new Uint8Array(raw.length)
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i)
  return bytes
}

export function suportado(): boolean {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window
}

export function permissao(): NotificationPermission | 'sem-suporte' {
  if (!('Notification' in window)) return 'sem-suporte'
  return Notification.permission
}

// garantirInscricao assume que a permissão já foi concedida — inscreve (ou
// reusa a inscrição existente) e manda para o backend. Cobre troca de
// endpoint e a migração do spike (§5).
export async function garantirInscricao(): Promise<boolean> {
  if (!suportado()) return false
  try {
    const reg = await navigator.serviceWorker.ready
    let sub = await reg.pushManager.getSubscription()
    if (!sub) {
      const { chave_publica } = await api.vapid()
      sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(chave_publica) as BufferSource,
      })
    }
    await api.pushSubscribe(sub.toJSON() as PushSubscriptionJSON)
    return true
  } catch (e) {
    console.error('falha ao inscrever para push', e)
    return false
  }
}

// pedirPermissaoEInscrever é o fluxo do botão "Ativar notificações".
export async function pedirPermissaoEInscrever(): Promise<boolean> {
  if (!suportado()) return false
  const resultado = await Notification.requestPermission()
  if (resultado !== 'granted') return false
  return garantirInscricao()
}

// inscricaoAtiva diz se já existe uma PushSubscription neste navegador
// (para a tela de Ajustes mostrar "inscrito: sim/não").
export async function inscricaoAtiva(): Promise<boolean> {
  if (!suportado()) return false
  try {
    const reg = await navigator.serviceWorker.getRegistration()
    if (!reg) return false
    const sub = await reg.pushManager.getSubscription()
    return !!sub
  } catch {
    return false
  }
}
