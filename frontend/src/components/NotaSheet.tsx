import { useState } from 'react'
import { api, ApiError, NetworkError } from '../api'
import type { Registro } from '../api'
import { IconeFechar } from '../icons'

interface Props {
  registroId: number
  titulo: string // nome do hábito, para o cabeçalho
  quando?: string // "hoje", "27/09"… — opcional
  notaInicial?: string
  onFechar: () => void
  onSalvo: (registro: Registro) => void
}

// Anotação de um feito: "o que eu fiz" além do "fiz" (pedido de 28/09) —
// capítulo lido, treino, episódio. Vazio apaga a nota.
export default function NotaSheet({ registroId, titulo, quando, notaInicial, onFechar, onSalvo }: Props) {
  const [nota, setNota] = useState(notaInicial ?? '')
  const [salvando, setSalvando] = useState(false)
  const [erro, setErro] = useState<string | null>(null)

  async function salvar() {
    setSalvando(true)
    setErro(null)
    try {
      onSalvo(await api.editarNotaRegistro(registroId, nota))
    } catch (e) {
      setErro(e instanceof NetworkError ? 'sem conexão com o redmilab' : e instanceof ApiError ? e.message : 'erro inesperado')
    } finally {
      setSalvando(false)
    }
  }

  return (
    <div className="folha-fundo" onClick={onFechar}>
      <div className="folha" role="dialog" aria-modal="true" aria-label={`Anotar ${titulo}`} onClick={(e) => e.stopPropagation()}>
        <div className="folha-cabecalho">
          <h2>
            {titulo}
            {quando && <span style={{ color: 'var(--fg-mudo)', fontWeight: 400 }}> · {quando}</span>}
          </h2>
          <button className="icone-botao" aria-label="fechar" onClick={onFechar}>
            <IconeFechar />
          </button>
        </div>

        {erro && <div className="estado-erro">{erro}</div>}

        <div className="campo">
          <label htmlFor="nota-registro">O que você fez?</label>
          <textarea
            id="nota-registro"
            autoFocus
            maxLength={500}
            rows={4}
            value={nota}
            onChange={(e) => setNota(e.target.value)}
            placeholder="ex.: Salmo 23 · cap. 4 do livro · 3×12 supino 40 kg"
          />
        </div>

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
