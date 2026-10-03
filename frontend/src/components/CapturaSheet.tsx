// Cartão de resultado da captura para `registrar`, `compras` e
// `desconhecido` (CONTRATO-FASE15.md §2/§6.3). `criar_item` não passa por
// aqui — abre o EditorSheet pré-preenchido direto (ver CapturaBarra).
import { useState } from 'react'
import { api, ApiError, NetworkError } from '../api'
import type { CapturaResposta, NovoItemPayload } from '../api'
import { IconeFechar } from '../icons'

const TITULO_MAX = 120 // mesmo limite do campo Título no EditorSheet

interface Props {
  resultado: CapturaResposta
  online: boolean
  onFechar: () => void // fechou sem confirmar -> descartada (a barra manda o desfecho)
  onConfirmado: (mensagemToast: string) => void
  onReinterpretado: (novo: CapturaResposta) => void
  onAbrirFormulario: (inicial: NovoItemPayload, vozTexto: string, capturaId: number) => void
}

function formatQuandoRegistro(em: string | null): string {
  if (!em) return 'hoje'
  const d = new Date(em)
  if (Number.isNaN(d.getTime())) return 'hoje'
  const hoje = new Date()
  const mesmoDia = (a: Date, b: Date) =>
    a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
  if (mesmoDia(d, hoje)) return 'hoje'
  const ontem = new Date(hoje)
  ontem.setDate(hoje.getDate() - 1)
  if (mesmoDia(d, ontem)) return 'ontem'
  return `${String(d.getDate()).padStart(2, '0')}/${String(d.getMonth() + 1).padStart(2, '0')}`
}

export default function CapturaSheet({ resultado, online, onFechar, onConfirmado, onReinterpretado, onAbrirFormulario }: Props) {
  const [textoEditavel, setTextoEditavel] = useState(resultado.texto)
  const [editandoTexto, setEditandoTexto] = useState(false)
  const [reinterpretando, setReinterpretando] = useState(false)
  const [erroReinterpretar, setErroReinterpretar] = useState<string | null>(null)

  const [salvando, setSalvando] = useState(false)
  const [erroSalvar, setErroSalvar] = useState<string | null>(null)

  async function reinterpretar() {
    const t = textoEditavel.trim()
    if (!t) return
    if (t === resultado.texto) {
      setEditandoTexto(false)
      return
    }
    setReinterpretando(true)
    setErroReinterpretar(null)
    try {
      const novo = await api.capturarTexto(t)
      // a proposta original foi trocada por uma reinterpretação — descartada.
      if (resultado.captura_id) api.desfechoCaptura(resultado.captura_id, 'descartada')
      onReinterpretado(novo)
    } catch (e) {
      setErroReinterpretar(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
    } finally {
      setReinterpretando(false)
    }
  }

  const titulo =
    resultado.acao === 'registrar'
      ? 'Registrar'
      : resultado.acao === 'compras'
        ? 'Comprar'
        : 'Não entendi bem'

  return (
    <div className="folha-fundo" onClick={onFechar}>
      <div className="folha" role="dialog" aria-modal="true" aria-label={titulo} onClick={(e) => e.stopPropagation()}>
        <div className="folha-cabecalho">
          <h2>{titulo}</h2>
          <button className="icone-botao" aria-label="fechar" onClick={onFechar}>
            <IconeFechar />
          </button>
        </div>

        <div className="captura-disse">
          {!editandoTexto ? (
            <>
              <p>
                Você disse: <em>{resultado.texto}</em>
              </p>
              <button className="captura-link" onClick={() => setEditandoTexto(true)}>
                editar texto
              </button>
            </>
          ) : (
            <div className="campo">
              <textarea
                autoFocus
                maxLength={500}
                value={textoEditavel}
                onChange={(e) => setTextoEditavel(e.target.value)}
              />
              {erroReinterpretar && <div className="estado-erro">{erroReinterpretar}</div>}
              <div className="botoes-linha">
                <button className="botao" onClick={() => { setEditandoTexto(false); setTextoEditavel(resultado.texto) }} disabled={reinterpretando}>
                  cancelar
                </button>
                <button className="botao primario" onClick={reinterpretar} disabled={reinterpretando}>
                  {reinterpretando ? 'reinterpretando…' : 'reinterpretar'}
                </button>
              </div>
            </div>
          )}
        </div>

        {(resultado.avisos ?? []).length > 0 && (
          <div className="captura-avisos">
            {resultado.avisos.map((a, i) => (
              <div key={i} className="captura-aviso">
                {a}
              </div>
            ))}
          </div>
        )}

        {erroSalvar && <div className="estado-erro">{erroSalvar}</div>}

        {resultado.acao === 'registrar' && resultado.registro && (
          <CartaoRegistrar
            registro={resultado.registro}
            online={online}
            salvando={salvando}
            onSalvar={async (nota) => {
              setSalvando(true)
              setErroSalvar(null)
              try {
                await api.criarRegistro(resultado.registro!.item_id, {
                  em: resultado.registro!.em ?? undefined,
                  nota: nota.trim() || undefined,
                })
                const mudou = nota.trim() !== (resultado.registro!.nota ?? '').trim()
                if (resultado.captura_id) api.desfechoCaptura(resultado.captura_id, mudou ? 'ajustada' : 'confirmada')
                const titulo = resultado.registro!.titulo
                onConfirmado(resultado.registro!.tipo === 'compromisso' ? `Concluído: ${titulo}` : `Registrado em ${titulo}`)
              } catch (e) {
                setErroSalvar(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
              } finally {
                setSalvando(false)
              }
            }}
          />
        )}

        {resultado.acao === 'compras' && resultado.compras && (
          <CartaoCompras
            compras={resultado.compras}
            online={online}
            salvando={salvando}
            onSalvar={async (itens) => {
              const marcados = itens.filter((i) => i.marcado && i.nome.trim())
              if (marcados.length === 0) {
                setErroSalvar('marque ao menos um item')
                return
              }
              setSalvando(true)
              setErroSalvar(null)
              try {
                for (const it of marcados) {
                  // sequencial (contrato §2): a regra de duplicata da fase 3
                  // depende da ordem de chegada na mesma lista.
                  // eslint-disable-next-line no-await-in-loop
                  await api.adicionarItemCompra(resultado.compras!.lista_id, {
                    nome: it.nome.trim(),
                    quantidade: it.quantidade.trim() || undefined,
                  })
                }
                const original = resultado.compras!.itens
                const mudou =
                  marcados.length !== original.length ||
                  marcados.some((it, i) => it.nome.trim() !== original[i]?.nome || it.quantidade.trim() !== (original[i]?.quantidade ?? ''))
                if (resultado.captura_id) api.desfechoCaptura(resultado.captura_id, mudou ? 'ajustada' : 'confirmada')
                onConfirmado(`${marcados.length} ${marcados.length === 1 ? 'item' : 'itens'} em ${resultado.compras!.lista_nome}`)
              } catch (e) {
                setErroSalvar(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
              } finally {
                setSalvando(false)
              }
            }}
          />
        )}

        {(resultado.acao === 'desconhecido' || (resultado.acao === 'criar_item' && !resultado.item)) && (
          <div className="folha-rodape" style={{ flexDirection: 'column' }}>
            <button
              className="botao primario botao-largo"
              disabled={salvando || !online}
              onClick={async () => {
                const titulo = resultado.texto.trim().slice(0, TITULO_MAX)
                setSalvando(true)
                setErroSalvar(null)
                try {
                  await api.criarItem({ tipo: 'compromisso', titulo })
                  if (resultado.captura_id) api.desfechoCaptura(resultado.captura_id, 'confirmada')
                  onConfirmado(`Criado: ${titulo}`)
                } catch (e) {
                  setErroSalvar(e instanceof ApiError || e instanceof NetworkError ? e.message : 'erro inesperado')
                } finally {
                  setSalvando(false)
                }
              }}
            >
              {salvando ? 'salvando…' : 'salvar como tarefa'}
            </button>
            <button
              className="botao botao-largo"
              disabled={salvando}
              style={{ marginTop: 8 }}
              onClick={() =>
                onAbrirFormulario(
                  { tipo: 'compromisso', titulo: resultado.texto.trim().slice(0, TITULO_MAX) },
                  resultado.texto,
                  resultado.captura_id,
                )
              }
            >
              abrir formulário
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

function CartaoRegistrar({
  registro,
  online,
  salvando,
  onSalvar,
}: {
  registro: NonNullable<CapturaResposta['registro']>
  online: boolean
  salvando: boolean
  onSalvar: (nota: string) => void
}) {
  const [nota, setNota] = useState(registro.nota ?? '')
  const ehCompromisso = registro.tipo === 'compromisso'
  const quando = formatQuandoRegistro(registro.em)

  return (
    <div>
      <p className="captura-pergunta">
        {ehCompromisso ? (
          <>
            Concluir <strong>{registro.titulo}</strong>?
          </>
        ) : (
          <>
            Registrar <strong>{registro.titulo}</strong> {quando}?
          </>
        )}
      </p>
      <div className="campo">
        <label htmlFor="captura-nota">Nota (opcional)</label>
        <textarea id="captura-nota" maxLength={500} value={nota} onChange={(e) => setNota(e.target.value)} />
      </div>
      <div className="folha-rodape">
        <button className="botao primario botao-largo" disabled={salvando || !online} onClick={() => onSalvar(nota)}>
          {salvando ? 'salvando…' : ehCompromisso ? 'concluir' : 'registrar'}
        </button>
      </div>
    </div>
  )
}

interface ItemComprasEditavel {
  nome: string
  quantidade: string
  marcado: boolean
}

function CartaoCompras({
  compras,
  online,
  salvando,
  onSalvar,
}: {
  compras: NonNullable<CapturaResposta['compras']>
  online: boolean
  salvando: boolean
  onSalvar: (itens: ItemComprasEditavel[]) => void
}) {
  const [itens, setItens] = useState<ItemComprasEditavel[]>(
    compras.itens.map((i) => ({ nome: i.nome, quantidade: i.quantidade ?? '', marcado: true })),
  )

  function atualizar(i: number, patch: Partial<ItemComprasEditavel>) {
    setItens((atual) => atual.map((it, idx) => (idx === i ? { ...it, ...patch } : it)))
  }

  return (
    <div>
      <p className="captura-pergunta">
        em <strong>{compras.lista_nome}</strong>
      </p>
      <div className="lista">
        {itens.map((it, i) => (
          <div key={i} className="captura-item-compra">
            <input
              type="checkbox"
              checked={it.marcado}
              aria-label={`incluir ${it.nome || 'item'}`}
              onChange={(e) => atualizar(i, { marcado: e.target.checked })}
            />
            <input
              type="text"
              className="captura-item-nome"
              value={it.nome}
              maxLength={60}
              placeholder="nome"
              onChange={(e) => atualizar(i, { nome: e.target.value })}
            />
            <input
              type="text"
              className="captura-item-qtd"
              value={it.quantidade}
              placeholder="qtd."
              onChange={(e) => atualizar(i, { quantidade: e.target.value })}
            />
          </div>
        ))}
      </div>
      <div className="folha-rodape">
        <button className="botao primario botao-largo" disabled={salvando || !online} onClick={() => onSalvar(itens)}>
          {salvando ? 'salvando…' : 'confirmar'}
        </button>
      </div>
    </div>
  )
}
