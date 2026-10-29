import { useEffect, useId, useRef, type ReactNode } from 'react';

export function Dialog({ title, children, onClose, locked = false }: { title: string; children: ReactNode; onClose: () => void; locked?: boolean }) {
  const ref = useRef<HTMLDialogElement>(null);
  const id = useId();
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const dialog = ref.current;
    dialog?.showModal();
    return () => { dialog?.close(); if (previous?.isConnected) previous.focus(); };
  }, []);
  return <dialog ref={ref} aria-labelledby={id} onCancel={event => { event.preventDefault(); if (!locked) onClose(); }}>
    <div className="dialog-header"><h2 id={id}>{title}</h2><button type="button" onClick={onClose} disabled={locked} aria-label="Close dialog">×</button></div>
    <div className="dialog-body">{children}</div>
  </dialog>;
}
