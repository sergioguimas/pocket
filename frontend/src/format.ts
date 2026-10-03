// Formatação e texto em português — nenhuma tela deve montar essas frases
// na mão, para não divergir.
import type { Regra } from './api'

const DIAS_SEMANA_ABREV = ['S', 'T', 'Q', 'Q', 'S', 'S', 'D'] // 1=segunda..7=domingo
// Nome completo para leitor de tela (as letras se repetem: S, Q).
export const DIAS_SEMANA_NOME = ['segunda', 'terça', 'quarta', 'quinta', 'sexta', 'sábado', 'domingo']
const DIAS_SEMANA_NOMES = [
  'segunda',
  'terça',
  'quarta',
  'quinta',
  'sexta',
  'sábado',
  'domingo',
]

export function diaSemanaAbrev(n: number): string {
  return DIAS_SEMANA_ABREV[n - 1] ?? '?'
}

export function hojeISO(d = new Date()): string {
  const ano = d.getFullYear()
  const mes = String(d.getMonth() + 1).padStart(2, '0')
  const dia = String(d.getDate()).padStart(2, '0')
  return `${ano}-${mes}-${dia}`
}

export function formatDataCurta(iso: string): string {
  // 'YYYY-MM-DD' -> 'dd/mm'
  const [, m, d] = iso.split('-')
  return `${d}/${m}`
}

// "hoje", "amanhã", "ontem"; senão "qui., 14 de out." (ano só se não for o corrente).
export function formatDataLonga(iso: string): string {
  const [ano, mes, dia] = iso.split('-').map(Number)
  const d = new Date(ano, mes - 1, dia)
  const agora = new Date()
  const hoje = new Date(agora.getFullYear(), agora.getMonth(), agora.getDate())
  const diff = Math.round((d.getTime() - hoje.getTime()) / 86400000)
  if (diff === 0) return 'hoje'
  if (diff === 1) return 'amanhã'
  if (diff === -1) return 'ontem'
  return d.toLocaleDateString('pt-BR', {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    ...(ano !== agora.getFullYear() ? { year: 'numeric' } : {}),
  })
}

export function formatHora(hhmm: string): string {
  return hhmm
}

export function formatHoraDeRFC3339(rfc: string): string {
  try {
    const d = new Date(rfc)
    return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
  } catch {
    return ''
  }
}

export function diasAtras(dataISO: string, hojeIso = hojeISO()): number {
  const [a1, m1, d1] = dataISO.split('-').map(Number)
  const [a2, m2, d2] = hojeIso.split('-').map(Number)
  const t1 = Date.UTC(a1, m1 - 1, d1)
  const t2 = Date.UTC(a2, m2 - 1, d2)
  return Math.round((t2 - t1) / 86400000)
}

// descreverRegra resume a regra de frequência em linguagem natural — para
// a lista de hábitos e o detalhe.
export function descreverRegra(regra: Regra): string {
  switch (regra.tipo) {
    case 'diario':
      return 'todo dia'
    case 'dias_semana':
      return regra.dias
        .slice()
        .sort((a, b) => a - b)
        .map((n) => DIAS_SEMANA_NOMES[n - 1])
        .join(', ')
    case 'vezes_periodo':
      return `${regra.vezes}× por ${regra.periodo === 'semana' ? 'semana' : 'mês'}`
    case 'intervalo':
      return `a cada ${regra.dias} dia${regra.dias === 1 ? '' : 's'}`
  }
}

export function descreverAntecedencia(a: string): string {
  const m = /^-(\d+)(m|h|d)$/.exec(a)
  if (!m) return a
  const n = Number(m[1])
  const unidade = m[2]
  if (unidade === 'm') return `${n} min antes`
  if (unidade === 'h') return `${n} h antes`
  return `${n} dia${n === 1 ? '' : 's'} antes`
}

export function formatTaxa(taxa: number | null): string {
  if (taxa === null) return 'sem taxa'
  return `${Math.round(taxa * 100)}%`
}

// Estado de hábito "a cada N dias" — ele não tem sequência (CONTRATO §3.6).
export function descreverIntervalo(i: { em_dia: boolean; dias: number }): string {
  const d = (n: number) => `${n} dia${n === 1 ? '' : 's'}`
  if (i.em_dia) return `em dia — faltam ${d(i.dias)}`
  return i.dias === 0 ? 'vence hoje' : `atrasado ${d(i.dias)}`
}
