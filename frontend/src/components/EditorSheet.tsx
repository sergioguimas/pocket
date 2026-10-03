import { useState } from 'react'
import { api, ApiError, NetworkError } from '../api'
import type { Item, TipoItem, Regra, NovoItemPayload, EditarItemPayload } from '../api'
import { diaSemanaAbrev, descreverAntecedencia, DIAS_SEMANA_NOME } from '../format'
import { IconeFechar } from '../icons'

interface Props {
  tipoInicial: TipoItem
  item?: Item
  // captura (fase 1.5, §6.4): cria já preenchido a partir da proposta do
  // LLM — mesmo formato de payload do POST /api/itens. Nada mais muda no
  // editor: o usuário ainda revisa e toca em "salvar" como sempre.
  inicial?: NovoItemPayload
  // texto (ou transcrição) que originou o preenchimento, só para mostrar.
  vozTexto?: string
  onFechar: () => void
  onSalvo: (item: Item) => void
}

type TipoRegraUI = 'diario' | 'dias_semana' | 'vezes_periodo' | 'intervalo'

const ANTECEDENCIAS_PRESET = ['-15m', '-1h', '-1d']

function regraInicial(regra?: Regra): {
  tipo: TipoRegraUI
  dias: number[]
  vezes: number
  periodo: 'semana' | 'mes'
  intervaloDias: number
} {
  const r = regra
  if (!r) return { tipo: 'diario', dias: [], vezes: 3, periodo: 'semana', intervaloDias: 3 }
  if (r.tipo === 'dias_semana') return { tipo: 'dias_semana', dias: r.dias, vezes: 3, periodo: 'semana', intervaloDias: 3 }
  if (r.tipo === 'vezes_periodo') return { tipo: 'vezes_periodo', dias: [], vezes: r.vezes, periodo: r.periodo, intervaloDias: 3 }
  if (r.tipo === 'intervalo') return { tipo: 'intervalo', dias: [], vezes: 3, periodo: 'semana', intervaloDias: r.dias }
  return { tipo: 'diario', dias: [], vezes: 3, periodo: 'semana', intervaloDias: 3 }
}

export default function EditorSheet({ tipoInicial, item, inicial, vozTexto, onFechar, onSalvo }: Props) {
  const editando = !!item
  const [tipo] = useState<TipoItem>(item?.tipo ?? inicial?.tipo ?? tipoInicial)
  const [titulo, setTitulo] = useState(item?.titulo ?? inicial?.titulo ?? '')
  const [notas, setNotas] = useState(item?.notas ?? inicial?.notas ?? '')

  const rInicial = regraInicial(item?.regra ?? inicial?.regra)
  const [regraTipo, setRegraTipo] = useState<TipoRegraUI>(rInicial.tipo)
  const [dias, setDias] = useState<number[]>(rInicial.dias)
  const [vezes, setVezes] = useState(rInicial.vezes)
  const [periodo, setPeriodo] = useState<'semana' | 'mes'>(rInicial.periodo)
  const [intervaloDias, setIntervaloDias] = useState(rInicial.intervaloDias)

  const [data, setData] = useState(item?.data ?? inicial?.data ?? '')
  const [hora, setHora] = useState(item?.hora ?? inicial?.hora ?? '')

  const horariosIniciais = item?.horarios ?? inicial?.horarios ?? []
  const [horariosHabito, setHorariosHabito] = useState<string[]>(tipo === 'habito' ? horariosIniciais : [])
  const [antecedencias, setAntecedencias] = useState<string[]>(
    tipo === 'compromisso' ? horariosIniciais.filter((h) => h.startsWith('-')) : [],
  )
  const [horariosCompromissoSemHora, setHorariosCompromissoSemHora] = useState<string[]>(
    tipo === 'compromisso' ? horariosIniciais.filter((h) => !h.startsWith('-')) : [],
  )
  const [novoHorario, setNovoHorario] = useState('07:30')
  const [customNumero, setCustomNumero] = useState(30)
  const [customUnidade, setCustomUnidade] = useState<'m' | 'h' | 'd'>('m')

  const [salvando, setSalvando] = useState(false)
  const [erro, setErro] = useState<string | null>(null)

  function alternarDiaSemana(n: number) {
    setDias((atual) => (atual.includes(n) ? atual.filter((d) => d !== n) : [...atual, n].sort((a, b) => a - b)))
  }

  function alternarAntecedencia(a: string) {
    setAntecedencias((atual) => (atual.includes(a) ? atual.filter((x) => x !== a) : [...atual, a]))
  }

  function adicionarAntecedenciaCustom() {
    const a = `-${customNumero}${customUnidade}`
    if (!antecedencias.includes(a)) setAntecedencias((atual) => [...atual, a])
  }

  function adicionarHorarioHabito() {
    if (!horariosHabito.includes(novoHorario)) setHorariosHabito((atual) => [...atual, novoHorario].sort())
  }

  function removerHorarioHabito(h: string) {
    setHorariosHabito((atual) => atual.filter((x) => x !== h))
  }

  function adicionarHorarioCompromisso() {
    if (!horariosCompromissoSemHora.includes(novoHorario)) {
      setHorariosCompromissoSemHora((atual) => [...atual, novoHorario].sort())
    }
  }

  function removerHorarioCompromisso(h: string) {
    setHorariosCompromissoSemHora((atual) => atual.filter((x) => x !== h))
  }

  function montarRegra(): Regra | null {
    if (tipo !== 'habito') return null
    if (regraTipo === 'diario') return { tipo: 'diario' }
    if (regraTipo === 'dias_semana') return { tipo: 'dias_semana', dias }
    if (regraTipo === 'vezes_periodo') return { tipo: 'vezes_periodo', vezes, periodo }
    return { tipo: 'intervalo', dias: intervaloDias }
  }

  function montarHorarios(): string[] {
    if (tipo === 'habito') return horariosHabito
    if (hora) return antecedencias
    if (data) return horariosCompromissoSemHora
    return []
  }

  async function salvar() {
    setErro(null)
    const tituloAparado = titulo.trim()
    if (!tituloAparado) {
      setErro('título é obrigatório')
      return
    }
    if (tipo === 'habito' && regraTipo === 'dias_semana' && dias.length === 0) {
      setErro('escolha ao menos um dia da semana')
      return
    }
    if (tipo === 'compromisso' && hora && !data) {
      setErro('hora requer data')
      return
    }

    setSalvando(true)
    try {
      let salvo: Item
      if (editando && item) {
        const payload: EditarItemPayload = { titulo: tituloAparado, notas }
        if (tipo === 'habito') payload.regra = montarRegra()!
        if (tipo === 'compromisso') {
          payload.data = data
          payload.hora = hora
        }
        payload.horarios = montarHorarios()
        salvo = await api.editarItem(item.id, payload)
      } else {
        const payload: NovoItemPayload = { tipo, titulo: tituloAparado, notas, horarios: montarHorarios() }
        if (tipo === 'habito') payload.regra = montarRegra()!
        if (tipo === 'compromisso') {
          if (data) payload.data = data
          if (hora) payload.hora = hora
        }
        salvo = await api.criarItem(payload)
      }
      onSalvo(salvo)
    } catch (e) {
      setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
    } finally {
      setSalvando(false)
    }
  }

  return (
    <div className="folha-fundo" onClick={onFechar}>
      <div
        className="folha"
        role="dialog"
        aria-modal="true"
        aria-label={item ? 'Editar' : tipoInicial === 'compromisso' ? 'Novo compromisso' : 'Novo hábito'}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="folha-cabecalho">
          <h2>{editando ? 'Editar' : tipo === 'habito' ? 'Novo hábito' : 'Novo compromisso'}</h2>
          <button className="icone-botao" aria-label="fechar" onClick={onFechar}>
            <IconeFechar />
          </button>
        </div>

        {vozTexto && (
          <p className="captura-disse-editor">
            Você disse: <em>{vozTexto}</em>
          </p>
        )}

        {erro && <div className="estado-erro">{erro}</div>}

        <div className="campo">
          <label htmlFor="titulo">Título</label>
          <input
            id="titulo"
            type="text"
            value={titulo}
            maxLength={120}
            onChange={(e) => setTitulo(e.target.value)}
            placeholder={tipo === 'habito' ? 'ex.: ler' : 'ex.: dentista'}
          />
        </div>

        <div className="campo">
          <label htmlFor="notas">Notas (opcional)</label>
          <textarea id="notas" value={notas} onChange={(e) => setNotas(e.target.value)} />
        </div>

        {tipo === 'habito' && (
          <>
            <div className="campo">
              <label>Frequência</label>
              <div className="grupo-opcoes">
                <button
                  className="opcao-pill"
                  aria-pressed={regraTipo === 'diario'}
                  onClick={() => setRegraTipo('diario')}
                >
                  Todo dia
                </button>
                <button
                  className="opcao-pill"
                  aria-pressed={regraTipo === 'dias_semana'}
                  onClick={() => setRegraTipo('dias_semana')}
                >
                  Em dias da semana
                </button>
                <button
                  className="opcao-pill"
                  aria-pressed={regraTipo === 'vezes_periodo'}
                  onClick={() => setRegraTipo('vezes_periodo')}
                >
                  X vezes por período
                </button>
                <button
                  className="opcao-pill"
                  aria-pressed={regraTipo === 'intervalo'}
                  onClick={() => setRegraTipo('intervalo')}
                >
                  A cada N dias
                </button>
              </div>
            </div>

            {regraTipo === 'dias_semana' && (
              <div className="campo">
                <label>Quais dias</label>
                <div className="chips-dias-semana">
                  {[1, 2, 3, 4, 5, 6, 7].map((n) => (
                    <button
                      key={n}
                      className="chip-dia-semana"
                      aria-pressed={dias.includes(n)}
                      aria-label={DIAS_SEMANA_NOME[n - 1]}
                      onClick={() => alternarDiaSemana(n)}
                    >
                      {diaSemanaAbrev(n)}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {regraTipo === 'vezes_periodo' && (
              <div className="campo">
                <label>Quantas vezes</label>
                <div className="linha-numero-unidade">
                  <input
                    type="number"
                    min={1}
                    max={periodo === 'semana' ? 7 : 28}
                    value={vezes}
                    onChange={(e) => setVezes(Number(e.target.value))}
                  />
                  <span>vezes por</span>
                  <select value={periodo} onChange={(e) => setPeriodo(e.target.value as 'semana' | 'mes')}>
                    <option value="semana">semana</option>
                    <option value="mes">mês</option>
                  </select>
                </div>
              </div>
            )}

            {regraTipo === 'intervalo' && (
              <div className="campo">
                <label>A cada quantos dias</label>
                <div className="linha-numero-unidade">
                  <input
                    type="number"
                    min={2}
                    max={90}
                    value={intervaloDias}
                    onChange={(e) => setIntervaloDias(Number(e.target.value))}
                  />
                  <span>dias</span>
                </div>
              </div>
            )}

            <div className="campo">
              <label>Horários de lembrete</label>
              <div className="grupo-opcoes" style={{ marginBottom: 8 }}>
                {horariosHabito.map((h) => (
                  <button key={h} className="opcao-pill" aria-pressed="true" onClick={() => removerHorarioHabito(h)}>
                    {h} ✕
                  </button>
                ))}
              </div>
              <div className="linha-numero-unidade">
                <input type="time" value={novoHorario} onChange={(e) => setNovoHorario(e.target.value)} />
                <button className="botao" onClick={adicionarHorarioHabito}>
                  adicionar
                </button>
              </div>
            </div>
          </>
        )}

        {tipo === 'compromisso' && (
          <>
            <div className="campo">
              <label htmlFor="data">Data (opcional)</label>
              <input id="data" type="date" value={data} onChange={(e) => setData(e.target.value)} />
            </div>

            <div className="campo">
              <label htmlFor="hora">Hora (opcional, exige data)</label>
              <input
                id="hora"
                type="time"
                value={hora}
                disabled={!data}
                onChange={(e) => setHora(e.target.value)}
              />
            </div>

            {data && hora && (
              <div className="campo">
                <label>Antecedências (além do lembrete na hora)</label>
                <div className="grupo-opcoes" style={{ marginBottom: 8 }}>
                  {ANTECEDENCIAS_PRESET.map((a) => (
                    <button
                      key={a}
                      className="opcao-pill"
                      aria-pressed={antecedencias.includes(a)}
                      onClick={() => alternarAntecedencia(a)}
                    >
                      {descreverAntecedencia(a)}
                    </button>
                  ))}
                  {antecedencias
                    .filter((a) => !ANTECEDENCIAS_PRESET.includes(a))
                    .map((a) => (
                      <button key={a} className="opcao-pill" aria-pressed="true" onClick={() => alternarAntecedencia(a)}>
                        {descreverAntecedencia(a)} ✕
                      </button>
                    ))}
                </div>
                <div className="linha-numero-unidade">
                  <input
                    type="number"
                    min={1}
                    value={customNumero}
                    onChange={(e) => setCustomNumero(Number(e.target.value))}
                  />
                  <select value={customUnidade} onChange={(e) => setCustomUnidade(e.target.value as 'm' | 'h' | 'd')}>
                    <option value="m">minutos antes</option>
                    <option value="h">horas antes</option>
                    <option value="d">dias antes</option>
                  </select>
                  <button className="botao" onClick={adicionarAntecedenciaCustom}>
                    adicionar
                  </button>
                </div>
              </div>
            )}

            {data && !hora && (
              <div className="campo">
                <label>Horários de lembrete</label>
                <div className="grupo-opcoes" style={{ marginBottom: 8 }}>
                  {horariosCompromissoSemHora.map((h) => (
                    <button
                      key={h}
                      className="opcao-pill"
                      aria-pressed="true"
                      onClick={() => removerHorarioCompromisso(h)}
                    >
                      {h} ✕
                    </button>
                  ))}
                </div>
                <div className="linha-numero-unidade">
                  <input type="time" value={novoHorario} onChange={(e) => setNovoHorario(e.target.value)} />
                  <button className="botao" onClick={adicionarHorarioCompromisso}>
                    adicionar
                  </button>
                </div>
              </div>
            )}
          </>
        )}

        <div className="folha-rodape">
          <button className="botao" onClick={onFechar} disabled={salvando}>
            cancelar
          </button>
          <button className="botao primario" onClick={salvar} disabled={salvando}>
            {salvando ? 'salvando…' : 'salvar'}
          </button>
        </div>
      </div>
    </div>
  )
}
