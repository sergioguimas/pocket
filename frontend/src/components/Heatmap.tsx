import type { DiaCalendario } from '../api'
import { hojeISO } from '../format'

interface Props {
  dias: DiaCalendario[]
  onTocar: (dia: DiaCalendario) => void
  desabilitado: (dia: DiaCalendario) => boolean
}

// weekday local em segunda=0..domingo=6 (A1: semana começa na segunda).
function indiceSemana(iso: string): number {
  const [a, m, d] = iso.split('-').map(Number)
  const js = new Date(a, m - 1, d).getDay() // 0=domingo..6=sábado
  return (js + 6) % 7
}

export default function Heatmap({ dias, onTocar, desabilitado }: Props) {
  if (dias.length === 0) return null

  // Preenche células vazias antes do primeiro dia para alinhar a coluna da
  // semana (grid-auto-flow: column, 7 linhas).
  const primeiro = indiceSemana(dias[0].dia)
  const celulas: (DiaCalendario | null)[] = Array(primeiro).fill(null)
  celulas.push(...dias)

  const hoje = hojeISO()

  return (
    <div>
      <div className="heatmap">
        {celulas.map((dia, i) =>
          dia ? (
            <button
              key={dia.dia}
              className="heatmap-dia"
              data-status={dia.status}
              data-futuro={dia.dia > hoje}
              disabled={desabilitado(dia)}
              aria-label={`${dia.dia.split('-').reverse().join('/')}${dia.status ? `, ${dia.status}` : ''}${dia.registros ? `, ${dia.registros} registro${dia.registros > 1 ? 's' : ''}` : ''}`}
              title={`${dia.dia}${dia.status ? ` — ${dia.status}` : ''}${dia.registros ? ` (${dia.registros} registro${dia.registros > 1 ? 's' : ''})` : ''}`}
              onClick={() => onTocar(dia)}
            />
          ) : (
            <span key={`vazio-${i}`} className="heatmap-dia" style={{ visibility: 'hidden' }} />
          ),
        )}
      </div>
      <div className="heatmap-legenda">
        <span>
          <i className="legenda-quadrado" style={{ background: 'var(--acento)' }} /> cumprida
        </span>
        <span>
          <i className="legenda-quadrado" style={{ background: 'var(--erro)', opacity: 0.55 }} /> perdida
        </span>
        <span>
          <i className="legenda-quadrado" style={{ background: 'var(--fg-fraco)' }} /> pulada
        </span>
        <span>
          <i className="legenda-quadrado" style={{ background: 'var(--card-borda)' }} /> sem ocorrência
        </span>
      </div>
    </div>
  )
}
