import { createContext } from 'preact';
import type { ComponentChildren } from 'preact';
import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { Modal } from './Modal';

export interface ConfirmOptions {
  title: string;
  message: string;
  confirmLabel: string;
  /** Style the confirming button as destructive. */
  danger?: boolean;
}

interface UI {
  toast: (message: string) => void;
  confirm: (options: ConfirmOptions) => Promise<boolean>;
}

const UIContext = createContext<UI | null>(null);

export function useUI(): UI {
  const value = useContext(UIContext);
  if (!value) throw new Error('useUI must be used inside UIProvider');
  return value;
}

interface Pending {
  options: ConfirmOptions;
  resolve: (ok: boolean) => void;
}

/** Owns the transient toast and the confirmation dialog shared by every page. */
export function UIProvider({ children }: { children: ComponentChildren }) {
  const [message, setMessage] = useState('');
  const [visible, setVisible] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  const [pending, setPending] = useState<Pending | null>(null);

  const toast = useCallback((text: string) => {
    setMessage(text);
    setVisible(true);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setVisible(false), 2600);
  }, []);

  const confirm = useCallback(
    (options: ConfirmOptions) =>
      new Promise<boolean>((resolve) => {
        setPending({ options, resolve });
      }),
    [],
  );

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const answer = (ok: boolean) => {
    pending?.resolve(ok);
    setPending(null);
  };

  const value = useMemo(() => ({ toast, confirm }), [toast, confirm]);

  return (
    <UIContext.Provider value={value}>
      {children}
      <div class={`toast${visible ? ' show' : ''}`} role="status" aria-live="polite">
        {message}
      </div>
      {pending && (
        <Modal titleId="confirm-title" title={pending.options.title} onCancel={() => answer(false)}>
          <p class="modal-text">{pending.options.message}</p>
          <div class="modal-actions">
            <button type="button" class="btn" autofocus onClick={() => answer(false)}>
              Cancel
            </button>
            <button
              type="button"
              class={`btn ${pending.options.danger ? 'btn-danger' : 'btn-primary'}`}
              onClick={() => answer(true)}
            >
              {pending.options.confirmLabel}
            </button>
          </div>
        </Modal>
      )}
    </UIContext.Provider>
  );
}
