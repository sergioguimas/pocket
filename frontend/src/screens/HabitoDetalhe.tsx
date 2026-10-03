import { useEffect, useState, useCallback } from 'react'
import { api, ApiError, NetworkError } from '../api'
import type { DetalheItemResponse, DiaCalendario, Item, Registro } from '../api'
import { descreverRegra, formatTaxa, hojeISO, descreverIntervalo } from '../format'
import { IconeVoltar } from '../icons'
import Heatmap from '../components/Heatmap'
import NotaSheet from '../components/NotaSheet'

interface Props {
  id: number
  onVoltar: () => void
  onEditar: (item: Item) => void
}

export default function HabitoDetalhe({ id, onVoltar, onEditar }: Props) {
  const [detalhe, setDetalhe] = useState<DetalheItemResponse | null>(null)
  const [dias, setDias] = useState<DiaCalendario[] | null>(null)
  const [erro, setErro] = useState<string | null>(null)
  const [carregando, setCarregando] = useState(true)
  const [acaoEmVoo, setAcaoEmVoo] = useState(false)
  const [anotando, setAnotando] = useState<Registro | null>(null)

  const carregar = useCallback(() => {
    setCarregando(true)
    setErro(null)
    Promise.all([api.detalheItem(id), api.calendario(id)])
      .then(([d, c]) => {
        setDetalhe(d)
        setDias(c)
      })
      .catch((e) => setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado'))
      .finally(() => setCarregando(false))
  }, [id])

  useEffect(carregar, [carregar])

  async function tocarDia(dia: DiaCalendario) {
    if (!detalhe) return
    if (dia.dia > hojeISO()) return

    const registroExistente = detalhe.registros.find((r) => r.em.slice(0, 10) === dia.dia)
    setAcaoEmVoo(true)
    try {
      if (registroExistente) {
        await api.desfazerRegistro(registroExistente.id)
      } else {
        await api.criarRegistro(id, { em: dia.dia })
      }
      carregar()
    } catch (e) {
      setErro(e instanceof Error ? e.message : 'erro inesperado')
    } finally {
      setAcaoEmVoo(false)
    }
  }

  function diaEstaFora30(dia: DiaCalendario): boolean {
    if (!detalhe) return false
    if (dia.registros === 0) return false
    return !detalhe.registros.some((r) => r.em.slice(0, 10) === dia.dia)
  }

  async function mudarEstado(estado: 'pausado' | 'ativo' | 'arquivado') {
    setAcaoEmVoo(true)
    try {
      await api.mudarEstado(id, estado)
      carregar()
    } catch (e) {
      setErro(e instanceof Error ? e.message : 'erro inesperado')
    } finally {
      setAcaoEmVoo(false)
    }
  }

  if (carregando && !detalhe) return <div className="estado-carregando">carregando…</div>

  if (erro && !detalhe) {
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

  if (!detalhe) return null

  const { item, registros } = detalhe
  const m = item.metricas

  return (
    <div>
      <button className="linha-toque" style={{ marginBottom: 12 }} onClick={onVoltar}>
        <IconeVoltar />
        <span style={{ marginLeft: 4 }}>voltar</span>
      </button>

      {erro && <div className="estado-erro">{erro}</div>}

      <h2 style={{ fontSize: 22, fontWeight: 700 }}>{item.titulo}</h2>
      {item.regra && <p style={{ color: 'var(--fg-mudo)', marginTop: 4 }}>{descreverRegra(item.regra)}</p>}
      {item.notas && <p style={{ color: 'var(--fg-mudo)', marginTop: 6 }}>{item.notas}</p>}

      {item.estado !== 'ativo' && (
        <div style={{ marginTop: 10 }}>
          <span className="chip">{item.estado === 'pausado' ? 'pausado' : 'arquivado'}</span>
        </div>
      )}

      {m && !m.intervalo && (
        <div className="metricas-grid">
          <div className="metrica-cartao">
            <div className="numero-medio">{m.sequencia_atual}</div>
            <div className="rotulo">sequência atual</div>
          </div>
          <div className="metrica-cartao">
            <div className="numero-medio">{m.melhor_sequencia}</div>
            <div className="rotulo">melhor sequência</div>
          </div>
          <div className="metrica-cartao">
            <div className="numero-medio">{formatTaxa(m.taxa_30d)}</div>
            <div className="rotulo">taxa 30 dias</div>
          </div>
        </div>
      )}

      {m?.intervalo && (
        <div style={{ marginTop: 10 }}>
          <span className={`chip ${m.intervalo.em_dia ? 'chip-ok' : 'chip-erro'}`}>
            {descreverIntervalo(m.intervalo)}
          </span>
        </div>
      )}

      <section className="secao">
        <h2>Últimas 12 semanas</h2>
        {dias && dias.length > 0 ? (
          <Heatmap dias={dias} onTocar={tocarDia} desabilitado={(d) => acaoEmVoo || d.dia > hojeISO() || diaEstaFora30(d)} />
        ) : (
          <p style={{ color: 'var(--fg-mudo)' }}>sem histórico ainda</p>
        )}
        <p style={{ color: 'var(--fg-fraco)', fontSize: 12, marginTop: 8 }}>
          toque num dia passado para marcar ou desmarcar
        </p>
      </section>

      {registros.length > 0 && (
        <section className="secao">
          <h2>Últimos registros</h2>
          <div className="cartao">
            {registros.slice(0, 8).map((r) => (
              <button key={r.id} className="linha-toque registro-linha" onClick={() => setAnotando(r)}>
                <span className="registro-quando">{quandoRegistro(r.em)}</span>
                {r.nota ? (
                  <span className="registro-nota">{r.nota}</span>
                ) : (
                  <span className="registro-nota vazia">+ anotar o que fez</span>
                )}
              </button>
            ))}
          </div>
        </section>
      )}

      {anotando && (
        <NotaSheet
          registroId={anotando.id}
          titulo={item.titulo}
          quando={quandoRegistro(anotando.em)}
          notaInicial={anotando.nota}
          onFechar={() => setAnotando(null)}
          onSalvo={() => {
            setAnotando(null)
            carregar()
          }}
        />
      )}

      <div className="botoes-linha">
        <button className="botao" disabled={acaoEmVoo} onClick={() => onEditar(item)}>
          editar
        </button>
        {item.estado === 'ativo' && (
          <button className="botao" disabled={acaoEmVoo} onClick={() => mudarEstado('pausado')}>
            pausar
          </button>
        )}
        {item.estado === 'pausado' && (
          <button className="botao" disabled={acaoEmVoo} onClick={() => mudarEstado('ativo')}>
            retomar
          </button>
        )}
        {item.estado !== 'arquivado' && (
          <button className="botao perigo" disabled={acaoEmVoo} onClick={() => mudarEstado('arquivado')}>
            arquivar
          </button>
        )}
        {item.estado === 'arquivado' && (
          <button className="botao" disabled={acaoEmVoo} onClick={() => mudarEstado('ativo')}>
            reativar
          </button>
        )}
      </div>
    </div>
  )
}

// "hoje 07:40", "ontem", "25/09" — a hora só importa no dia corrente.
function quandoRegistro(em: string): string {
  const d = new Date(em)
  const dia = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
  const diff = Math.round((dia(new Date()) - dia(d)) / 86400000)
  if (diff === 0) return `hoje ${d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })}`
  if (diff === 1) return 'ontem'
  return d.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit' })
}
