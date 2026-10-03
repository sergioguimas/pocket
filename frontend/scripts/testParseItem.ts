// Checagem simples de src/parseItem.ts — sem framework de teste (decisão
// F3-3, README "Decisões durante o build"): asserts em Node, rodado com
// `node --experimental-strip-types scripts/testParseItem.ts` (Node 22 lê
// TypeScript direto, sem precisar compilar antes do build).
import assert from 'node:assert/strict'
import { parseItem } from '../src/parseItem.ts'

const casos: [string, { nome: string; quantidade?: string }][] = [
  ['leite 2', { nome: 'leite', quantidade: '2' }],
  ['2 leite', { nome: 'leite', quantidade: '2' }],
  ['leite 2 kg', { nome: 'leite', quantidade: '2 kg' }],
  ['tomate', { nome: 'tomate' }],
  ['Tomates', { nome: 'Tomates' }],
  ['  leite   2  ', { nome: 'leite', quantidade: '2' }],
  ['papel higienico 3 pct', { nome: 'papel higienico', quantidade: '3 pct' }],
  ['3 papel higienico', { nome: 'papel higienico', quantidade: '3' }],
  ['1 cx bombom', { nome: 'cx bombom', quantidade: '1' }], // número isolado no início: regra do contrato vale mesmo sem ser o único token restante
  ['', { nome: '' }],
  ['5', { nome: '5' }], // número sozinho: sem nome depois de tirar a quantidade, mantém como nome
  ['leite 2,5 l', { nome: 'leite', quantidade: '2,5 l' }],
  ['refrigerante 2l', { nome: 'refrigerante 2l' }], // sem espaço entre número e unidade: não separa (mais simples que exigir regex de token colado)
]

let falhas = 0
for (const [entrada, esperado] of casos) {
  const resultado = parseItem(entrada)
  try {
    assert.deepEqual(resultado, esperado)
    console.log(`ok   ${JSON.stringify(entrada)} -> ${JSON.stringify(resultado)}`)
  } catch {
    falhas++
    console.error(`FALHA ${JSON.stringify(entrada)}: esperado ${JSON.stringify(esperado)}, veio ${JSON.stringify(resultado)}`)
  }
}

if (falhas > 0) {
  console.error(`\n${falhas} caso(s) falharam`)
  process.exit(1)
}
console.log(`\n${casos.length} casos ok`)
