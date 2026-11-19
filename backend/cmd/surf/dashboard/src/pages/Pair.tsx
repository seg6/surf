import { useDashboard } from '../state';

export function Pair() {
  const { session, pairingBusy, config, copy, cancelPairing } = useDashboard();
  const code = String(session?.code || '');
  const address = String(session?.publicAddress || config?.publicAddress || config?.lanURL || '').trim();
  const loading = pairingBusy && !session;

  return (
    <div class="panel panel-pad pair">
      {session?.qrPNG ? (
        <img class="qr" src={`data:image/png;base64,${session.qrPNG}`} alt="Pairing QR code" width="180" height="180" />
      ) : (
        <div class="qr qr-empty" aria-hidden="true" />
      )}
      <div class="pair-main">
        <p class="lede-inline">
          Scan the code with a Surf app, or enter the address and code on the device. The code works once, and five
          wrong attempts cancel it.
        </p>
        <dl class="pair-facts">
          <div>
            <dt>Code</dt>
            <dd class="code" aria-live="polite">{code || (loading ? '------' : '—')}</dd>
          </div>
          <div>
            <dt>Address</dt>
            <dd class="address">
              {address || (loading ? 'Loading…' : 'No reachable address detected')}
              {address && (
                <button type="button" class="btn btn-small" onClick={() => void copy(address)}>
                  Copy
                </button>
              )}
            </dd>
          </div>
        </dl>
        {!loading && !address && (
          <p class="muted">Set a public address under Connection, save, and try again.</p>
        )}
        <div class="actions">
          <button type="button" class="btn" onClick={() => void cancelPairing()}>
            Cancel Invitation
          </button>
        </div>
      </div>
    </div>
  );
}
