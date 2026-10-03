// Ajustes da lista (ícone no topo da aba Compras, CONTRATO-FASE3 §6.3):
// renomear, lembrete (Todo dia / Em dias da semana + horários — mesmos
// seletores do EditorSheet), seções (renomear, reordenar ↑↓, criar,
// apagar) e arquivar. Exige rede (F2: ajustes ficam fora da fila offline).
import { useState } from 'react'
import { api, ApiError, NetworkError } from '../api'
import type { Lista, Secao, Regra } from '../api'
import { diaSemanaAbrev, DIAS_SEMANA_NOME } from '../format'
import { IconeFechar, IconeSetaCima, IconeSetaBaixo } from '../icons'

interface Props {
  lista: Lista
  secoes: Secao[]
  online: boolean
  onFechar: () => void
  onAtualizada: () => void
  onArquivada: () => void
}

type TipoRegraLembreteUI = 'diario' | 'dias_semana'

export default function AjustesListaSheet({ lista, secoes, online, onFechar, onAtualizada, onArquivada }: Props) {
  const [nome, setNome] = useState(lista.nome)
  const [salvandoNome, setSalvandoNome] = useState(false)

  const [temLembrete, setTemLembrete] = useState(!!lista.lembrete)
  const [regraTipo, setRegraTipo] = useState<TipoRegraLembreteUI>(
    lista.lembrete?.regra.tipo === 'dias_semana' ? 'dias_semana' : 'diario',
  )
  const [dias, setDias] = useState<number[]>(
    lista.lembrete?.regra.tipo === 'dias_semana' ? lista.lembrete.regra.dias : [],
  )
  const [horarios, setHorarios] = useState<string[]>(lista.lembrete?.horarios ?? [])
  const [novoHorario, setNovoHorario] = useState('09:00')
  const [salvandoLembrete, setSalvandoLembrete] = useState(false)

  const [secoesLocais, setSecoesLocais] = useState(secoes)
  const [novaSecao, setNovaSecao] = useState('')
  const [editandoSecaoId, setEditandoSecaoId] = useState<number | null>(null)
  const [nomeSecaoEdicao, setNomeSecaoEdicao] = useState('')

  const [erro, setErro] = useState<string | null>(null)
  const [arquivando, setArquivando] = useState(false)

  function tratarErro(e: unknown) {
    setErro(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
  }

  async function salvarNome() {
    const aparado = nome.trim()
    if (!aparado || aparado === lista.nome) return
    setSalvandoNome(true)
    setErro(null)
    try {
      await api.editarLista(lista.id, { nome: aparado })
      onAtualizada()
    } catch (e) {
      tratarErro(e)
    } finally {
      setSalvandoNome(false)
    }
  }

  function alternarDia(n: number) {
    setDias((atual) => (atual.includes(n) ? atual.filter((d) => d !== n) : [...atual, n].sort((a, b) => a - b)))
  }

  function adicionarHorario() {
    if (!horarios.includes(novoHorario)) setHorarios((atual) => [...atual, novoHorario].sort())
  }

  function removerHorario(h: string) {
    setHorarios((atual) => atual.filter((x) => x !== h))
  }

  async function salvarLembrete() {
    setErro(null)
    if (temLembrete) {
      if (regraTipo === 'dias_semana' && dias.length === 0) {
        setErro('escolha ao menos um dia da semana')
        return
      }
      if (horarios.length === 0) {
        setErro('escolha ao menos um horário')
        return
      }
    }
    const regra: Regra | null = temLembrete
      ? regraTipo === 'diario'
        ? { tipo: 'diario' }
        : { tipo: 'dias_semana', dias }
      : null
    setSalvandoLembrete(true)
    try {
      await api.editarLista(lista.id, {
        lembrete: temLembrete && regra ? { regra, horarios } : null,
      })
      onAtualizada()
    } catch (e) {
      tratarErro(e)
    } finally {
      setSalvandoLembrete(false)
    }
  }

  async function criarSecao() {
    const aparado = novaSecao.trim()
    if (!aparado) return
    setErro(null)
    try {
      const sc = await api.criarSecao(lista.id, { nome: aparado })
      setSecoesLocais((atual) => [...atual, sc])
      setNovaSecao('')
      onAtualizada()
    } catch (e) {
      tratarErro(e)
    }
  }

  async function renomearSecao(id: number) {
    const aparado = nomeSecaoEdicao.trim()
    if (!aparado) {
      setEditandoSecaoId(null)
      return
    }
    setErro(null)
    try {
      const sc = await api.editarSecao(id, { nome: aparado })
      setSecoesLocais((atual) => atual.map((s) => (s.id === id ? sc : s)))
      setEditandoSecaoId(null)
      onAtualizada()
    } catch (e) {
      tratarErro(e)
    }
  }

  async function moverSecao(id: number, direcao: -1 | 1) {
    const ordenadas = [...secoesLocais].sort((a, b) => a.ordem - b.ordem)
    const i = ordenadas.findIndex((s) => s.id === id)
    const j = i + direcao
    if (i < 0 || j < 0 || j >= ordenadas.length) return
    const a = ordenadas[i]
    const b = ordenadas[j]
    setErro(null)
    try {
      const [scA, scB] = await Promise.all([
        api.editarSecao(a.id, { ordem: b.ordem }),
        api.editarSecao(b.id, { ordem: a.ordem }),
      ])
      setSecoesLocais((atual) => atual.map((s) => (s.id === scA.id ? scA : s.id === scB.id ? scB : s)))
      onAtualizada()
    } catch (e) {
      tratarErro(e)
    }
  }

  async function apagarSecao(id: number) {
    setErro(null)
    try {
      await api.apagarSecao(id)
      setSecoesLocais((atual) => atual.filter((s) => s.id !== id))
      onAtualizada()
    } catch (e) {
      tratarErro(e)
    }
  }

  async function arquivar() {
    setArquivando(true)
    setErro(null)
    try {
      await api.mudarEstadoLista(lista.id, true)
      onArquivada()
    } catch (e) {
      tratarErro(e)
    } finally {
      setArquivando(false)
    }
  }

  const secoesOrdenadas = [...secoesLocais].sort((a, b) => a.ordem - b.ordem)

  return (
    <div className="folha-fundo" onClick={onFechar}>
      <div className="folha" role="dialog" aria-modal="true" aria-label={`Ajustes da lista ${lista.nome}`} onClick={(e) => e.stopPropagation()}>
        <div className="folha-cabecalho">
          <h2>Ajustes — {lista.nome}</h2>
          <button className="icone-botao" aria-label="fechar" onClick={onFechar}>
            <IconeFechar />
          </button>
        </div>

        {!online && <div className="banner-offline">sem conexão — ajustes exigem rede</div>}
        {erro && <div className="estado-erro">{erro}</div>}

        <fieldset disabled={!online} style={{ border: 'none', padding: 0, margin: 0 }}>
          <div className="campo">
            <label htmlFor="nome-lista">Nome</label>
            <div className="linha-numero-unidade">
              <input id="nome-lista" type="text" value={nome} maxLength={40} onChange={(e) => setNome(e.target.value)} style={{ flex: 1 }} />
              <button className="botao" disabled={salvandoNome} onClick={salvarNome}>
                {salvandoNome ? 'salvando…' : 'salvar'}
              </button>
            </div>
          </div>

          <div className="campo">
            <label>Lembrete</label>
            <div className="grupo-opcoes" style={{ marginBottom: 8 }}>
              <button className="opcao-pill" aria-pressed={!temLembrete} onClick={() => setTemLembrete(false)}>
                sem lembrete
              </button>
              <button className="opcao-pill" aria-pressed={temLembrete} onClick={() => setTemLembrete(true)}>
                com lembrete
              </button>
            </div>

            {temLembrete && (
              <>
                <div className="grupo-opcoes" style={{ marginBottom: 8 }}>
                  <button className="opcao-pill" aria-pressed={regraTipo === 'diario'} onClick={() => setRegraTipo('diario')}>
                    Todo dia
                  </button>
                  <button className="opcao-pill" aria-pressed={regraTipo === 'dias_semana'} onClick={() => setRegraTipo('dias_semana')}>
                    Em dias da semana
                  </button>
                </div>

                {regraTipo === 'dias_semana' && (
                  <div className="chips-dias-semana" style={{ marginBottom: 8 }}>
                    {[1, 2, 3, 4, 5, 6, 7].map((n) => (
                      <button
                        key={n}
                        className="chip-dia-semana"
                        aria-pressed={dias.includes(n)}
                        aria-label={DIAS_SEMANA_NOME[n - 1]}
                        onClick={() => alternarDia(n)}
                      >
                        {diaSemanaAbrev(n)}
                      </button>
                    ))}
                  </div>
                )}

                <div className="grupo-opcoes" style={{ marginBottom: 8 }}>
                  {horarios.map((h) => (
                    <button key={h} className="opcao-pill" aria-pressed="true" onClick={() => removerHorario(h)}>
                      {h} ✕
                    </button>
                  ))}
                </div>
                <div className="linha-numero-unidade">
                  <input type="time" value={novoHorario} onChange={(e) => setNovoHorario(e.target.value)} />
                  <button className="botao" onClick={adicionarHorario}>
                    adicionar horário
                  </button>
                </div>
              </>
            )}

            <div style={{ marginTop: 10 }}>
              <button className="botao primario" disabled={salvandoLembrete} onClick={salvarLembrete}>
                {salvandoLembrete ? 'salvando…' : 'salvar lembrete'}
              </button>
            </div>
          </div>

          <div className="campo">
            <label>Seções</label>
            <div className="lista">
              {secoesOrdenadas.map((s, i) => (
                <div key={s.id} className="item-lista">
                  {editandoSecaoId === s.id ? (
                    <input
                      type="text"
                      value={nomeSecaoEdicao}
                      autoFocus
                      maxLength={40}
                      onChange={(e) => setNomeSecaoEdicao(e.target.value)}
                      onBlur={() => renomearSecao(s.id)}
                      onKeyDown={(e) => e.key === 'Enter' && renomearSecao(s.id)}
                      style={{ flex: 1 }}
                    />
                  ) : (
                    <button
                      className="linha-toque item-lista-corpo"
                      onClick={() => {
                        setEditandoSecaoId(s.id)
                        setNomeSecaoEdicao(s.nome)
                      }}
                    >
                      <div className="item-lista-titulo">{s.nome}</div>
                    </button>
                  )}
                  <button className="icone-botao" aria-label={`mover ${s.nome} para cima`} disabled={i === 0} onClick={() => moverSecao(s.id, -1)}>
                    <IconeSetaCima />
                  </button>
                  <button
                    className="icone-botao"
                    aria-label={`mover ${s.nome} para baixo`}
                    disabled={i === secoesOrdenadas.length - 1}
                    onClick={() => moverSecao(s.id, 1)}
                  >
                    <IconeSetaBaixo />
                  </button>
                  <button className="icone-botao" aria-label={`apagar seção ${s.nome}`} onClick={() => apagarSecao(s.id)}>
                    <IconeFechar />
                  </button>
                </div>
              ))}
            </div>
            <div className="linha-numero-unidade" style={{ marginTop: 8 }}>
              <input
                type="text"
                placeholder="nova seção"
                value={novaSecao}
                maxLength={40}
                onChange={(e) => setNovaSecao(e.target.value)}
                style={{ flex: 1 }}
              />
              <button className="botao" onClick={criarSecao}>
                criar
              </button>
            </div>
          </div>

          <div className="campo">
            <button className="botao perigo botao-largo" disabled={arquivando} onClick={arquivar}>
              {arquivando ? 'arquivando…' : 'Arquivar lista'}
            </button>
          </div>
        </fieldset>
      </div>
    </div>
  )
}
