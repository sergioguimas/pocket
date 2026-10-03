import { Component, type ReactNode } from 'react'

// Rede de segurança: erro de renderização vira mensagem com "recarregar",
// nunca tela branca (CONTRATO §6).
export default class ErroGlobal extends Component<{ children: ReactNode }, { erro: Error | null }> {
  state = { erro: null as Error | null }

  static getDerivedStateFromError(erro: Error) {
    return { erro }
  }

  componentDidCatch(erro: Error) {
    console.error('erro de renderização', erro)
  }

  render() {
    if (!this.state.erro) return this.props.children
    return (
      <main className="tela" style={{ paddingTop: 48 }}>
        <h1 style={{ fontSize: 20, marginBottom: 8 }}>Algo quebrou nesta tela</h1>
        <p style={{ color: 'var(--fg-mudo)', marginBottom: 16 }}>{this.state.erro.message}</p>
        <button className="botao" onClick={() => location.reload()}>
          recarregar
        </button>
      </main>
    )
  }
}
