import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import ErroGlobal from './components/ErroGlobal'
import './index.css'
import { garantirInscricao, permissao } from './push'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErroGlobal>
      <App />
    </ErroGlobal>
  </StrictMode>,
)

if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch((e) => console.error('sw.js', e))
  })
}

// Toda abertura do app: se a permissão já foi concedida, garante a
// inscrição (troca de endpoint, migração do spike — CONTRATO §5/§6).
if (permissao() === 'granted') {
  garantirInscricao()
}
