
export function Logo({ size = 24, className = "" }: { size?: number; className?: string }) {
  return (
    <img
      src="/logo.png"
      width={size}
      height={size}
      alt="VEBOX"
      draggable={false}
      className={`shrink-0 select-none object-contain ${className}`}
    />
  );
}

export function LogoLarge({ size = 120, className = "" }: { size?: number; className?: string }) {
  return <Logo size={size} className={className} />;
}
