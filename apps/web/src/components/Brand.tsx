import type { ReactNode } from 'react';

export function Brand() {
  return <div className="brand"><img className="brand-mark" src="/favicon.svg" alt="" width="28" height="28" />Folio</div>;
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
