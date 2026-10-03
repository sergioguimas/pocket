// Barra de captura rápida por voz/texto (CONTRATO-FASE15.md §6.1) — fica no
// topo de Hoje. Some se `GET /api/captura` disser `ativa: false` (chave do
// Groq vazia); fica visível mas desabilitada sem rede (a captura não tem
// fila offline, diferente das Compras).
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError, CanceladoError, NetworkError } from '../api'
import type { CapturaResposta, NovoItemPayload } from '../api'
import { useGravador } from '../hooks/useGravador'
import { IconeEnviar, IconeMicrofone, IconeParar } from '../icons'
import CapturaSheet from './CapturaSheet'

interface Props {
  online: boolean
  // criar_item: quem monta a tela (App.tsx) é dono do EditorSheet.
  onAbrirCriarItem: (inicial: NovoItemPayload, vozTexto: string, capturaId: number) => void
  onToast: (mensagem: string) => void
  onRecarregar: () => void
  // V6/V11: veio de "Falar com o Pocket" (?captura=voz) — tenta gravar já.
  autoIniciarGravacao: boolean
  onAutoIniciarConsumido: () => void
  // "Tem mais" (V1) mora no App: salvar no EditorSheet recria Hoje (key), e
  // o resto não pode sumir junto.
  resto: string | null
  onResto: (resto: string | null) => void
}

type EstadoEnvio = 'ocioso' | 'entendendo' | 'erro'

export default function CapturaBarra({
  online,
  onAbrirCriarItem,
  onToast,
  onRecarregar,
  autoIniciarGravacao,
  onAutoIniciarConsumido,
  resto: restoPendente,
  onResto: setRestoPendente,
}: Props) {
  const [ativa, setAtiva] = useState<boolean | null>(null)
  const [texto, setTexto] = useState('')
  const [estadoEnvio, setEstadoEnvio] = useState<EstadoEnvio>('ocioso')
  const [erroEnvio, setErroEnvio] = useState<string | null>(null)
  // erro de rede (V10): sem conexão, "salvar como tarefa"/"abrir formulário"
  // também falhariam — a barra oferece só "tentar de novo".
  const [erroDeRede, setErroDeRede] = useState(false)
  const envioRef = useRef<AbortController | null>(null)
  // áudio da última tentativa: se a rede cair, reenvia sem regravar.
  const ultimoAudioRef = useRef<Blob | null>(null)
  const [resultado, setResultado] = useState<CapturaResposta | null>(null)
  const [destaqueMic, setDestaqueMic] = useState(false)

  const autoTentadoRef = useRef(false)

  useEffect(() => {
    api
      .capturaEstado()
      .then((r) => setAtiva(r.ativa))
      .catch(() => setAtiva((atual) => atual ?? true)) // sem rede: mantém visível (desabilitada), não some
  }, [])

  function tratarResultado(resp: CapturaResposta) {
    setEstadoEnvio('ocioso')
    setErroEnvio(null)
    setErroDeRede(false)
    ultimoAudioRef.current = null
    setTexto('')
    if (resp.resto) setRestoPendente(resp.resto)
    if (resp.acao === 'criar_item' && resp.item) {
      onAbrirCriarItem(resp.item, resp.texto, resp.captura_id)
      return
    }
    setResultado(resp)
  }

  function tratarErroEnvio(e: unknown) {
    if (e instanceof CanceladoError) {
      setEstadoEnvio('ocioso')
      return
    }
    setErroDeRede(e instanceof NetworkError)
    setEstadoEnvio('erro')
    if (e instanceof ApiError || e instanceof NetworkError) setErroEnvio(e.message)
    else setErroEnvio('erro inesperado')
    // a fala foi transcrita, só a interpretação falhou: o texto vai para o
    // campo — dá para reenviar (sem regravar) ou salvar como tarefa.
    if (e instanceof ApiError && e.texto) setTexto(e.texto)
  }

  // enviarAudio é estável (o gravador guarda a referência da primeira
  // renderização); os tratadores mudam a cada render — lidos por ref para
  // não usar props/estado velhos.
  const tratadoresRef = useRef({ tratarResultado, tratarErroEnvio })
  tratadoresRef.current = { tratarResultado, tratarErroEnvio }

  function novoEnvio(): AbortSignal {
    envioRef.current?.abort()
    envioRef.current = new AbortController()
    return envioRef.current.signal
  }

  async function enviarTexto(t: string) {
    const aparado = t.trim()
    if (!aparado) return
    ultimoAudioRef.current = null
    setEstadoEnvio('entendendo')
    setErroEnvio(null)
    try {
      tratarResultado(await api.capturarTexto(aparado, novoEnvio()))
    } catch (e) {
      tratarErroEnvio(e)
    }
  }

  const enviarAudio = useCallback(async (blob: Blob) => {
    ultimoAudioRef.current = blob
    setEstadoEnvio('entendendo')
    setErroEnvio(null)
    envioRef.current?.abort()
    envioRef.current = new AbortController()
    try {
      tratadoresRef.current.tratarResultado(await api.capturarAudio(blob, envioRef.current.signal))
    } catch (e) {
      tratadoresRef.current.tratarErroEnvio(e)
    } finally {
      gravador.finalizarProcessamento()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const gravador = useGravador(enviarAudio)

  // V11: chegou por "Falar com o Pocket" — tenta gravar sem esperar toque;
  // se o Chrome recusar (sem gesto do usuário ainda), destaca o botão em
  // vez de mostrar o erro de permissão (que só faz sentido num toque real).
  useEffect(() => {
    if (!autoIniciarGravacao || autoTentadoRef.current || ativa !== true || !online) return
    autoTentadoRef.current = true
    // iniciar() nunca rejeita (falha de permissão vira gravador.erro) — o
    // efeito abaixo observa esse estado e troca a mensagem pelo destaque.
    gravador.iniciar()
    onAutoIniciarConsumido()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoIniciarGravacao, ativa, online])

  useEffect(() => {
    // a tentativa automática falhou (permissão) — troca o erro por destaque.
    if (autoTentadoRef.current && gravador.erro && gravador.estado === 'parado') {
      setDestaqueMic(true)
    }
  }, [gravador.erro, gravador.estado])

  function aoTocarMic() {
    setDestaqueMic(false)
    if (gravador.estado === 'gravando') {
      gravador.parar()
    } else {
      gravador.iniciar()
    }
  }

  async function interpretarResto() {
    if (!restoPendente) return
    const r = restoPendente
    setRestoPendente(null)
    await enviarTexto(r)
  }

  function tentarDeNovo() {
    if (ultimoAudioRef.current) enviarAudio(ultimoAudioRef.current)
    else enviarTexto(texto)
  }

  async function salvarComoTarefaDoErro() {
    const titulo = texto.trim().slice(0, 120)
    if (!titulo) return
    try {
      await api.criarItem({ tipo: 'compromisso', titulo })
      setTexto('')
      setEstadoEnvio('ocioso')
      setErroEnvio(null)
      onToast(`Criado: ${titulo}`)
      onRecarregar()
    } catch (e) {
      setErroEnvio(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
    }
  }

  function abrirFormularioDoErro() {
    const inicial: NovoItemPayload = { tipo: 'compromisso', titulo: texto.trim().slice(0, 120) }
    onAbrirCriarItem(inicial, texto, 0)
    setEstadoEnvio('ocioso')
    setErroEnvio(null)
  }

  if (ativa === false) return null

  const desabilitada = !online || ativa === null || estadoEnvio === 'entendendo'

  return (
    <div className="captura-bloco">
      <div className="captura-barra">
        <input
          type="text"
          className="captura-campo"
          placeholder="Fale ou escreva…"
          value={texto}
          disabled={desabilitada}
          maxLength={500}
          onChange={(e) => setTexto(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && texto.trim()) enviarTexto(texto)
          }}
        />
        {texto.trim() && (
          <button
            className="captura-enviar"
            aria-label="enviar"
            disabled={desabilitada}
            onClick={() => enviarTexto(texto)}
          >
            <IconeEnviar />
          </button>
        )}
        <button
          className={`captura-mic ${gravador.estado === 'gravando' ? 'gravando' : ''} ${destaqueMic ? 'destaque' : ''}`}
          aria-label={gravador.estado === 'gravando' ? 'parar gravação' : 'gravar'}
          disabled={!online || ativa === null || estadoEnvio === 'entendendo'}
          onClick={aoTocarMic}
        >
          {gravador.estado === 'gravando' ? <IconeParar /> : <IconeMicrofone />}
        </button>
      </div>

      {!online && <div className="captura-aviso">sem rede — captura desligada até a conexão voltar</div>}

      {gravador.estado === 'gravando' && (
        <div className="captura-gravando">
          <div className="captura-nivel" style={{ transform: `scaleX(${0.15 + gravador.nivel * 0.85})` }} />
          <span>gravando… {gravador.segundos}s / 60s</span>
        </div>
      )}

      {gravador.erro && gravador.estado === 'parado' && !destaqueMic && (
        <div className="captura-aviso captura-aviso-erro">{gravador.erro}</div>
      )}

      {estadoEnvio === 'entendendo' && (
        <div className="captura-aviso">
          entendendo…{' '}
          <button className="captura-link" onClick={() => envioRef.current?.abort()}>
            cancelar
          </button>
        </div>
      )}

      {estadoEnvio === 'erro' && erroEnvio && (
        <div className="estado-erro captura-erro-envio">
          {erroEnvio}
          <div className="botoes-linha" style={{ marginTop: 10 }}>
            {erroDeRede && (ultimoAudioRef.current || texto.trim()) && (
              <button className="botao primario" onClick={tentarDeNovo} disabled={!online}>
                tentar de novo
              </button>
            )}
            {!erroDeRede && texto.trim() && (
              <button className="botao primario" onClick={salvarComoTarefaDoErro} disabled={!online}>
                salvar como tarefa
              </button>
            )}
            {!erroDeRede && (
              <button className="botao" onClick={abrirFormularioDoErro}>
                abrir formulário
              </button>
            )}
          </div>
        </div>
      )}

      {restoPendente && (
        <button className="captura-resto" onClick={interpretarResto}>
          Tem mais: <em>{restoPendente}</em> → interpretar
        </button>
      )}

      {resultado && (
        <CapturaSheet
          resultado={resultado}
          online={online}
          onFechar={() => {
            if (resultado.captura_id) api.desfechoCaptura(resultado.captura_id, 'descartada')
            setResultado(null)
          }}
          onConfirmado={(mensagem) => {
            setResultado(null)
            onToast(mensagem)
            onRecarregar()
          }}
          onReinterpretado={(novo) => {
            setResultado(null)
            tratarResultado(novo)
          }}
          onAbrirFormulario={(inicial, vozTexto, capturaId) => {
            setResultado(null)
            onAbrirCriarItem(inicial, vozTexto, capturaId)
          }}
        />
      )}
    </div>
  )
}
