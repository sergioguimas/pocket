import { useEffect, useState, useCallback, useMemo } from 'react'
import { api, ApiError, NetworkError } from '../api'
import type { Item } from '../api'
import { formatDataLonga, hojeISO } from '../format'
import { IconeCheck } from '../icons'

interface Props {
  onAbrirEditar: (item: Item) => void
}

export default function Compromissos({ onAbrirEditar }: Props) {
  const [itens, setItens] = useState<Item[] | null>(null)
  const [erro, setErro] = useState<string | null>(null)
  const [carregando, setCarregando] = useState(true)

  const carregar = useCallback(() => {
    setCarregando(true)
    setErro(null)
    api
      .listarItens({ tipo: 'compromisso', estado: 'ativo' })
      .then(setItens)
      .catch((e) => setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado'))
      .finally(() => setCarregando(false))
  }, [])

  useEffect(carregar, [carregar])

  const hoje = hojeISO()

  const { atrasados, proximos, semData } = useMemo(() => {
    const atrasados: Item[] = []
    const proximos: Item[] = []
    const semData: Item[] = []
    for (const item of itens ?? []) {
      if (!item.data) semData.push(item)
      else if (item.data < hoje) atrasados.push(item)
      else proximos.push(item)
    }
    proximos.sort((a, b) => (a.data ?? '').localeCompare(b.data ?? ''))
    atrasados.sort((a, b) => (a.data ?? '').localeCompare(b.data ?? ''))
    return { atrasados, proximos, semData }
  }, [itens, hoje])

  async function concluir(itemId: number) {
    try {
      await api.mudarEstado(itemId, 'concluido')
      carregar()
    } catch (e) {
      setErro(e instanceof Error ? e.message : 'erro inesperado')
    }
  }

  if (carregando && !itens) return <div className="estado-carregando">carregando…</div>

  if (erro && !itens) {
    return (
      <div className="estado-erro">
        {erro}
        <div style={{ marginTop: 10 }}>
          <button className="botao" onClick={carregar}>
            tentar de novo
          </button>
        </div>
      </div>
    )
  }

  if (!itens) return null

  if (itens.length === 0) {
    return (
      <div className="estado-vazio">
        <div className="titulo">nenhum compromisso</div>
        <p>toque em + para criar o primeiro</p>
      </div>
    )
  }

  return (
    <div>
      {erro && <div className="estado-erro">{erro}</div>}

      {atrasados.length > 0 && (
        <section className="secao">
          <h2>Atrasados</h2>
          <div className="cartao">
            {atrasados.map((item) => (
              <LinhaCompromisso key={item.id} item={item} destaque onConcluir={concluir} onEditar={onAbrirEditar} />
            ))}
          </div>
        </section>
      )}

      {proximos.length > 0 && (
        <section className="secao">
          <h2>Próximos</h2>
          <div className="cartao">
            {proximos.map((item) => (
              <LinhaCompromisso key={item.id} item={item} onConcluir={concluir} onEditar={onAbrirEditar} />
            ))}
          </div>
        </section>
      )}

      {semData.length > 0 && (
        <section className="secao">
          <h2>Sem data</h2>
          <div className="cartao">
            {semData.map((item) => (
              <LinhaCompromisso key={item.id} item={item} onConcluir={concluir} onEditar={onAbrirEditar} />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}

function LinhaCompromisso({
  item,
  destaque,
  onConcluir,
  onEditar,
}: {
  item: Item
  destaque?: boolean
  onConcluir: (id: number) => void
  onEditar: (item: Item) => void
}) {
  return (
    <div className="item-lista">
      <button
        className="marcar-circulo"
        aria-label={`concluir ${item.titulo}`}
        onClick={() => onConcluir(item.id)}
      >
        <IconeCheck />
      </button>
      <button className="linha-toque item-lista-corpo" onClick={() => onEditar(item)}>
        <div className={`item-lista-titulo ${destaque ? 'marcador-atraso' : ''}`}>{item.titulo}</div>
        <div className="item-lista-sub">
          {item.data && <span>{formatDataLonga(item.data)}</span>}
          {item.hora && <span>{item.hora}</span>}
          {!item.data && <span>sem data</span>}
        </div>
      </button>
    </div>
  )
}
