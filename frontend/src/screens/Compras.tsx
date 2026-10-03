import { useEffect, useState, useCallback, useRef, useMemo } from 'react'
import { api, ApiError, NetworkError } from '../api'
import type { Lista, ListaDetalhe, ItemCompra, Frequente, Secao } from '../api'
import { parseItem } from '../parseItem'
import * as offline from '../offline'
import { useOnline } from '../conexao'
import { IconeAjustes, IconeMenuPontos, IconeFechar } from '../icons'
import AjustesListaSheet from '../components/AjustesListaSheet'

interface Props {
  // id de lista vindo de fora (Hoje, ou ?lista= do sw.js) — abre direto
  // nessa lista uma vez, depois App.tsx "consome" o valor.
  listaAlvoId: number | null
  onListaAlvoConsumida: () => void
  // incrementado pelo botão "+" flutuante quando esta aba está ativa —
  // useEffect dispara o foco no campo de adicionar (item 2 do pedido).
  focoChave: number
}

// chaveAproximada é uma normalização simples (minúsculas, sem acento, sem
// espaço duplicado) só para o dedupe/otimismo do front — o servidor é a
// fonte da verdade de nome_chave (§2.5, com regra de plural); replicar a
// regra exata aqui não valia o risco de divergir dela silenciosamente.
function chaveAproximada(nome: string): string {
  return nome
    .trim()
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/\s+/g, ' ')
}

export default function Compras({ listaAlvoId, onListaAlvoConsumida, focoChave }: Props) {
  const [listas, setListas] = useState<Lista[] | null>(null)
  const [listaAtivaId, setListaAtivaId] = useState<number | null>(null)
  const [detalhe, setDetalhe] = useState<ListaDetalhe | null>(null)
  const [carregando, setCarregando] = useState(true)
  const [erro, setErro] = useState<string | null>(null)
  const [deCache, setDeCache] = useState(false)
  const [filaTamanho, setFilaTamanho] = useState(0)
  const online = useOnline()

  const [texto, setTexto] = useState('')
  const [sugestoes, setSugestoes] = useState<Frequente[]>([])
  const [frequentesChips, setFrequentesChips] = useState<Frequente[]>([])
  const inputRef = useRef<HTMLInputElement>(null)
  const debounceRef = useRef<number | null>(null)

  const [acoesItem, setAcoesItem] = useState<ItemCompra | null>(null)
  const [editandoItem, setEditandoItem] = useState<{ item: ItemCompra; nome: string; quantidade: string } | null>(null)
  const [movendoItem, setMovendoItem] = useState<ItemCompra | null>(null)
  const [ajustesAberto, setAjustesAberto] = useState(false)
  const pressTimer = useRef<number | null>(null)
  const pressDisparou = useRef(false)


  const carregarListas = useCallback(async (preferirId?: number | null) => {
    try {
      const l = await api.listarListas()
      setListas(l)
      offline.cachearListas(l)
      setDeCache(false)
      return l
    } catch (e) {
      const cache = await offline.listasCacheadas()
      if (cache) {
        setListas(cache)
        setDeCache(true)
        return cache
      }
      setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
      return null
    } finally {
      void preferirId
    }
  }, [])

  const carregarDetalhe = useCallback(async (id: number) => {
    setCarregando(true)
    setErro(null)
    try {
      const d = await api.detalheLista(id)
      setDetalhe(d)
      offline.cachearDetalhe(id, d)
      setDeCache(false)
    } catch (e) {
      const cache = await offline.detalheCacheado(id)
      if (cache) {
        setDetalhe(cache)
        setDeCache(true)
      } else {
        setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
      }
    } finally {
      setCarregando(false)
    }
  }, [])

  // carga inicial: lista alvo (Hoje/notificação) tem prioridade; senão a
  // primeira não arquivada.
  useEffect(() => {
    ;(async () => {
      const l = await carregarListas()
      if (!l || l.length === 0) {
        // sem listas (ou erro sem cache): não há detalhe a carregar, então
        // ninguém mais desliga "carregando" — desliga aqui.
        setCarregando(false)
        return
      }
      const alvo = listaAlvoId && l.some((x) => x.id === listaAlvoId) ? listaAlvoId : l[0].id
      setListaAtivaId(alvo)
      if (listaAlvoId) onListaAlvoConsumida()
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (listaAlvoId && listas?.some((x) => x.id === listaAlvoId)) {
      setListaAtivaId(listaAlvoId)
      onListaAlvoConsumida()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [listaAlvoId])

  useEffect(() => {
    if (listaAtivaId !== null) carregarDetalhe(listaAtivaId)
  }, [listaAtivaId, carregarDetalhe])

  useEffect(() => {
    if (focoChave > 0) inputRef.current?.focus()
  }, [focoChave])

  const atualizarFilaTamanho = useCallback(() => {
    offline.tamanhoFila().then(setFilaTamanho)
  }, [])

  useEffect(() => {
    atualizarFilaTamanho()
  }, [atualizarFilaTamanho])

  const dispararDrenagem = useCallback(() => {
    offline
      .drenarFila(
        (idProvisorio, itemReal) => {
          setDetalhe((d) => {
            if (!d) return d
            // já existia um pendente com o mesmo nome (dedupe do servidor,
            // §2.3 regra 1): funde em vez de duplicar.
            const jaExistiaOutro = d.itens.some((it) => it.id === itemReal.id && it.id !== idProvisorio)
            const semProvisorio = d.itens.filter((it) => it.id !== idProvisorio)
            return { ...d, itens: jaExistiaOutro ? semProvisorio : [...semProvisorio, itemReal] }
          })
        },
        () => {
          if (listaAtivaId !== null) carregarDetalhe(listaAtivaId)
        },
      )
      .finally(atualizarFilaTamanho)
  }, [listaAtivaId, carregarDetalhe, atualizarFilaTamanho])

  // a conexão com o redmilab voltou (conexao.ts, que não confia em
  // navigator.onLine): drena o que ficou na fila.
  useEffect(() => {
    if (online) dispararDrenagem()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [online])

  // drena ao abrir a tela, quando a rede volta, e a cada 20s enquanto
  // houver fila (§3).
  useEffect(() => {
    dispararDrenagem()
    const onOnline = () => dispararDrenagem()
    window.addEventListener('online', onOnline)
    const intervalo = window.setInterval(() => {
      offline.tamanhoFila().then((n) => n > 0 && dispararDrenagem())
    }, 20000)
    return () => {
      window.removeEventListener('online', onOnline)
      window.clearInterval(intervalo)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [listaAtivaId])

  // autocompletar dos frequentes com debounce
  useEffect(() => {
    if (!listaAtivaId || !online) {
      setSugestoes([])
      return
    }
    const q = parseItem(texto).nome.trim()
    if (!q) {
      setSugestoes([])
      return
    }
    if (debounceRef.current) window.clearTimeout(debounceRef.current)
    debounceRef.current = window.setTimeout(() => {
      api
        .frequentes(listaAtivaId, q)
        .then(setSugestoes)
        .catch(() => setSugestoes([]))
    }, 250)
    return () => {
      if (debounceRef.current) window.clearTimeout(debounceRef.current)
    }
  }, [texto, listaAtivaId, online])

  // chips "de novo?": frequentes que não estão pendentes na lista
  useEffect(() => {
    if (!listaAtivaId || !online) {
      setFrequentesChips([])
      return
    }
    api
      .frequentes(listaAtivaId)
      .then((freq) => {
        const pendentesChave = new Set((detalhe?.itens ?? []).filter((i) => !i.comprado).map((i) => i.nome_chave))
        setFrequentesChips(freq.filter((f) => !pendentesChave.has(f.nome_chave)).slice(0, 12))
      })
      .catch(() => setFrequentesChips([]))
  }, [listaAtivaId, online, detalhe?.itens])

  function fecharSugestoes() {
    setSugestoes([])
  }

  async function adicionarItem(nomeRaw: string, origem: 'manual' | 'frequente' = 'manual', secaoIdSugerida?: number) {
    if (!listaAtivaId || !detalhe) return
    const parsed = parseItem(nomeRaw)
    if (!parsed.nome) return
    fecharSugestoes()
    setTexto('')

    const chave = chaveAproximada(parsed.nome)
    const existente = detalhe.itens.find((it) => !it.comprado && chaveAproximada(it.nome) === chave)
    const secaoAlvo =
      (secaoIdSugerida && detalhe.secoes.find((s) => s.id === secaoIdSugerida)) ||
      detalhe.secoes.find((s) => s.nome.toLowerCase() === 'outros') ||
      detalhe.secoes[0]

    if (existente) {
      // regra 1 (§2.3): já pendente — otimista, só atualiza a quantidade.
      const itemAtualizado: ItemCompra = { ...existente, quantidade: parsed.quantidade ?? existente.quantidade }
      setDetalhe((d) => (d ? { ...d, itens: d.itens.map((it) => (it.id === existente.id ? itemAtualizado : it)) } : d))
      await offline.enfileirar({
        tipo: 'adicionar',
        lista_id: listaAtivaId,
        item_id: existente.id,
        payload: { nome: parsed.nome, quantidade: parsed.quantidade, origem },
      })
    } else {
      const idProvisorio = -Date.now()
      const itemOtimista: ItemCompra = {
        id: idProvisorio,
        lista_id: listaAtivaId,
        secao_id: secaoAlvo?.id ?? 0,
        nome: parsed.nome,
        nome_chave: chave,
        quantidade: parsed.quantidade ?? '',
        nota: '',
        comprado: false,
        comprado_em: '',
        origem,
        receita_id: null,
        criado_em: new Date().toISOString(),
      }
      setDetalhe((d) => (d ? { ...d, itens: [...d.itens, itemOtimista] } : d))
      await offline.enfileirar({
        tipo: 'adicionar',
        lista_id: listaAtivaId,
        item_id: idProvisorio,
        payload: { nome: parsed.nome, quantidade: parsed.quantidade, origem, secao_id: secaoIdSugerida ?? undefined },
      })
    }
    atualizarFilaTamanho()
    dispararDrenagem()
  }

  async function alternarComprado(item: ItemCompra) {
    const comprado = !item.comprado
    setDetalhe((d) =>
      d
        ? {
            ...d,
            itens: d.itens.map((it) =>
              it.id === item.id ? { ...it, comprado, comprado_em: comprado ? new Date().toISOString() : '' } : it,
            ),
          }
        : d,
    )
    if (item.id > 0) {
      await offline.enfileirar({
        tipo: comprado ? 'riscar' : 'desriscar',
        lista_id: item.lista_id,
        item_id: item.id,
      })
      atualizarFilaTamanho()
      dispararDrenagem()
    }
  }

  async function limparComprados() {
    if (!listaAtivaId || !online) return
    try {
      await api.limparComprados(listaAtivaId)
      carregarDetalhe(listaAtivaId)
    } catch (e) {
      setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
    }
  }

  async function criarLista() {
    const nome = window.prompt('Nome da nova lista')
    if (!nome || !nome.trim()) return
    try {
      const l = await api.criarLista({ nome: nome.trim() })
      const atualizadas = await carregarListas()
      setListaAtivaId(l.id)
      void atualizadas
    } catch (e) {
      setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
    }
  }

  function iniciarPress(item: ItemCompra) {
    pressDisparou.current = false
    pressTimer.current = window.setTimeout(() => {
      pressDisparou.current = true
      setAcoesItem(item)
    }, 450)
  }

  function cancelarPress() {
    if (pressTimer.current) window.clearTimeout(pressTimer.current)
  }

  function aoTocarItem(item: ItemCompra) {
    if (pressDisparou.current) {
      pressDisparou.current = false
      return
    }
    alternarComprado(item)
  }

  async function salvarEdicaoItem() {
    if (!editandoItem || !online) return
    try {
      const atualizado = await api.editarItemCompra(editandoItem.item.id, {
        nome: editandoItem.nome.trim() || undefined,
        quantidade: editandoItem.quantidade,
      })
      setDetalhe((d) => (d ? { ...d, itens: d.itens.map((it) => (it.id === atualizado.id ? atualizado : it)) } : d))
      setEditandoItem(null)
    } catch (e) {
      setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
    }
  }

  async function moverItemParaSecao(item: ItemCompra, secaoId: number) {
    if (!online) return
    try {
      const atualizado = await api.editarItemCompra(item.id, { secao_id: secaoId })
      setDetalhe((d) => (d ? { ...d, itens: d.itens.map((it) => (it.id === atualizado.id ? atualizado : it)) } : d))
      setMovendoItem(null)
    } catch (e) {
      setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
    }
  }

  async function apagarItem(item: ItemCompra) {
    if (!online) return
    try {
      await api.apagarItemCompra(item.id)
      setDetalhe((d) => (d ? { ...d, itens: d.itens.filter((it) => it.id !== item.id) } : d))
      setAcoesItem(null)
    } catch (e) {
      setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
    }
  }

  const grupos = useMemo(() => {
    if (!detalhe) return []
    const secoesOrdenadas = [...detalhe.secoes].sort((a, b) => a.ordem - b.ordem)
    return secoesOrdenadas
      .map((s) => {
        const itensSecao = detalhe.itens.filter((it) => it.secao_id === s.id)
        const pendentes = itensSecao.filter((it) => !it.comprado)
        const riscados = itensSecao.filter((it) => it.comprado)
        return { secao: s, pendentes, riscados }
      })
      .filter((g) => g.pendentes.length > 0 || g.riscados.length > 0)
  }, [detalhe])

  const totalRiscados = useMemo(() => (detalhe ? detalhe.itens.filter((i) => i.comprado).length : 0), [detalhe])

  if (carregando && !detalhe && !listas) {
    return <div className="estado-carregando">carregando…</div>
  }

  if (erro && !listas) {
    return (
      <div className="estado-erro">
        {erro}
        <div style={{ marginTop: 10 }}>
          <button className="botao" onClick={() => carregarListas()}>
            tentar de novo
          </button>
        </div>
      </div>
    )
  }

  if (!listas || listas.length === 0) {
    return (
      <div className="estado-vazio">
        <div className="titulo">nenhuma lista</div>
        <p>toque em "+ lista" para criar a primeira</p>
        <div style={{ marginTop: 14 }}>
          <button className="botao primario" onClick={criarLista}>
            + lista
          </button>
        </div>
      </div>
    )
  }

  const listaAtiva = listas.find((l) => l.id === listaAtivaId) ?? null

  return (
    <div>
      {(!online || filaTamanho > 0) && (
        <div className="banner-offline">
          {!online ? 'sem conexão' : 'enviando'}
          {filaTamanho > 0
            ? ` — ${filaTamanho} ${filaTamanho === 1 ? 'mudança pendente' : 'mudanças pendentes'}`
            : ' — as mudanças serão enviadas quando a rede voltar'}
        </div>
      )}
      {deCache && online && <div className="banner-offline">mostrando dados salvos — atualizando…</div>}
      {erro && <div className="estado-erro">{erro}</div>}

      {/* chips de lista */}
      <div className="chips-lista-compras" role="tablist" aria-label="Listas de compras">
        {listas.map((l) => (
          <button
            key={l.id}
            className="chip-lista"
            role="tab"
            aria-selected={l.id === listaAtivaId}
            onClick={() => setListaAtivaId(l.id)}
          >
            {l.nome}
            {/* Lista aberta: conta pela tela (riscar/adicionar é otimista); as outras, pelo servidor. */}
            {(() => {
              const n = l.id === listaAtivaId && detalhe ? detalhe.itens.filter((i) => !i.comprado).length : l.pendentes
              return typeof n === 'number' && <span className="chip-lista-contagem">{n}</span>
            })()}
          </button>
        ))}
        <button className="chip-lista chip-lista-nova" onClick={criarLista}>
          + lista
        </button>
      </div>

      {listaAtiva && (
        <>
          <div className="compras-cabecalho-lista">
            <h2 style={{ marginBottom: 0 }}>{listaAtiva.nome}</h2>
            <button className="icone-botao" aria-label="ajustes da lista" onClick={() => setAjustesAberto(true)}>
              <IconeAjustes />
            </button>
          </div>

          {/* campo adicionar item */}
          <div className="campo-adicionar-item">
            <input
              ref={inputRef}
              type="text"
              placeholder='adicionar item — ex.: "leite 2"'
              value={texto}
              onChange={(e) => setTexto(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && texto.trim()) adicionarItem(texto)
              }}
            />
            <button className="botao primario" disabled={!texto.trim()} onClick={() => adicionarItem(texto)}>
              adicionar
            </button>
            {sugestoes.length > 0 && (
              <ul className="lista-sugestoes">
                {sugestoes.map((s) => (
                  <li key={s.nome_chave}>
                    <button
                      className="linha-toque"
                      onClick={() => adicionarItem(`${s.nome}${s.ultima_quantidade ? ' ' + s.ultima_quantidade : ''}`, 'frequente', s.secao_id ?? undefined)}
                    >
                      {s.nome}
                      {s.ultima_quantidade ? ` · ${s.ultima_quantidade}` : ''}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {frequentesChips.length > 0 && (
            <div className="secao">
              <h2>de novo?</h2>
              <div className="chips-frequentes">
                {frequentesChips.map((f) => (
                  <button
                    key={f.nome_chave}
                    className="chip chip-ouro"
                    onClick={() => adicionarItem(`${f.nome}${f.ultima_quantidade ? ' ' + f.ultima_quantidade : ''}`, 'frequente', f.secao_id ?? undefined)}
                  >
                    {f.nome}
                  </button>
                ))}
              </div>
            </div>
          )}

          {grupos.length === 0 ? (
            <div className="estado-vazio">
              <div className="titulo">lista vazia</div>
              <p>adicione o primeiro item acima</p>
            </div>
          ) : (
            grupos.map(({ secao, pendentes, riscados }) => (
              <section className="secao" key={secao.id}>
                <h2>{secao.nome}</h2>
                <div className="cartao">
                  {pendentes.map((item) => (
                    <LinhaItemCompra
                      key={item.id}
                      item={item}
                      onTocar={() => aoTocarItem(item)}
                      onPressStart={() => iniciarPress(item)}
                      onPressEnd={cancelarPress}
                      onAcoes={() => setAcoesItem(item)}
                    />
                  ))}
                  {riscados.map((item) => (
                    <LinhaItemCompra
                      key={item.id}
                      item={item}
                      onTocar={() => aoTocarItem(item)}
                      onPressStart={() => iniciarPress(item)}
                      onPressEnd={cancelarPress}
                      onAcoes={() => setAcoesItem(item)}
                    />
                  ))}
                </div>
              </section>
            ))
          )}

          {totalRiscados > 0 && (
            <div style={{ marginTop: 18 }}>
              <button className="botao botao-largo" disabled={!online} onClick={limparComprados}>
                Limpar comprados ({totalRiscados})
              </button>
            </div>
          )}
        </>
      )}

      {ajustesAberto && listaAtiva && detalhe && (
        <AjustesListaSheet
          lista={listaAtiva}
          secoes={detalhe.secoes}
          online={online}
          onFechar={() => setAjustesAberto(false)}
          onAtualizada={() => {
            carregarListas()
            if (listaAtivaId) carregarDetalhe(listaAtivaId)
          }}
          onArquivada={() => {
            setAjustesAberto(false)
            carregarListas().then((l) => {
              if (l && l.length > 0) setListaAtivaId(l[0].id)
              else setListaAtivaId(null)
            })
          }}
        />
      )}

      {acoesItem && (
        <AcoesItemSheet
          item={acoesItem}
          secoes={detalhe?.secoes ?? []}
          online={online}
          onFechar={() => setAcoesItem(null)}
          onEditar={() => {
            setEditandoItem({ item: acoesItem, nome: acoesItem.nome, quantidade: acoesItem.quantidade ?? '' })
            setAcoesItem(null)
          }}
          onMover={() => {
            setMovendoItem(acoesItem)
            setAcoesItem(null)
          }}
          onApagar={() => apagarItem(acoesItem)}
        />
      )}

      {editandoItem && (
        <div className="folha-fundo" onClick={() => setEditandoItem(null)}>
          <div className="folha" role="dialog" aria-modal="true" aria-label="Editar item" onClick={(e) => e.stopPropagation()}>
            <div className="folha-cabecalho">
              <h2>Editar item</h2>
              <button className="icone-botao" aria-label="fechar" onClick={() => setEditandoItem(null)}>
                <IconeFechar />
              </button>
            </div>
            <div className="campo">
              <label htmlFor="edit-nome">Nome</label>
              <input
                id="edit-nome"
                type="text"
                value={editandoItem.nome}
                maxLength={60}
                onChange={(e) => setEditandoItem({ ...editandoItem, nome: e.target.value })}
              />
            </div>
            <div className="campo">
              <label htmlFor="edit-qtd">Quantidade</label>
              <input
                id="edit-qtd"
                type="text"
                value={editandoItem.quantidade}
                onChange={(e) => setEditandoItem({ ...editandoItem, quantidade: e.target.value })}
              />
            </div>
            <div className="folha-rodape">
              <button className="botao" onClick={() => setEditandoItem(null)}>
                cancelar
              </button>
              <button className="botao primario" onClick={salvarEdicaoItem}>
                salvar
              </button>
            </div>
          </div>
        </div>
      )}

      {movendoItem && (
        <div className="folha-fundo" onClick={() => setMovendoItem(null)}>
          <div className="folha" role="dialog" aria-modal="true" aria-label="Mover de seção" onClick={(e) => e.stopPropagation()}>
            <div className="folha-cabecalho">
              <h2>Mover para</h2>
              <button className="icone-botao" aria-label="fechar" onClick={() => setMovendoItem(null)}>
                <IconeFechar />
              </button>
            </div>
            <div className="lista">
              {(detalhe?.secoes ?? [])
                .slice()
                .sort((a, b) => a.ordem - b.ordem)
                .map((s) => (
                  <button key={s.id} className="linha-toque cartao" onClick={() => moverItemParaSecao(movendoItem, s.id)}>
                    {s.nome}
                  </button>
                ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function LinhaItemCompra({
  item,
  onTocar,
  onPressStart,
  onPressEnd,
  onAcoes,
}: {
  item: ItemCompra
  onTocar: () => void
  onPressStart: () => void
  onPressEnd: () => void
  onAcoes: () => void
}) {
  return (
    <div className="item-lista item-compra">
      <button
        className={`linha-toque item-lista-corpo ${item.comprado ? 'item-comprado' : ''}`}
        onClick={onTocar}
        onPointerDown={onPressStart}
        onPointerUp={onPressEnd}
        onPointerLeave={onPressEnd}
        aria-label={`${item.comprado ? 'desmarcar' : 'marcar'} ${item.nome}`}
      >
        <div className={`item-lista-titulo ${item.comprado ? 'riscado' : ''}`}>
          {item.nome}
          {item.quantidade ? <span className="item-quantidade"> · {item.quantidade}</span> : null}
        </div>
      </button>
      <button className="icone-botao" aria-label={`ações para ${item.nome}`} onClick={onAcoes}>
        <IconeMenuPontos />
      </button>
    </div>
  )
}

function AcoesItemSheet({
  item,
  secoes,
  online,
  onFechar,
  onEditar,
  onMover,
  onApagar,
}: {
  item: ItemCompra
  secoes: Secao[]
  online: boolean
  onFechar: () => void
  onEditar: () => void
  onMover: () => void
  onApagar: () => void
}) {
  void secoes
  return (
    <div className="folha-fundo" onClick={onFechar}>
      <div className="folha" role="dialog" aria-modal="true" aria-label={`Ações para ${item.nome}`} onClick={(e) => e.stopPropagation()}>
        <div className="folha-cabecalho">
          <h2>{item.nome}</h2>
          <button className="icone-botao" aria-label="fechar" onClick={onFechar}>
            <IconeFechar />
          </button>
        </div>
        <div className="lista">
          <button className="botao botao-largo" disabled={!online} onClick={onEditar}>
            editar nome/quantidade
          </button>
          <button className="botao botao-largo" disabled={!online} onClick={onMover}>
            mover de seção
          </button>
          <button className="botao perigo botao-largo" disabled={!online} onClick={onApagar}>
            apagar
          </button>
        </div>
        {!online && <p style={{ marginTop: 10, color: 'var(--fg-mudo)', fontSize: 13 }}>essas ações exigem rede</p>}
      </div>
    </div>
  )
}
