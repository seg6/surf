import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import { useDashboard } from '../state';
import { useUI } from '../components/ui';
import { api } from '../lib/api';
import {
  applyLogUpdate, buildLogSources, filterLogRecords, formatLogTime, logQuery, logRecordParts,
} from '../lib/logs';
import type { LogRecord, LogSource, LogUpdate } from '../lib/types';

type Live = 'paused' | 'connecting' | 'live' | 'unavailable';

const LIVE_TEXT: Record<Live, string> = {
  paused: 'Paused', connecting: 'Connecting', live: 'Live', unavailable: 'Unavailable',
};

function LogLine({ record }: { record: LogRecord }) {
  const parts = logRecordParts(record);
  return (
    <div class="log-line">
      <span class="log-time" title={parts.timestamp}>{formatLogTime(parts.timestamp)}</span>
      <span class={`log-level log-level-${parts.level.toLowerCase()}`}>{parts.level}</span>
      <span class="log-component">{parts.component}</span>
      <span class="log-message">{parts.message}</span>
      {parts.fields && <span class="log-fields">{parts.fields}</span>}
    </div>
  );
}

export function Logs() {
  const { copy } = useDashboard();
  const { toast, confirm } = useUI();
  const [sources, setSources] = useState<LogSource[]>([]);
  const [selectedID, setSelectedID] = useState('server');
  const [records, setRecords] = useState<LogRecord[]>([]);
  const [query, setQuery] = useState('');
  const [live, setLive] = useState<Live>('paused');
  const [error, setError] = useState('');
  const [clearing, setClearing] = useState(false);
  const output = useRef<HTMLDivElement>(null);
  const stick = useRef(true);

  const selected = sources.find((source) => source.id === selectedID) ?? sources[0];
  const visible = filterLogRecords(records, query);

  // Load the source list once when the page opens.
  useEffect(() => {
    let cancelled = false;
    api.logSources().then(
      (payload) => {
        if (!cancelled) setSources(buildLogSources(payload));
      },
      (failure: unknown) => {
        if (cancelled) return;
        setError(failure instanceof Error ? failure.message : String(failure));
        setLive('unavailable');
      },
    );
    return () => {
      cancelled = true;
    };
  }, []);

  // Stream the chosen source. Leaving the page or changing source closes it.
  const selectedKey = selected?.id;
  useEffect(() => {
    if (!selected) return undefined;
    setRecords([]);
    stick.current = true;
    setLive('connecting');
    const stream = new EventSource(api.logStreamURL(logQuery(selected)));
    stream.onopen = () => setLive('live');
    stream.onmessage = (event) => {
      const update = JSON.parse(event.data as string) as LogUpdate;
      setRecords((current) => applyLogUpdate(current, update));
    };
    stream.onerror = () => setLive('connecting');
    return () => {
      stream.close();
      setLive('paused');
    };
    // The stream depends on which source is selected, not on the list object.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedKey]);

  // Follow new output only while the reader is already at the bottom.
  const onScroll = () => {
    const node = output.current;
    if (node) stick.current = node.scrollHeight - node.scrollTop - node.clientHeight < 48;
  };
  useLayoutEffect(() => {
    const node = output.current;
    if (node && stick.current) node.scrollTop = node.scrollHeight;
  }, [visible.length]);

  const copyJSON = async () => {
    if (!visible.length) {
      toast('No log records to copy');
      return;
    }
    const label = selected?.label ?? 'log';
    const filtered = query.trim() ? ' filtered' : '';
    await copy(
      JSON.stringify(visible, null, 2),
      `Copied ${visible.length}${filtered} ${label} log record${visible.length === 1 ? '' : 's'} as JSON`,
    );
  };

  const clear = async () => {
    if (!selected) return;
    const ok = await confirm({
      title: `Clear ${selected.label} logs?`,
      message: 'The saved log lines for this source are deleted.',
      confirmLabel: 'Clear',
      danger: true,
    });
    if (!ok) return;
    setClearing(true);
    try {
      const result = await api.clearLogs(logQuery(selected));
      setRecords([]);
      stick.current = true;
      toast(result.pending ? 'Saved logs cleared; device will clear when it reconnects' : 'Logs cleared');
    } catch (failure) {
      toast(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setClearing(false);
    }
  };

  let placeholder = '';
  if (error) placeholder = error;
  else if (!visible.length) placeholder = query.trim() ? 'No matching log lines.' : 'Waiting for log output…';

  return (
    <>
      <div class="log-toolbar">
        <div class="segmented" role="group" aria-label="Log source">
          {sources.map((source) => (
            <button
              key={source.id}
              type="button"
              aria-pressed={source.id === selected?.id}
              onClick={() => setSelectedID(source.id)}
            >
              {source.label}
            </button>
          ))}
        </div>
        <input
          class="control search"
          type="search"
          aria-label="Filter logs"
          placeholder="Filter"
          value={query}
          onInput={(event) => setQuery(event.currentTarget.value)}
        />
        <button type="button" class="btn btn-small" onClick={() => void copyJSON()}>Copy JSON</button>
        <button type="button" class="btn btn-small btn-danger" disabled={clearing || !selected} onClick={() => void clear()}>
          Clear
        </button>
        <span class={`live-state${live === 'live' ? ' is-live' : ''}`} role="status">
          <span class="dot" aria-hidden="true" />
          {LIVE_TEXT[live]}
        </span>
      </div>
      <div class="log-output" ref={output} onScroll={onScroll} tabIndex={0} role="log" aria-label="Log output">
        {placeholder ? <p class="log-empty">{placeholder}</p> : visible.map((record, index) => <LogLine key={index} record={record} />)}
      </div>
    </>
  );
}
