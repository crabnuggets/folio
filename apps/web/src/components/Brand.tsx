import type { ReactNode } from 'react';

export function Brand() {
  return <div className="brand"><svg className="brand-mark" aria-hidden="true" viewBox="0 0 32 32" width="28" height="28"><rect width="32" height="32" rx="8" fill="currentColor" /><path d="M8 11h9M21 11h3M8 16h9M21 16h3M8 21h9M21 21h3" stroke="var(--on-accent)" strokeWidth="2" strokeLinecap="round" /></svg>Folio</div>;
}

function Icon({ size = 18, children }: { size?: number; children: ReactNode }) {
  return <svg aria-hidden="true" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{children}</svg>;
}
export const ChevronLeft = () => <Icon><path d="m15 6-6 6 6 6" /></Icon>;
export const ChevronRight = () => <Icon><path d="m9 6 6 6-6 6" /></Icon>;
export const ChevronDown = () => <Icon size={16}><path d="m6 9 6 6 6-6" /></Icon>;
export const Check = () => <Icon size={16}><path d="m5 12.5 4.5 4.5L19 7.5" /></Icon>;
export const Alert = () => <Icon size={16}><path d="M12 8v5M12 16.5h.01" /><circle cx="12" cy="12" r="9" /></Icon>;
export const Close = () => <Icon size={16}><path d="M6 6l12 12M18 6 6 18" /></Icon>;
export const Plus = () => <Icon size={16}><path d="M12 5v14M5 12h14" /></Icon>;
export const More = () => <Icon><path d="M5 12h.01M12 12h.01M19 12h.01" strokeWidth="2.6" /></Icon>;
export const Undo = () => <Icon><path d="M9 5 4 10l5 5" /><path d="M4 10h10a6 6 0 0 1 6 6v2" /></Icon>;
export const Redo = () => <Icon><path d="m15 5 5 5-5 5" /><path d="M20 10H10a6 6 0 0 0-6 6v2" /></Icon>;
export const Attach = () => <Icon size={20}><path d="m20 11.5-8.2 8.2a5 5 0 0 1-7-7l8.5-8.5a3.3 3.3 0 0 1 4.7 4.7l-8.5 8.5a1.7 1.7 0 0 1-2.4-2.4l7.8-7.8" /></Icon>;
export const Sliders = () => <Icon size={20}><path d="M4 7h9M17 7h3M4 17h3M11 17h9" /><circle cx="15" cy="7" r="2" /><circle cx="9" cy="17" r="2" /></Icon>;
export const Send = () => <Icon><path d="M12 19V5M6 11l6-6 6 6" /></Icon>;
