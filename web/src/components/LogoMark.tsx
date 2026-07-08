interface LogoMarkProps {
  size?: number
  color?: string
}

export default function LogoMark({ size = 22, color = '#FBF0DC' }: LogoMarkProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <circle cx="12" cy="12" r="10" stroke={color} strokeWidth="1.8" />
      <line x1="12" y1="4"  x2="12"  y2="6.5" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
      <line x1="4"  y1="12" x2="6.5" y2="12"  stroke={color} strokeWidth="1.5" strokeLinecap="round" />
      <line x1="20" y1="12" x2="17.5" y2="12"  stroke={color} strokeWidth="1.5" strokeLinecap="round" />
      <line x1="12" y1="12" x2="17.2" y2="7.8" stroke={color} strokeWidth="2"   strokeLinecap="round" />
      <circle cx="12" cy="12" r="1.5" fill={color} />
    </svg>
  )
}
