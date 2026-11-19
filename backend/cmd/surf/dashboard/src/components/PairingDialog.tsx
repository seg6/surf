import { useState } from 'preact/hooks';
import { useDashboard } from '../state';
import { deviceName, phraseWords } from '../lib/devices';
import { Modal } from './Modal';

/** Appears over any page when a device asks to pair. Pairing requests come from the host's candidates API. */
export function PairingDialog() {
  const { candidates, rejectCandidate } = useDashboard();
  const [rejecting, setRejecting] = useState(false);
  const candidate = candidates[0];
  if (!candidate) return null;

  const words = phraseWords(candidate.phrase);
  const reject = async () => {
    setRejecting(true);
    try {
      await rejectCandidate(candidate.id);
    } finally {
      setRejecting(false);
    }
  };

  return (
    <Modal key={candidate.id} titleId="pairing-title" title={`${deviceName(candidate)} wants to pair`}>
      <p class="modal-text">Check that the device shows these six words, then confirm on the device.</p>
      {words.length ? (
        <ol class="words" aria-label="Verification words">
          {words.map((word, index) => (
            <li key={`${index}-${word}`}>{word}</li>
          ))}
        </ol>
      ) : (
        <p class="modal-text">Verification words are unavailable.</p>
      )}
      <div class="modal-actions modal-split">
        <span class="muted">
          {candidates.length > 1 ? `Waiting for the device. ${candidates.length - 1} more waiting.` : 'Waiting for the device…'}
        </span>
        <button type="button" class="btn btn-danger" autofocus disabled={rejecting} onClick={() => void reject()}>
          Reject
        </button>
      </div>
    </Modal>
  );
}
