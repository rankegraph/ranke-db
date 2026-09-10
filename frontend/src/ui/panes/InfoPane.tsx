/**
 * package: ui / panes
 * type:    view
 * job:     answer about whatever is selected — an edge, a claim, or the graph
 * limits:  presentation; what is selected lives in the store (-> core/store)
 *
 * One pane rather than three tabs to choose between: the question "what is this" has one
 * answer at a time, and which thing it is about is the selection rather than a tab.
 */

import { shortId } from '../../core/claims.ts';
import { edgeDetail } from '../../core/session.ts';
import { useExplorer } from '../../core/store.ts';
import { revealClaim, walkHistory } from '../../render/renderer.ts';
import { Empty, ExtensionFields, KeyValue, PaneTitle } from '../components/Field.tsx';
import { contentSummary, inlineLabel } from '../format.ts';
import { GraphPane } from './GraphPane.tsx';
import { SelectionPane } from './SelectionPane.tsx';

export function InfoPane() {
  const selected = useExplorer((s) => s.selection.selected);
  const selectedEdge = useExplorer((s) => s.selection.selectedEdge);

  return (
    <>
      <HistoryNav />
      {selectedEdge ? <EdgeInfo edgeKey={selectedEdge} /> : null}
      {!selectedEdge && selected ? <SelectionPane /> : null}
      {!selectedEdge && !selected ? <GraphPane /> : null}
    </>
  );
}

/**
 * HistoryNav walks the trail of what the reader has asked about. Following a reference is how a
 * graph is read, and a reader three references deep needs the way back — the canvas cannot offer
 * it, since what they came from may be nowhere near what they are looking at.
 */
function HistoryNav() {
  const history = useExplorer((s) => s.history);
  const back = history.at > 0;
  const forward = history.at >= 0 && history.at < history.visits.length - 1;

  return (
    <nav className="history-nav" aria-label="selection history">
      <button
        type="button"
        className="history-step"
        onClick={() => walkHistory(-1)}
        disabled={!back}
        title="back to what you were looking at"
      >
        ←
      </button>
      <button
        type="button"
        className="history-step"
        onClick={() => walkHistory(1)}
        disabled={!forward}
        title="forward again"
      >
        →
      </button>
      <span className="history-at">
        {history.visits.length === 0 ? '' : `${history.at + 1} / ${history.visits.length}`}
      </span>
    </nav>
  );
}

/**
 * EdgeInfo shows one edge: its type, and the claims at either end. The direction is the
 * substance — an edge runs from the claim that owns it to the claim it references — so both
 * ends are named and the
 * arrow between them is spelled out rather than implied.
 */
function EdgeInfo({ edgeKey }: { edgeKey: string }) {
  const detail = edgeDetail(edgeKey);

  if (!detail) {
    return (
      <div className="pane">
        <PaneTitle>edge</PaneTitle>
        <Empty>That edge is no longer in the graph.</Empty>
      </div>
    );
  }

  return (
    <div className="pane">
      <PaneTitle hint={detail.edgeType}>edge</PaneTitle>
      <KeyValue
        rows={[
          ['type', detail.edgeType || '—'],
          // An edge carries content of its own, exactly as a claim does — the declaration
          // travels whether or not a read carried the bytes.
          ['content', contentSummary(detail.contentKind, detail.contentSize, detail.encoding)],
          ...(detail.contentHash
            ? ([['content hash', <code className="claim-id">{detail.contentHash}</code>]] as [
                string,
                React.ReactNode,
              ][])
            : []),
          ...(detail.direction === 0
            ? []
            : ([['direction', detail.direction > 0 ? 'from (+1)' : 'to (−1)']] as [string, string][])),
        ]}
      />

      <ExtensionFields fields={detail.fields} />

      <h2>owned by</h2>
      <EdgeEnd
        id={detail.from}
        label={detail.fromLabel}
        claimType={detail.fromType}
        onSelect={() => revealClaim(detail.from)}
      />

      <h2>references</h2>
      <EdgeEnd
        id={detail.to}
        label={detail.toLabel}
        claimType={detail.toType}
        onSelect={() => revealClaim(detail.to)}
      />

      <p className="note">
        The edge belongs to the claim it points from — that claim created it — so its
        provenance is read there.
      </p>
    </div>
  );
}

/** EdgeEnd names one end of an edge and offers to select it. */
function EdgeEnd({
  id,
  label,
  claimType,
  onSelect,
}: {
  id: string;
  label: string;
  claimType: string;
  onSelect: () => void;
}) {
  return (
    <div className="edge-end">
      <span className="ref-type">{claimType || '—'}</span>
      <button type="button" className="ref-id" onClick={onSelect} title={id}>
        {label ? inlineLabel(label) : shortId(id)}
      </button>
    </div>
  );
}
