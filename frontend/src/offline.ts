// Fila offline da aba Compras (CONTRATO-FASE3 §3, decisão F2) — IndexedDB
// puro, sem lib. Guarda (a) a última resposta de GET /api/listas e de
// GET /api/listas/{id}, mostradas se a rede cair, e (b) a fila de operações
// otimistas (riscar/desriscar/adicionar), drenada em ordem quando a rede
// volta. Só este módulo toca IndexedDB — Compras.tsx e App.tsx chamam as
// funções daqui.
import { api, ApiError, NetworkError } from './api'
import { estaOnline } from './conexao'
import type { ItemCompra, Lista, ListaDetalhe, NovoItemCompraPayload } from './api'

const DB_NOME = 'pocket-compras'
const DB_VERSAO = 1
const LOJA_CACHE_LISTAS = 'cache_listas'
const LOJA_CACHE_DETALHE = 'cache_detalhe'
const LOJA_FILA = 'fila'

export type TipoOperacao = 'riscar' | 'desriscar' | 'adicionar'

export interface OperacaoFila {
  id?: number
  tipo: TipoOperacao
  lista_id: number
  item_id?: number
  payload?: NovoItemCompraPayload
  id_provisorio?: number
  criado_em: string
}

let dbPromise: Promise<IDBDatabase> | null = null

function abrirDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise
  dbPromise = new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NOME, DB_VERSAO)
    req.onupgradeneeded = () => {
      const db = req.result
      if (!db.objectStoreNames.contains(LOJA_CACHE_LISTAS)) db.createObjectStore(LOJA_CACHE_LISTAS)
      if (!db.objectStoreNames.contains(LOJA_CACHE_DETALHE)) db.createObjectStore(LOJA_CACHE_DETALHE)
      if (!db.objectStoreNames.contains(LOJA_FILA)) db.createObjectStore(LOJA_FILA, { keyPath: 'id', autoIncrement: true })
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
  return dbPromise
}

function comLoja<T>(loja: string, modo: IDBTransactionMode, fn: (s: IDBObjectStore) => IDBRequest): Promise<T> {
  return abrirDB().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const tx = db.transaction(loja, modo)
        const store = tx.objectStore(loja)
        const req = fn(store)
        req.onsuccess = () => resolve(req.result as T)
        req.onerror = () => reject(req.error)
      }),
  )
}

// ---------- cache de leitura ----------

export async function cachearListas(listas: Lista[]): Promise<void> {
  try {
    await comLoja(LOJA_CACHE_LISTAS, 'readwrite', (s) => s.put(listas, 'todas'))
  } catch {
    // IndexedDB indisponível (ex.: modo privado) — segue sem cache
  }
}

export async function listasCacheadas(): Promise<Lista[] | null> {
  try {
    const r = await comLoja<Lista[] | undefined>(LOJA_CACHE_LISTAS, 'readonly', (s) => s.get('todas'))
    return r ?? null
  } catch {
    return null
  }
}

export async function cachearDetalhe(listaId: number, detalhe: ListaDetalhe): Promise<void> {
  try {
    await comLoja(LOJA_CACHE_DETALHE, 'readwrite', (s) => s.put(detalhe, listaId))
  } catch {
    // sem cache
  }
}

export async function detalheCacheado(listaId: number): Promise<ListaDetalhe | null> {
  try {
    const r = await comLoja<ListaDetalhe | undefined>(LOJA_CACHE_DETALHE, 'readonly', (s) => s.get(listaId))
    return r ?? null
  } catch {
    return null
  }
}

// ---------- fila de operações ----------

export async function enfileirar(op: Omit<OperacaoFila, 'id' | 'criado_em'>): Promise<number> {
  const registro: OperacaoFila = { ...op, criado_em: new Date().toISOString() }
  const id = await comLoja<number>(LOJA_FILA, 'readwrite', (s) => s.add(registro))
  return id
}

export async function listarFila(): Promise<OperacaoFila[]> {
  try {
    return await comLoja<OperacaoFila[]>(LOJA_FILA, 'readonly', (s) => s.getAll())
  } catch {
    return []
  }
}

export async function tamanhoFila(): Promise<number> {
  try {
    return await comLoja<number>(LOJA_FILA, 'readonly', (s) => s.count())
  } catch {
    return 0
  }
}

async function removerDaFila(id: number): Promise<void> {
  await comLoja(LOJA_FILA, 'readwrite', (s) => s.delete(id))
}

// atualizarIdProvisorio troca, nas operações ainda enfileiradas, o id
// provisório negativo de um item recém-adicionado offline pelo id real
// devolvido pelo servidor — para que um riscar/apagar posterior na mesma
// sessão offline aponte pro item certo.
async function atualizarIdProvisorio(provisorio: number, real: number): Promise<void> {
  const fila = await listarFila()
  for (const op of fila) {
    if (op.item_id === provisorio && op.id !== undefined) {
      await comLoja(LOJA_FILA, 'readwrite', (s) => s.put({ ...op, item_id: real }))
    }
  }
}

export interface ResultadoDrenagem {
  processadas: number
  itemAtualizado?: { idProvisorio: number; itemReal: ItemCompra }
  erro?: string
  parou: boolean
}

let drenando = false

// drenarFila processa a fila em ordem (FIFO). Erro 4xx de uma operação:
// descarta a operação e sinaliza recarregar a lista (não trava a fila).
// Erro de rede: para e tenta depois (mesma fila intacta).
export async function drenarFila(onItemReal?: (idProvisorio: number, itemReal: ItemCompra) => void, onRecarregar?: () => void): Promise<ResultadoDrenagem> {
  if (drenando) return { processadas: 0, parou: true }
  if (!estaOnline()) return { processadas: 0, parou: true }
  drenando = true
  let processadas = 0
  try {
    for (;;) {
      const fila = await listarFila()
      const proxima = fila.sort((a, b) => (a.id ?? 0) - (b.id ?? 0))[0]
      if (!proxima) break

      try {
        if (proxima.tipo === 'riscar' && proxima.item_id !== undefined) {
          await api.riscar(proxima.item_id)
        } else if (proxima.tipo === 'desriscar' && proxima.item_id !== undefined) {
          await api.desriscar(proxima.item_id)
        } else if (proxima.tipo === 'adicionar' && proxima.payload) {
          const real = await api.adicionarItemCompra(proxima.lista_id, proxima.payload)
          if (proxima.item_id !== undefined && proxima.item_id < 0) {
            await atualizarIdProvisorio(proxima.item_id, real.id)
            onItemReal?.(proxima.item_id, real)
          }
        }
        if (proxima.id !== undefined) await removerDaFila(proxima.id)
        processadas++
      } catch (e) {
        if (e instanceof NetworkError) {
          // rede caiu no meio da drenagem — para aqui, tenta depois
          return { processadas, parou: true, erro: e.message }
        }
        if (e instanceof ApiError && e.status >= 400 && e.status < 500) {
          // operação inválida (ex.: item já apagado no servidor): descarta
          // e recarrega a lista para reconciliar a tela com o servidor
          if (proxima.id !== undefined) await removerDaFila(proxima.id)
          onRecarregar?.()
          continue
        }
        // erro 5xx ou inesperado: trata como falha de rede — para e tenta depois
        return { processadas, parou: true, erro: e instanceof Error ? e.message : 'erro inesperado' }
      }
    }
    return { processadas, parou: false }
  } finally {
    drenando = false
  }
}
