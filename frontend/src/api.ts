// Cliente tipado da API do Pocket — formatos exatos de API.md. Nenhum outro
// arquivo deve montar URL ou fazer fetch direto na API.
import { marcarOffline, marcarOnline } from './conexao'

export type TipoItem = 'habito' | 'compromisso'
export type EstadoItem = 'ativo' | 'pausado' | 'arquivado' | 'concluido'
export type StatusOcorrencia = 'aberta' | 'cumprida' | 'perdida' | 'pulada'
export type Origem = 'app' | 'notificacao'

export type Regra =
  | { tipo: 'diario' }
  | { tipo: 'dias_semana'; dias: number[] }
  | { tipo: 'vezes_periodo'; vezes: number; periodo: 'semana' | 'mes' }
  | { tipo: 'intervalo'; dias: number }

export interface IntervaloInfo {
  em_dia: boolean
  dias: number
}

export interface Metricas {
  sequencia_atual: number
  melhor_sequencia: number
  taxa_30d: number | null
  intervalo: IntervaloInfo | null
}

export interface Item {
  id: number
  tipo: TipoItem
  titulo: string
  notas: string
  regra?: Regra
  data?: string | null
  hora?: string
  horarios: string[]
  estado: EstadoItem
  cor?: string
  ordem: number
  criado_em: string
  atualizado_em: string
  metricas?: Metricas
}

export interface Ocorrencia {
  id: number
  inicio?: string
  fim?: string
  meta: number
  status: StatusOcorrencia
  feitos?: number
}

export interface Registro {
  id: number
  em: string
  nota?: string
  origem: Origem
}

export interface HojeHabito {
  item: Item
  ocorrencia: Ocorrencia
  intervalo: IntervaloInfo | null
}

export interface HojeCompromisso {
  item: Item
  ocorrencia: Ocorrencia
  atrasado: boolean
}

export interface HojeCompra {
  lista_id: number
  nome: string
  pendentes: number
}

export interface HojeResponse {
  data: string
  habitos: HojeHabito[]
  compromissos: HojeCompromisso[]
  sem_data: number
  compras: HojeCompra[]
}

// --- fase 3 (compras, CONTRATO-FASE3.md §2/§5) ---

export interface Lembrete {
  regra: Regra
  horarios: string[]
}

export interface Lista {
  id: number
  nome: string
  ordem: number
  arquivada: boolean
  lembrete: Lembrete | null
  criado_em: string
  pendentes?: number
}

export interface Secao {
  id: number
  lista_id: number
  nome: string
  ordem: number
}

export type OrigemItemCompra = 'manual' | 'frequente' | 'receita'

export interface ItemCompra {
  id: number
  lista_id: number
  secao_id: number
  nome: string
  nome_chave: string
  quantidade?: string
  nota?: string
  comprado: boolean
  comprado_em?: string
  origem: OrigemItemCompra
  receita_id: number | null
  criado_em: string
}

export interface ListaDetalhe {
  lista: Lista
  secoes: Secao[]
  itens: ItemCompra[]
}

export interface Frequente {
  nome: string
  nome_chave: string
  secao_id?: number | null
  ultima_quantidade?: string
  vezes: number
  ultima_em: string
}

export interface NovaListaPayload {
  nome: string
}

export interface EditarListaPayload {
  nome?: string
  ordem?: number
  lembrete?: Lembrete | null
}

export interface NovoItemCompraPayload {
  nome: string
  quantidade?: string
  secao_id?: number | null
  origem?: OrigemItemCompra
}

export interface EditarItemCompraPayload {
  nome?: string
  quantidade?: string
  secao_id?: number
  nota?: string
}

export interface NovaSecaoPayload {
  nome: string
}

export interface EditarSecaoPayload {
  nome?: string
  ordem?: number
}

export interface DetalheItemResponse {
  item: Item
  registros: Registro[]
  ocorrencia_aberta: Ocorrencia | null
}

export interface DiaCalendario {
  dia: string
  status: string
  registros: number
}

export interface NovoItemPayload {
  tipo: TipoItem
  titulo: string
  notas?: string
  regra?: Regra
  data?: string
  hora?: string
  horarios?: string[]
  cor?: string
  ordem?: number
}

export interface EditarItemPayload {
  titulo?: string
  notas?: string
  regra?: Regra
  data?: string
  hora?: string
  horarios?: string[]
  cor?: string
  ordem?: number
}

export interface CriarRegistroPayload {
  em?: string
  nota?: string
}

export interface CriarRegistroResponse {
  item: Item
  registro: Registro
}

// --- fase 1.5 (captura por voz e texto, CONTRATO-FASE15.md §2/§3) ---

export interface CapturaAtivaResponse {
  ativa: boolean
}

export interface CapturaRegistroProposta {
  item_id: number
  titulo: string
  tipo: TipoItem
  em: string | null
  nota?: string
}

export interface CapturaComprasItem {
  nome: string
  quantidade?: string
}

export interface CapturaComprasProposta {
  lista_id: number
  lista_nome: string
  itens: CapturaComprasItem[]
}

export type AcaoCaptura = 'criar_item' | 'registrar' | 'compras' | 'desconhecido'

export interface CapturaResposta {
  captura_id: number
  texto: string
  acao: AcaoCaptura
  item: NovoItemPayload | null
  registro: CapturaRegistroProposta | null
  compras: CapturaComprasProposta | null
  resto: string | null
  avisos: string[]
}

export type DesfechoCaptura = 'confirmada' | 'ajustada' | 'descartada'

export interface SaudeResponse {
  ok: boolean
  versao: string
  tick_em: string | null
  cert_expira_em: string | null
  subs: number
}

export interface VapidResponse {
  chave_publica: string
}

// ApiError carrega a mensagem em português já vinda de {"erro": "..."} —
// as telas mostram error.message direto.
export class ApiError extends Error {
  status: number
  // texto: só na captura (CONTRATO-FASE15 §3) — a transcrição que já tinha
  // saído quando a interpretação falhou (429/502), para o recado não se perder.
  texto?: string
  constructor(message: string, status: number, texto?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.texto = texto
  }
}

// NetworkError sinaliza que a requisição nem chegou ao servidor (sem
// conexão com o redmilab) — telas mostram um aviso diferente do ApiError.
export class NetworkError extends Error {
  constructor() {
    super('sem conexão com o redmilab')
    this.name = 'NetworkError'
  }
}

// tratarResposta concentra o "response -> T ou erro" comum a request() e
// requestMultipart() — status não-2xx vira ApiError com a mensagem em
// português de {"erro": "..."}, corpo vazio vira undefined.
async function tratarResposta<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let mensagem = 'erro inesperado'
    let texto: string | undefined
    try {
      const j = await res.json()
      if (j && typeof j.erro === 'string') mensagem = j.erro
      if (j && typeof j.texto === 'string') texto = j.texto
    } catch {
      // corpo não era JSON — mantém a mensagem genérica
    }
    throw new ApiError(mensagem, res.status, texto)
  }

  if (res.status === 204) return undefined as T
  const texto = await res.text()
  if (!texto) return undefined as T
  return JSON.parse(texto) as T
}

// CanceladoError: o usuário desistiu da requisição (ex.: "cancelar" durante
// o "Entendendo…" da captura) — não é falha de rede nem do servidor.
export class CanceladoError extends Error {
  constructor() {
    super('cancelado')
    this.name = 'CanceladoError'
  }
}

// Prazo padrão das chamadas (V10 da fase 1.5): sem ele, com o Tailscale
// ligado em modo avião, o fetch fica pendurado indefinidamente.
const PRAZO_PADRAO_MS = 12_000
const PRAZO_CAPTURA_MS = 75_000

interface OpcoesRede {
  prazoMs?: number
  signal?: AbortSignal // cancelamento pelo usuário
}

// buscar é o fetch com prazo: estourou o prazo ou a rede falhou →
// NetworkError (e o app passa a se considerar offline); o servidor
// respondeu, com qualquer status → online.
async function buscar(path: string, init: RequestInit, opcoes: OpcoesRede = {}): Promise<Response> {
  const ctrl = new AbortController()
  const prazo = window.setTimeout(() => ctrl.abort(), opcoes.prazoMs ?? PRAZO_PADRAO_MS)
  const aoCancelar = () => ctrl.abort()
  opcoes.signal?.addEventListener('abort', aoCancelar)
  try {
    const res = await fetch(path, { ...init, signal: ctrl.signal })
    marcarOnline()
    return res
  } catch {
    if (opcoes.signal?.aborted) throw new CanceladoError()
    marcarOffline()
    throw new NetworkError()
  } finally {
    window.clearTimeout(prazo)
    opcoes.signal?.removeEventListener('abort', aoCancelar)
  }
}

async function request<T>(method: string, path: string, body?: unknown, opcoes?: OpcoesRede): Promise<T> {
  const res = await buscar(
    path,
    {
      method,
      headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    },
    opcoes,
  )
  return tratarResposta<T>(res)
}

// requestMultipart não define Content-Type — o navegador monta o
// multipart/form-data com o boundary certo; forçar JSON aqui quebraria o
// upload de áudio (§3 do contrato).
async function requestMultipart<T>(path: string, form: FormData, opcoes?: OpcoesRede): Promise<T> {
  const res = await buscar(path, { method: 'POST', body: form }, opcoes)
  return tratarResposta<T>(res)
}

export const api = {
  hoje: () => request<HojeResponse>('GET', '/api/hoje'),

  listarItens: (params?: { tipo?: TipoItem; estado?: EstadoItem }) => {
    const qs = new URLSearchParams()
    if (params?.tipo) qs.set('tipo', params.tipo)
    if (params?.estado) qs.set('estado', params.estado)
    const q = qs.toString()
    return request<Item[]>('GET', `/api/itens${q ? `?${q}` : ''}`)
  },

  criarItem: (payload: NovoItemPayload) => request<Item>('POST', '/api/itens', payload),

  detalheItem: (id: number) => request<DetalheItemResponse>('GET', `/api/itens/${id}`),

  editarItem: (id: number, payload: EditarItemPayload) =>
    request<Item>('PATCH', `/api/itens/${id}`, payload),

  mudarEstado: (id: number, estado: EstadoItem) =>
    request<Item>('POST', `/api/itens/${id}/estado`, { estado }),

  calendario: (id: number, de?: string, ate?: string) => {
    const qs = new URLSearchParams()
    if (de) qs.set('de', de)
    if (ate) qs.set('ate', ate)
    const q = qs.toString()
    return request<DiaCalendario[]>('GET', `/api/itens/${id}/calendario${q ? `?${q}` : ''}`)
  },

  criarRegistro: (id: number, payload?: CriarRegistroPayload) =>
    request<CriarRegistroResponse>('POST', `/api/itens/${id}/registros`, payload ?? {}),

  desfazerRegistro: (registroId: number) => request<Item>('DELETE', `/api/registros/${registroId}`),

  editarNotaRegistro: (registroId: number, nota: string) =>
    request<Registro>('PATCH', `/api/registros/${registroId}`, { nota }),

  pular: (ocorrenciaId: number) => request<Item>('POST', `/api/ocorrencias/${ocorrenciaId}/pular`),
  despular: (ocorrenciaId: number) => request<Item>('POST', `/api/ocorrencias/${ocorrenciaId}/despular`),

  vapid: () => request<VapidResponse>('GET', '/api/push/vapid'),
  pushSubscribe: (sub: PushSubscriptionJSON) => request<{ ok: boolean }>('POST', '/api/push/subscribe', sub),
  pushTeste: () => request<{ enviados: number }>('POST', '/api/push/teste'),

  saude: () => request<SaudeResponse>('GET', '/health'),

  // --- compras (fase 3) ---

  listarListas: () => request<Lista[]>('GET', '/api/listas'),

  criarLista: (payload: NovaListaPayload) => request<Lista>('POST', '/api/listas', payload),

  editarLista: (id: number, payload: EditarListaPayload) =>
    request<Lista>('PATCH', `/api/listas/${id}`, payload),

  mudarEstadoLista: (id: number, arquivada: boolean) =>
    request<Lista>('POST', `/api/listas/${id}/estado`, { arquivada }),

  detalheLista: (id: number) => request<ListaDetalhe>('GET', `/api/listas/${id}`),

  adicionarItemCompra: (listaId: number, payload: NovoItemCompraPayload) =>
    request<ItemCompra>('POST', `/api/listas/${listaId}/itens`, payload),

  editarItemCompra: (id: number, payload: EditarItemCompraPayload) =>
    request<ItemCompra>('PATCH', `/api/compras/${id}`, payload),

  riscar: (id: number) => request<ItemCompra>('POST', `/api/compras/${id}/comprado`),

  desriscar: (id: number) => request<ItemCompra>('POST', `/api/compras/${id}/desmarcar`),

  apagarItemCompra: (id: number) => request<void>('DELETE', `/api/compras/${id}`),

  limparComprados: (listaId: number) =>
    request<{ removidos: number }>('POST', `/api/listas/${listaId}/limpar-comprados`),

  frequentes: (listaId: number, q?: string) => {
    const qs = new URLSearchParams()
    if (q) qs.set('q', q)
    const query = qs.toString()
    return request<Frequente[]>('GET', `/api/listas/${listaId}/frequentes${query ? `?${query}` : ''}`)
  },

  criarSecao: (listaId: number, payload: NovaSecaoPayload) =>
    request<Secao>('POST', `/api/listas/${listaId}/secoes`, payload),

  editarSecao: (id: number, payload: EditarSecaoPayload) =>
    request<Secao>('PATCH', `/api/secoes/${id}`, payload),

  apagarSecao: (id: number) => request<void>('DELETE', `/api/secoes/${id}`),

  // --- captura (fase 1.5) ---

  capturaEstado: () => request<CapturaAtivaResponse>('GET', '/api/captura'),

  // Prazo longo: no pior caso o servidor transcreve (20 s) e interpreta
  // (15 s), cada um com uma nova tentativa.
  capturarTexto: (texto: string, signal?: AbortSignal) =>
    request<CapturaResposta>('POST', '/api/captura', { texto }, { prazoMs: PRAZO_CAPTURA_MS, signal }),

  capturarAudio: (blob: Blob, signal?: AbortSignal) => {
    // nome do arquivo pelo mime — o Go escolhe o decodificador por ele.
    const extensao = blob.type.includes('ogg') ? 'ogg' : blob.type.includes('mp4') ? 'mp4' : 'webm'
    const form = new FormData()
    form.append('audio', blob, `audio.${extensao}`)
    return requestMultipart<CapturaResposta>('/api/captura', form, { prazoMs: PRAZO_CAPTURA_MS, signal })
  },

  // fire-and-forget (§3): o front avisa o desfecho, mas uma falha aqui não
  // deve incomodar quem já confirmou/descartou a proposta.
  desfechoCaptura: (id: number, desfecho: DesfechoCaptura): void => {
    request('POST', `/api/captura/${id}/desfecho`, { desfecho }).catch(() => {})
  },
}
