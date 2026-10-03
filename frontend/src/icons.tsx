// Ícones em SVG inline, traço simples — nada de biblioteca de ícones (sem
// dependência extra) nem emoji.
import type { JSX } from 'react'

type Props = { className?: string }

function base(children: JSX.Element, props?: Props) {
  return (
    // width/height padrão: sem eles o SVG ocupa 100% do container (a seta da
    // lista de hábitos e o "voltar" viravam ícones gigantes). CSS sobrescreve.
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={props?.className}
      aria-hidden="true"
    >
      {children}
    </svg>
  )
}

export const IconeHoje = (p?: Props) =>
  base(
    <>
      <path d="M8 3v4M16 3v4M3 9h18" />
      <rect x="3" y="5" width="18" height="16" rx="2" />
      <path d="M9 15l2 2 4-4" />
    </>,
    p,
  )

export const IconeHabitos = (p?: Props) =>
  base(
    <>
      <path d="M3 12a9 9 0 1 0 9-9" />
      <path d="M3 3v6h6" />
      <path d="M12 8v4l3 3" />
    </>,
    p,
  )

export const IconeCompromissos = (p?: Props) =>
  base(
    <>
      <rect x="4" y="5" width="16" height="15" rx="2" />
      <path d="M8 3v4M16 3v4M4 10h16" />
      <path d="M9 14h.01M12 14h.01M15 14h.01M9 17h.01M12 17h.01" />
    </>,
    p,
  )

export const IconeMais = (p?: Props) => base(<path d="M12 5v14M5 12h14" />, p)

export const IconeAjustes = (p?: Props) =>
  base(
    <>
      <circle cx="12" cy="12" r="3.2" />
      <path d="M19.4 13.5a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1.03 1.56V19.6a2 2 0 1 1-4 0v-.09A1.7 1.7 0 0 0 8.98 18a1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.7 1.7 0 0 0 4.6 13.5a1.7 1.7 0 0 0-1.56-1.03H2.9a2 2 0 1 1 0-4h.09A1.7 1.7 0 0 0 4.6 7.5a1.7 1.7 0 0 0-.34-1.87l-.06-.06A2 2 0 1 1 7.03 2.74l.06.06A1.7 1.7 0 0 0 8.96 3.14a1.7 1.7 0 0 0 1.03-1.56V1.5a2 2 0 1 1 4 0v.09A1.7 1.7 0 0 0 15.02 3a1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.7 1.7 0 0 0-.34 1.87v.02A1.7 1.7 0 0 0 21 8.98h.09a2 2 0 1 1 0 4h-.09a1.7 1.7 0 0 0-1.6 1.02z" />
    </>,
    p,
  )

export const IconeCheck = (p?: Props) => base(<path d="M20 6 9 17l-5-5" />, p)

export const IconeSeta = (p?: Props) => base(<path d="M9 6l6 6-6 6" />, p)

export const IconeVoltar = (p?: Props) => base(<path d="M15 6l-6 6 6 6" />, p)

export const IconeFechar = (p?: Props) => base(<path d="M6 6l12 12M18 6L6 18" />, p)

export const IconeCompras = (p?: Props) =>
  base(
    <>
      <path d="M6 6h15l-1.5 9h-12z" />
      <path d="M6 6 5 3H2" />
      <circle cx="9.5" cy="19" r="1.4" />
      <circle cx="17.5" cy="19" r="1.4" />
    </>,
    p,
  )

export const IconeMenuPontos = (p?: Props) =>
  base(
    <>
      <circle cx="5" cy="12" r="1.3" fill="currentColor" stroke="none" />
      <circle cx="12" cy="12" r="1.3" fill="currentColor" stroke="none" />
      <circle cx="19" cy="12" r="1.3" fill="currentColor" stroke="none" />
    </>,
    p,
  )

export const IconeSetaCima = (p?: Props) => base(<path d="M6 15l6-6 6 6" />, p)

export const IconeSetaBaixo = (p?: Props) => base(<path d="M6 9l6 6 6-6" />, p)

export const IconeSino = (p?: Props) =>
  base(
    <>
      <path d="M6 8a6 6 0 1 1 12 0c0 5 2 6 2 6H4s2-1 2-6" />
      <path d="M10 20a2 2 0 0 0 4 0" />
    </>,
    p,
  )

export const IconeMicrofone = (p?: Props) =>
  base(
    <>
      <rect x="9" y="2" width="6" height="12" rx="3" />
      <path d="M5 10a7 7 0 0 0 14 0" />
      <path d="M12 17v4M9 21h6" />
    </>,
    p,
  )

export const IconeParar = (p?: Props) => base(<rect x="6" y="6" width="12" height="12" rx="2" />, p)

export const IconeEnviar = (p?: Props) => base(<path d="M4 12h16M13 5l7 7-7 7" />, p)
