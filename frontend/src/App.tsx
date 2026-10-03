import { useEffect, useState, useCallback } from 'react'
import {
  IconeHoje,
  IconeHabitos,
  IconeCompromissos,
  IconeCompras,
  IconeMais,
  IconeAjustes,
} from './icons'
import Hoje from './screens/Hoje'
import Habitos from './screens/Habitos'
import HabitoDetalhe from './screens/HabitoDetalhe'
import Compromissos from './screens/Compromissos'
import Compras from './screens/Compras'
import Ajustes from './screens/Ajustes'
import EditorSheet from './components/EditorSheet'
import { api } from './api'
import type { Item, TipoItem, NovoItemPayload } from './api'
import { useOnline } from './conexao'

type Tela =
  | { nome: 'hoje' }
  | { nome: 'habitos' }
  | { nome: 'habitoDetalhe'; id: number }
  | { nome: 'compromissos' }
  | { nome: 'compras' }
  | { nome: 'ajustes' }

type AbaPrincipal = 'hoje' | 'habitos' | 'compromissos' | 'compras'

// lê ?lista=<id> na abertura (sw.js abre assim no toque de um lembrete de
// lista, item 7 do pedido) e limpa o parâmetro da URL em seguida, para não
// reabrir a mesma lista num refresh manual.
function lerListaAlvoDaURL(): number | null {
  const params = new URLSearchParams(window.location.search)
  const v = params.get('lista')
  if (!v) return null
  const id = Number(v)
  if (!Number.isFinite(id)) return null
  params.delete('lista')
  const resto = params.toString()
  window.history.replaceState(null, '', window.location.pathname + (resto ? `?${resto}` : ''))
  return id
}

// lê ?captura=voz (atalho "Falar com o Pocket" do manifest, §6.5/V11) e
// limpa o parâmetro — igual ao padrão de ?lista= acima.
function lerCapturaVozDaURL(): boolean {
  const params = new URLSearchParams(window.location.search)
  if (params.get('captura') !== 'voz') return false
  params.delete('captura')
  const resto = params.toString()
  window.history.replaceState(null, '', window.location.pathname + (resto ? `?${resto}` : ''))
  return true
}

export default function App() {
  const [listaAlvoId, setListaAlvoId] = useState<number | null>(() => lerListaAlvoDaURL())
  const [tela, setTela] = useState<Tela>(() => (listaAlvoId !== null ? { nome: 'compras' } : { nome: 'hoje' }))
  const online = useOnline()
  const [recarregarChave, setRecarregarChave] = useState(0)
  const [focoAdicionarChave, setFocoAdicionarChave] = useState(0)
  const [autoIniciarGravacao, setAutoIniciarGravacao] = useState(() => lerCapturaVozDaURL())
  const [toast, setToast] = useState<string | null>(null)
  const [capturaResto, setCapturaResto] = useState<string | null>(null)

  // captura (fase 1.5): `item` preenchido = editando de verdade; `inicial`
  // preenchido = criando a partir da proposta do LLM; `capturaId` (>0)
  // marca que este editor veio de uma captura, para mandar o desfecho.
  const [editor, setEditor] = useState<{
    tipo: TipoItem
    item?: Item
    inicial?: NovoItemPayload
    vozTexto?: string
    capturaId?: number
  } | null>(null)

  const forcarRecarga = useCallback(() => setRecarregarChave((n) => n + 1), [])

  useEffect(() => {
    if (!toast) return
    const t = setTimeout(() => setToast(null), 4000)
    return () => clearTimeout(t)
  }, [toast])

  const abaAtiva: AbaPrincipal | null =
    tela.nome === 'hoje'
      ? 'hoje'
      : tela.nome === 'habitos' || tela.nome === 'habitoDetalhe'
        ? 'habitos'
        : tela.nome === 'compromissos'
          ? 'compromissos'
          : tela.nome === 'compras'
            ? 'compras'
            : null

  function abrirCriar(tipo: TipoItem) {
    setEditor({ tipo })
  }

  // "+" flutuante: na aba Compras foca o campo de adicionar em vez de
  // abrir a folha modal (decisão de produto, item 2 do pedido).
  function aoTocarMais() {
    if (abaAtiva === 'compras') {
      setFocoAdicionarChave((n) => n + 1)
      return
    }
    abrirCriar(abaAtiva === 'compromissos' ? 'compromisso' : 'habito')
  }

  function abrirEditar(item: Item) {
    setEditor({ tipo: item.tipo, item })
  }

  // captura (fase 1.5 §6.4): abre já preenchido a partir da proposta do
  // LLM (`criar_item`, ou o escape "Abrir formulário" de `desconhecido`/erro).
  // capturaId 0 = não há linha de captura associada (ex.: erro antes de
  // gerar `captura_id`) — não manda desfecho nesse caso.
  function abrirCapturaEditor(inicial: NovoItemPayload, vozTexto: string, capturaId: number) {
    setEditor({ tipo: inicial.tipo, inicial, vozTexto, capturaId })
  }

  function fecharEditor() {
    // DECISÃO (contrato §6.5): não dá para distinguir "fechou sem mexer" de
    // "mexeu e desistiu" sem rastrear cada campo — fechar sem salvar sempre
    // manda "descartada", salvar sempre manda "confirmada" (nunca "ajustada"
    // pelo EditorSheet).
    if (editor?.capturaId) api.desfechoCaptura(editor.capturaId, 'descartada')
    setEditor(null)
  }

  function aoSalvarEditor(salvo: Item) {
    if (editor?.capturaId) api.desfechoCaptura(editor.capturaId, 'confirmada')
    // veio da captura: confirma com o mesmo toast dos cartões (§6.6)
    if (editor?.vozTexto !== undefined) setToast(`Criado: ${salvo.titulo}`)
    setEditor(null)
    forcarRecarga()
  }

  const titulo =
    tela.nome === 'hoje'
      ? 'Hoje'
      : tela.nome === 'habitos'
        ? 'Hábitos'
        : tela.nome === 'habitoDetalhe'
          ? 'Hábito'
          : tela.nome === 'compromissos'
            ? 'Compromissos'
            : tela.nome === 'compras'
              ? 'Compras'
              : 'Ajustes'

  return (
    <div className="app">
      <header className="topbar">
        <h1>
          {tela.nome === 'hoje' ? (
            <>
              Pocket
            </>
          ) : (
            titulo
          )}
        </h1>
        <button
          className="icone-botao"
          aria-label="Ajustes"
          onClick={() => setTela({ nome: 'ajustes' })}
        >
          <IconeAjustes />
        </button>
      </header>

      {!online && (
        <div style={{ padding: '0 16px' }}>
          <div className="banner-offline">sem conexão com o redmilab</div>
        </div>
      )}

      <main className="tela">
        {tela.nome === 'hoje' && (
          <Hoje
            key={recarregarChave}
            online={online}
            onAbrirCriarHabito={() => abrirCriar('habito')}
            onIrParaCompromissos={() => setTela({ nome: 'compromissos' })}
            onIrParaCompras={(listaId) => {
              setListaAlvoId(listaId)
              setTela({ nome: 'compras' })
            }}
            onAbrirCapturaEditor={abrirCapturaEditor}
            onToast={setToast}
            autoIniciarGravacao={autoIniciarGravacao}
            onAutoIniciarConsumido={() => setAutoIniciarGravacao(false)}
            capturaResto={capturaResto}
            onCapturaResto={setCapturaResto}
          />
        )}
        {tela.nome === 'habitos' && (
          <Habitos
            key={recarregarChave}
            onAbrirDetalhe={(id) => setTela({ nome: 'habitoDetalhe', id })}
          />
        )}
        {tela.nome === 'habitoDetalhe' && (
          <HabitoDetalhe
            id={tela.id}
            onVoltar={() => setTela({ nome: 'habitos' })}
            onEditar={abrirEditar}
          />
        )}
        {tela.nome === 'compromissos' && (
          <Compromissos key={recarregarChave} onAbrirEditar={abrirEditar} />
        )}
        {tela.nome === 'compras' && (
          <Compras
            listaAlvoId={listaAlvoId}
            onListaAlvoConsumida={() => setListaAlvoId(null)}
            focoChave={focoAdicionarChave}
          />
        )}
        {tela.nome === 'ajustes' && <Ajustes onVoltar={() => setTela({ nome: 'hoje' })} />}
      </main>

      <div className="bottom-nav-wrap">
        <nav className="bottom-nav">
          <button
            aria-current={abaAtiva === 'hoje'}
            onClick={() => setTela({ nome: 'hoje' })}
          >
            <IconeHoje />
            Hoje
          </button>
          <button
            aria-current={abaAtiva === 'habitos'}
            onClick={() => setTela({ nome: 'habitos' })}
          >
            <IconeHabitos />
            Hábitos
          </button>
          <button
            aria-current={abaAtiva === 'compromissos'}
            onClick={() => setTela({ nome: 'compromissos' })}
          >
            <IconeCompromissos />
            Compromissos
          </button>
          <button
            aria-current={abaAtiva === 'compras'}
            onClick={() => setTela({ nome: 'compras' })}
          >
            <IconeCompras />
            Compras
          </button>
          <button
            className="botao-adicionar"
            aria-label={abaAtiva === 'compras' ? 'Adicionar item' : 'Adicionar'}
            onClick={aoTocarMais}
          >
            <IconeMais />
          </button>
        </nav>
      </div>

      {editor && (
        <EditorSheet
          tipoInicial={editor.tipo}
          item={editor.item}
          inicial={editor.inicial}
          vozTexto={editor.vozTexto}
          onFechar={fecharEditor}
          onSalvo={aoSalvarEditor}
        />
      )}

      {toast && <div className="desfazer-barra captura-toast">{toast}</div>}
    </div>
  )
}
