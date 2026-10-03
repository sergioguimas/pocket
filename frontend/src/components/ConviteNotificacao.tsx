import { useState } from 'react'
import { pedirPermissaoEInscrever } from '../push'
import { IconeSino, IconeFechar } from '../icons'

// Convite discreto em Hoje quando a permissão de notificação ainda não foi
// decidida (CONTRATO §5/§6) — some depois de aceitar ou fechar.
export default function ConviteNotificacao({ onFechar }: { onFechar: () => void }) {
  const [processando, setProcessando] = useState(false)

  async function ativar() {
    setProcessando(true)
    const ok = await pedirPermissaoEInscrever()
    setProcessando(false)
    if (ok) onFechar()
  }

  return (
    <div className="convite-notificacao">
      <IconeSino />
      <p style={{ flex: 1 }}>Ativar notificações para receber lembretes dos seus hábitos e compromissos.</p>
      <button className="icone-botao" aria-label="dispensar" onClick={onFechar}>
        <IconeFechar />
      </button>
      <button className="botao primario" disabled={processando} onClick={ativar}>
        {processando ? '…' : 'Ativar'}
      </button>
    </div>
  )
}
