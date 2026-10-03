import { useEffect, useState, useCallback } from 'react'
import { api, ApiError, NetworkError } from '../api'
import type { SaudeResponse } from '../api'
import { permissao, inscricaoAtiva, pedirPermissaoEInscrever } from '../push'
import { IconeVoltar } from '../icons'

interface Props {
  onVoltar: () => void
}

export default function Ajustes({ onVoltar }: Props) {
  const [saude, setSaude] = useState<SaudeResponse | null>(null)
  const [erroSaude, setErroSaude] = useState<string | null>(null)
  const [perm, setPerm] = useState(permissao())
  const [inscrito, setInscrito] = useState(false)
  const [processandoAtivar, setProcessandoAtivar] = useState(false)
  const [resultadoTeste, setResultadoTeste] = useState<string | null>(null)
  const [processandoTeste, setProcessandoTeste] = useState(false)

  const carregarSaude = useCallback(() => {
    api
      .saude()
      .then(setSaude)
      .catch((e) => setErroSaude(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado'))
  }, [])

  useEffect(() => {
    carregarSaude()
    inscricaoAtiva().then(setInscrito)
  }, [carregarSaude])

  async function ativarNotificacoes() {
    setProcessandoAtivar(true)
    const ok = await pedirPermissaoEInscrever()
    setPerm(permissao())
    setInscrito(await inscricaoAtiva())
    setProcessandoAtivar(false)
    if (!ok) setResultadoTeste('não foi possível ativar — permissão negada ou navegador sem suporte')
  }

  async function testarPush() {
    setProcessandoTeste(true)
    setResultadoTeste(null)
    try {
      const r = await api.pushTeste()
      setResultadoTeste(r.enviados > 0 ? `push de teste enviado (${r.enviados})` : 'nenhuma inscrição recebeu o teste')
    } catch (e) {
      setResultadoTeste(e instanceof Error ? e.message : 'erro inesperado')
    } finally {
      setProcessandoTeste(false)
    }
  }

  function validadeCert(): string {
    if (!saude?.cert_expira_em) return '—'
    const d = new Date(saude.cert_expira_em)
    const dias = Math.round((d.getTime() - Date.now()) / 86400000)
    return `${d.toLocaleDateString('pt-BR')} (${dias >= 0 ? `em ${dias} dias` : 'vencido'})`
  }

  return (
    <div>
      <button className="linha-toque" style={{ marginBottom: 12 }} onClick={onVoltar}>
        <IconeVoltar />
        <span style={{ marginLeft: 4 }}>voltar</span>
      </button>

      <section className="secao">
        <h2>Notificações</h2>
        <div className="cartao">
          <div className="linha-ajuste">
            <span className="rotulo">permissão</span>
            <span className="valor">
              {perm === 'granted' ? 'concedida' : perm === 'denied' ? 'negada' : perm === 'sem-suporte' ? 'sem suporte' : 'não decidida'}
            </span>
          </div>
          <div className="linha-ajuste">
            <span className="rotulo">inscrito neste aparelho</span>
            <span className="valor">{inscrito ? 'sim' : 'não'}</span>
          </div>
        </div>

        {perm !== 'granted' && (
          <div style={{ marginTop: 12 }}>
            <button className="botao primario botao-largo" disabled={processandoAtivar} onClick={ativarNotificacoes}>
              {processandoAtivar ? 'ativando…' : 'Ativar notificações'}
            </button>
          </div>
        )}

        {perm === 'granted' && (
          <div style={{ marginTop: 12 }}>
            <button className="botao botao-largo" disabled={processandoTeste} onClick={testarPush}>
              {processandoTeste ? 'enviando…' : 'Testar notificação'}
            </button>
          </div>
        )}

        {resultadoTeste && <p style={{ marginTop: 8, color: 'var(--fg-mudo)', fontSize: 13 }}>{resultadoTeste}</p>}
      </section>

      <section className="secao">
        <h2>Servidor</h2>
        {erroSaude && <div className="estado-erro">{erroSaude}</div>}
        {saude && (
          <div className="cartao">
            <div className="linha-ajuste">
              <span className="rotulo">status</span>
              <span className={`valor ${saude.ok ? '' : 'marcador-atraso'}`}>{saude.ok ? 'ok' : 'atenção'}</span>
            </div>
            <div className="linha-ajuste">
              <span className="rotulo">certificado válido até</span>
              <span className="valor">{validadeCert()}</span>
            </div>
            <div className="linha-ajuste">
              <span className="rotulo">inscrições ativas</span>
              <span className="valor">{saude.subs}</span>
            </div>
            <div className="linha-ajuste">
              <span className="rotulo">versão</span>
              <span className="valor">{saude.versao}</span>
            </div>
          </div>
        )}
      </section>
    </div>
  )
}
