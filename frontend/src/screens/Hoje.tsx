import { useEffect, useState, useCallback } from 'react'
import { api, ApiError, NetworkError } from '../api'
import type { HojeResponse, HojeHabito, HojeCompromisso, NovoItemPayload } from '../api'
import { IconeCheck } from '../icons'
import { permissao } from '../push'
import NotaSheet from '../components/NotaSheet'
import ConviteNotificacao from '../components/ConviteNotificacao'
import CapturaBarra from '../components/CapturaBarra'

interface Props {
  online: boolean
  onAbrirCriarHabito: () => void
  onIrParaCompromissos: () => void
  onIrParaCompras: (listaId: number) => void
  // captura (fase 1.5, §6): App.tsx é dono do EditorSheet e do toast.
  onAbrirCapturaEditor: (inicial: NovoItemPayload, vozTexto: string, capturaId: number) => void
  onToast: (mensagem: string) => void
  autoIniciarGravacao: boolean
  onAutoIniciarConsumido: () => void
  capturaResto: string | null
  onCapturaResto: (resto: string | null) => void
}

interface Desfazer {
  registroId: number
  titulo: string
}

export default function Hoje({
  online,
  onAbrirCriarHabito,
  onIrParaCompromissos,
  onIrParaCompras,
  onAbrirCapturaEditor,
  onToast,
  autoIniciarGravacao,
  onAutoIniciarConsumido,
  capturaResto,
  onCapturaResto,
}: Props) {
  const [dados, setDados] = useState<HojeResponse | null>(null)
  const [erro, setErro] = useState<string | null>(null)
  const [carregando, setCarregando] = useState(true)
  const [desfazer, setDesfazer] = useState<Desfazer | null>(null)
  const [anotando, setAnotando] = useState<Desfazer | null>(null)
  const [mostrarConvite, setMostrarConvite] = useState(false)

  const carregar = useCallback(() => {
    setCarregando(true)
    setErro(null)
    api
      .hoje()
      .then(setDados)
      .catch((e) => {
        if (e instanceof ApiError || e instanceof NetworkError) setErro(e.message)
        else setErro('erro inesperado')
      })
      .finally(() => setCarregando(false))
  }, [])

  useEffect(() => {
    carregar()
    setMostrarConvite(permissao() === 'default')
  }, [carregar])

  // a conexão com o redmilab voltou (conexao.ts) e a tela ficou no erro de
  // rede: recarrega sozinha, sem exigir "tentar de novo".
  useEffect(() => {
    if (online && erro) carregar()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [online])

  useEffect(() => {
    if (!desfazer) return
    if (anotando) return // folha aberta: a barra espera
    const t = setTimeout(() => setDesfazer(null), 8000)
    return () => clearTimeout(t)
  }, [desfazer, anotando])

  async function marcarFeito(itemId: number, titulo: string) {
    try {
      const resp = await api.criarRegistro(itemId)
      setDesfazer({ registroId: resp.registro.id, titulo })
      carregar()
    } catch (e) {
      setErro(e instanceof Error ? e.message : 'erro inesperado')
    }
  }

  async function desfazerRegistro() {
    if (!desfazer) return
    try {
      await api.desfazerRegistro(desfazer.registroId)
    } catch {
      // registro já pode ter sido removido — sem problema, só recarrega
    } finally {
      setDesfazer(null)
      carregar()
    }
  }

  async function concluirCompromisso(itemId: number) {
    try {
      await api.mudarEstado(itemId, 'concluido')
      carregar()
    } catch (e) {
      setErro(e instanceof Error ? e.message : 'erro inesperado')
    }
  }

  if (carregando && !dados) {
    return <div className="estado-carregando">carregando…</div>
  }

  if (erro && !dados) {
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

  if (!dados) return null

  const semNada =
    dados.habitos.length === 0 &&
    dados.compromissos.length === 0 &&
    dados.sem_data === 0 &&
    dados.compras.length === 0

  return (
    <div>
      <CapturaBarra
        online={online}
        onAbrirCriarItem={onAbrirCapturaEditor}
        onToast={onToast}
        onRecarregar={carregar}
        autoIniciarGravacao={autoIniciarGravacao}
        onAutoIniciarConsumido={onAutoIniciarConsumido}
        resto={capturaResto}
        onResto={onCapturaResto}
      />

      {erro && <div className="estado-erro">{erro}</div>}

      {mostrarConvite && <ConviteNotificacao onFechar={() => setMostrarConvite(false)} />}

      {semNada ? (
        <div className="estado-vazio">
          <div className="titulo">nada para hoje</div>
          <p>que tal criar um hábito?</p>
          <div style={{ marginTop: 14 }}>
            <button className="botao primario" onClick={onAbrirCriarHabito}>
              criar hábito
            </button>
          </div>
        </div>
      ) : (
        <>
          {dados.habitos.length > 0 && (
            <section className="secao">
              <h2>Hábitos de hoje</h2>
              <div className="cartao">
                {dados.habitos.map((h) => (
                  <HabitoHoje key={h.item.id} entrada={h} onMarcar={marcarFeito} />
                ))}
              </div>
            </section>
          )}

          {dados.compromissos.length > 0 && (
            <section className="secao">
              <h2>Compromissos</h2>
              <div className="cartao">
                {dados.compromissos.map((c) => (
                  <CompromissoHoje key={c.item.id} entrada={c} onConcluir={concluirCompromisso} />
                ))}
              </div>
            </section>
          )}

          {dados.sem_data > 0 && (
            <section className="secao">
              <button className="linha-toque cartao" onClick={onIrParaCompromissos}>
                <span style={{ flex: 1 }}>Sem data</span>
                <span className="chip chip-ouro">{dados.sem_data}</span>
              </button>
            </section>
          )}

          {dados.compras.length > 0 && (
            <section className="secao">
              <h2>Compras</h2>
              <div className="cartao">
                {dados.compras.map((c) => (
                  <button
                    key={c.lista_id}
                    className="linha-toque item-lista"
                    onClick={() => onIrParaCompras(c.lista_id)}
                  >
                    <span style={{ flex: 1 }}>
                      {c.nome} · {c.pendentes} {c.pendentes === 1 ? 'item' : 'itens'}
                    </span>
                  </button>
                ))}
              </div>
            </section>
          )}
        </>
      )}

      {anotando && (
        <NotaSheet
          registroId={anotando.registroId}
          titulo={anotando.titulo}
          quando="hoje"
          onFechar={() => setAnotando(null)}
          onSalvo={() => {
            setAnotando(null)
            setDesfazer(null)
          }}
        />
      )}

      {desfazer && !anotando && (
        <div className="desfazer-barra">
          <span>"{desfazer.titulo}" feito</span>
          <span style={{ display: 'flex' }}>
            <button onClick={() => setAnotando(desfazer)}>Anotar</button>
            <button onClick={desfazerRegistro}>Desfazer</button>
          </span>
        </div>
      )}
    </div>
  )
}

function HabitoHoje({
  entrada,
  onMarcar,
}: {
  entrada: HojeHabito
  onMarcar: (id: number, titulo: string) => void
}) {
  const { item, ocorrencia, intervalo } = entrada
  const cumprida = ocorrencia.status === 'cumprida'
  const perdida = ocorrencia.status === 'perdida'
  const pulada = ocorrencia.status === 'pulada'

  const regra = item.regra
  let chipPeriodo: string | null = null
  if (regra?.tipo === 'vezes_periodo' && ocorrencia.feitos !== undefined) {
    const periodoTexto = regra.periodo === 'semana' ? 'esta semana' : 'este mês'
    chipPeriodo = `${ocorrencia.feitos}/${ocorrencia.meta} ${periodoTexto}`
  }

  let chipIntervalo: string | null = null
  if (intervalo && !intervalo.em_dia) {
    chipIntervalo = intervalo.dias === 0 ? 'vence hoje' : `atrasado ${intervalo.dias} dia${intervalo.dias === 1 ? '' : 's'}`
  }

  return (
    <div className="item-hoje">
      <button
        className={`marcar-circulo ${cumprida ? 'feito' : ''} ${perdida ? 'perdida' : ''}`}
        disabled={cumprida}
        aria-label={cumprida ? 'já feito' : `marcar ${item.titulo} como feito`}
        onClick={() => onMarcar(item.id, item.titulo)}
      >
        {cumprida && <IconeCheck />}
      </button>
      <div className="item-hoje-corpo">
        <div className={`item-hoje-titulo ${cumprida ? 'riscado' : ''}`}>{item.titulo}</div>
        <div className="item-hoje-meta">
          {chipPeriodo && <span className="chip">{chipPeriodo}</span>}
          {chipIntervalo && <span className="chip chip-erro">{chipIntervalo}</span>}
          {pulada && <span className="chip">pulado</span>}
        </div>
      </div>
    </div>
  )
}

function CompromissoHoje({
  entrada,
  onConcluir,
}: {
  entrada: HojeCompromisso
  onConcluir: (id: number) => void
}) {
  const { item, atrasado } = entrada
  const concluido = item.estado === 'concluido'
  const hora = item.hora || null

  return (
    <div className="item-hoje">
      <button
        className={`marcar-circulo ${concluido ? 'feito' : ''}`}
        disabled={concluido}
        aria-label={concluido ? 'concluído' : `concluir ${item.titulo}`}
        onClick={() => onConcluir(item.id)}
      >
        {concluido && <IconeCheck />}
      </button>
      <div className="item-hoje-corpo">
        <div className={`item-hoje-titulo ${concluido ? 'riscado' : ''}`}>{item.titulo}</div>
        <div className="item-hoje-meta">
          {hora && <span className="chip">{hora}</span>}
          {atrasado && !concluido && <span className="chip chip-erro">atrasado</span>}
        </div>
      </div>
    </div>
  )
}
