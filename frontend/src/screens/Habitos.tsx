import { useEffect, useState, useCallback } from 'react'
import { api, ApiError, NetworkError } from '../api'
import type { Item } from '../api'
import { descreverRegra, formatTaxa, descreverIntervalo } from '../format'
import { IconeSeta } from '../icons'

interface Props {
  onAbrirDetalhe: (id: number) => void
}

export default function Habitos({ onAbrirDetalhe }: Props) {
  const [itens, setItens] = useState<Item[] | null>(null)
  const [erro, setErro] = useState<string | null>(null)
  const [carregando, setCarregando] = useState(true)

  const carregar = useCallback(() => {
    setCarregando(true)
    setErro(null)
    api
      .listarItens({ tipo: 'habito', estado: 'ativo' })
      .then(setItens)
      .catch((e) => setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado'))
      .finally(() => setCarregando(false))
  }, [])

  useEffect(carregar, [carregar])

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
        <div className="titulo">nenhum hábito ainda</div>
        <p>toque em + para criar o primeiro</p>
      </div>
    )
  }

  return (
    <div className="secao">
      <div className="cartao">
        {itens.map((item) => (
          <button key={item.id} className="linha-toque item-lista" onClick={() => onAbrirDetalhe(item.id)}>
            <div className="item-lista-corpo">
              <div className="item-lista-titulo">{item.titulo}</div>
              <div className="item-lista-sub">
                {item.regra && <span>{descreverRegra(item.regra)}</span>}
                {item.metricas && item.regra?.tipo === 'intervalo' && item.metricas.intervalo && (
                  <span>· {descreverIntervalo(item.metricas.intervalo)}</span>
                )}
                {item.metricas && item.regra?.tipo !== 'intervalo' && (
                  <>
                    <span>· sequência {item.metricas.sequencia_atual}</span>
                    <span>
                      · {item.metricas.taxa_30d === null ? 'sem taxa ainda' : `taxa ${formatTaxa(item.metricas.taxa_30d)}`}
                    </span>
                  </>
                )}
              </div>
            </div>
            <IconeSeta className="item-lista-seta" />
          </button>
        ))}
      </div>
    </div>
  )
}
