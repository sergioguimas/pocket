// Gravador de voz para a captura (CONTRATO-FASE15.md §6.2): toque para
// começar, toque para parar (nunca "segurar"), para sozinho aos 60 s.
// Mostra tempo decorrido e um nível simples (AnalyserNode) enquanto grava.
// Libera as trilhas do microfone ao parar/desmontar — nada fica gravando
// em segundo plano.
import { useCallback, useEffect, useRef, useState } from 'react'

const DURACAO_MAXIMA_S = 60

// Ordem de preferência dos mimes suportados (§6.3): opus é o menor/melhor,
// ogg e mp4 são os fallbacks que o Go também decodifica.
const MIMES_PREFERIDOS = ['audio/webm;codecs=opus', 'audio/ogg', 'audio/mp4']

function escolherMime(): string | undefined {
  if (typeof MediaRecorder === 'undefined') return undefined
  for (const m of MIMES_PREFERIDOS) {
    if (MediaRecorder.isTypeSupported(m)) return m
  }
  return undefined
}

export type EstadoGravador = 'parado' | 'gravando' | 'processando'

interface UseGravadorResult {
  estado: EstadoGravador
  segundos: number
  nivel: number // 0..1, para uma barrinha de nível simples
  erro: string | null
  iniciar: () => Promise<void>
  parar: () => void
  // o hook só sabe gravar; quem chama onGravado (a barra de captura) decide
  // quando o processamento (POST /api/captura) terminou e chama isto para
  // voltar o botão ao estado ocioso.
  finalizarProcessamento: () => void
}

// Mensagem pt-BR com o caminho para liberar o microfone no Chrome (§6.2) —
// mostrada quando getUserMedia é negado.
export const MENSAGEM_PERMISSAO_NEGADA =
  'permissão de microfone negada — no Chrome, toque no cadeado ao lado do endereço, abra "Permissões do site" e libere o microfone para este site'

export function useGravador(onGravado: (blob: Blob) => void): UseGravadorResult {
  const [estado, setEstado] = useState<EstadoGravador>('parado')
  const [segundos, setSegundos] = useState(0)
  const [nivel, setNivel] = useState(0)
  const [erro, setErro] = useState<string | null>(null)

  const streamRef = useRef<MediaStream | null>(null)
  const recorderRef = useRef<MediaRecorder | null>(null)
  const chunksRef = useRef<Blob[]>([])
  const cronometroRef = useRef<number | null>(null)
  const audioCtxRef = useRef<AudioContext | null>(null)
  const analiserRef = useRef<AnalyserNode | null>(null)
  const nivelRafRef = useRef<number | null>(null)

  const liberarTudo = useCallback(() => {
    if (cronometroRef.current) {
      window.clearInterval(cronometroRef.current)
      cronometroRef.current = null
    }
    if (nivelRafRef.current) {
      cancelAnimationFrame(nivelRafRef.current)
      nivelRafRef.current = null
    }
    streamRef.current?.getTracks().forEach((t) => t.stop())
    streamRef.current = null
    if (audioCtxRef.current) {
      audioCtxRef.current.close().catch(() => {})
      audioCtxRef.current = null
    }
    analiserRef.current = null
    setNivel(0)
  }, [])

  useEffect(() => liberarTudo, [liberarTudo])

  const medirNivel = useCallback(() => {
    const analiser = analiserRef.current
    if (!analiser) return
    const dados = new Uint8Array(analiser.frequencyBinCount)
    analiser.getByteTimeDomainData(dados)
    let soma = 0
    for (const v of dados) {
      const d = (v - 128) / 128
      soma += d * d
    }
    const rms = Math.sqrt(soma / dados.length)
    setNivel(Math.min(1, rms * 4))
    nivelRafRef.current = requestAnimationFrame(medirNivel)
  }, [])

  const parar = useCallback(() => {
    if (recorderRef.current && recorderRef.current.state !== 'inactive') {
      recorderRef.current.stop()
    }
  }, [])

  const iniciar = useCallback(async () => {
    setErro(null)
    let stream: MediaStream
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    } catch {
      setErro(MENSAGEM_PERMISSAO_NEGADA)
      return
    }
    streamRef.current = stream

    const mime = escolherMime()
    const recorder = mime ? new MediaRecorder(stream, { mimeType: mime }) : new MediaRecorder(stream)
    recorderRef.current = recorder
    chunksRef.current = []

    recorder.ondataavailable = (e) => {
      if (e.data.size > 0) chunksRef.current.push(e.data)
    }
    recorder.onstop = () => {
      liberarTudo()
      setEstado('processando')
      const blob = new Blob(chunksRef.current, { type: recorder.mimeType || mime || 'audio/webm' })
      chunksRef.current = []
      onGravado(blob)
    }

    try {
      const AudioCtx = window.AudioContext ?? (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
      const ctx = new AudioCtx()
      audioCtxRef.current = ctx
      const fonte = ctx.createMediaStreamSource(stream)
      const analiser = ctx.createAnalyser()
      analiser.fftSize = 512
      fonte.connect(analiser)
      analiserRef.current = analiser
      nivelRafRef.current = requestAnimationFrame(medirNivel)
    } catch {
      // sem medidor de nível — a gravação segue normal
    }

    recorder.start()
    setEstado('gravando')
    setSegundos(0)
    cronometroRef.current = window.setInterval(() => {
      setSegundos((s) => {
        const proximo = s + 1
        if (proximo >= DURACAO_MAXIMA_S) parar()
        return proximo
      })
    }, 1000)
  }, [liberarTudo, medirNivel, onGravado, parar])

  const finalizarProcessamento = useCallback(() => setEstado('parado'), [])

  return { estado, segundos, nivel, erro, iniciar, parar, finalizarProcessamento }
}
