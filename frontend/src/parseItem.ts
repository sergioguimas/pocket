// Parser do campo "adicionar item" da aba Compras (CONTRATO-FASE3 §6/K2):
// "leite 2" / "2 leite" / "leite 2 kg" → nome + quantidade. Número isolado
// no início ou no fim vira quantidade; "2 kg" no fim (número + unidade
// curta de até 3 letras) também. Função pura, sem dependência do DOM — veja
// scripts/testParseItem.mjs para os casos verificados.

export interface ItemParseado {
  nome: string
  quantidade?: string
}

const NUMERO = /^\d+([.,]\d+)?$/
// Unidade curta: só letras (com acento), 1 a 3 caracteres — "kg", "cx",
// "un", "g", "l", "ml", "pct" caberiam se tivessem <=3.
const UNIDADE_CURTA = /^[a-zà-ú]{1,3}$/i

export function parseItem(entrada: string): ItemParseado {
  const texto = entrada.trim().replace(/\s+/g, ' ')
  if (!texto) return { nome: '' }

  const tokens = texto.split(' ')
  if (tokens.length < 2) return { nome: texto }

  // "leite 2 kg" — últimos dois tokens são número + unidade curta.
  if (tokens.length >= 3) {
    const [penultimo, ultimo] = tokens.slice(-2)
    if (NUMERO.test(penultimo) && UNIDADE_CURTA.test(ultimo) && !NUMERO.test(ultimo)) {
      const nome = tokens.slice(0, -2).join(' ').trim()
      if (nome) return { nome, quantidade: `${penultimo} ${ultimo}` }
    }
  }

  // "leite 2" — número isolado no fim.
  const ultimo = tokens[tokens.length - 1]
  if (NUMERO.test(ultimo)) {
    const nome = tokens.slice(0, -1).join(' ').trim()
    if (nome) return { nome, quantidade: ultimo }
  }

  // "2 leite" — número isolado no início.
  const primeiro = tokens[0]
  if (NUMERO.test(primeiro)) {
    const nome = tokens.slice(1).join(' ').trim()
    if (nome) return { nome, quantidade: primeiro }
  }

  return { nome: texto }
}
