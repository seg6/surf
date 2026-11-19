import type { ComponentChildren } from 'preact';
import { useEffect, useRef } from 'preact/hooks';

interface ModalProps {
  /** Id of the element inside that titles the dialog. */
  titleId: string;
  title: string;
  /** Called when the user presses Escape. Omit to make Escape do nothing. */
  onCancel?: () => void;
  children: ComponentChildren;
}

/**
 * A titled dialog built on the native modal element, which traps focus, makes
 * the page behind it inert, and gives focus back to the opener when it closes.
 * The control that should receive focus first carries the `autofocus` attribute.
 */
export function Modal({ titleId, title, onCancel, children }: ModalProps) {
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return undefined;
    if (!dialog.open) dialog.showModal();
    return () => {
      if (dialog.open) dialog.close();
    };
  }, []);

  return (
    <dialog
      ref={ref}
      class="modal"
      role="alertdialog"
      aria-labelledby={titleId}
      onCancel={(event) => {
        event.preventDefault();
        onCancel?.();
      }}
    >
      <div class="modal-title">
        <h2 id={titleId}>{title}</h2>
      </div>
      <div class="modal-body">{children}</div>
    </dialog>
  );
}
