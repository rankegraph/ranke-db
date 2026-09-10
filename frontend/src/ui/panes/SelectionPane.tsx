/**
 * package: ui / panes
 * type:    view
 * job:     show the claim the user clicked
 * limits:  presentation; selection state is the store's (-> core/store)
 *
 * The selection pane: the claim the user clicked.
 *
 * It reads `selection.selected` and never `selection.hovered` — hover drives only a
 * cheap preview, so sweeping the pointer across a thousand nodes must not re-render
 * anything here.
 */

import { useEffect } from 'react';
import { shortId } from '../../core/claims.ts';
import { claimDetail, fetchContent, openClaimCbor } from '../../core/session.ts';
import { CONTENT_LIMIT } from '../../core/data/source.ts';
import type { Reference } from '../../core/session.ts';
import { useExplorer } from '../../core/store.ts';
import { revealClaim } from '../../render/renderer.ts';
import { Button, Empty, ExtensionFields, KeyValue, PaneTitle } from '../components/Field.tsx';
import { asText, contentSummary, formatBytes, hexDump, inlineLabel, isTextual } from '../format.ts';

/**
 * ContentBlock shows the claim's bytes, read as text where the encoding says they are text
 * and as a hex dump where it does not. A size on its own says nothing about a note.
 */
function ContentBlock() {
  const content = useExplorer((s) => s.content);
  const selected = useExplorer((s) => s.selection.selected);
  if (!content || content.id !== selected) return null;

  return (
    <>
      <h2>content</h2>
      {contentBody(content)}
    </>
  );
}

/** contentBody is what to show for each state the read can be in. */
function contentBody(content: NonNullable<ReturnType<typeof useExplorer.getState>['content']>) {
  switch (content.state) {
    case 'none':
      return <Empty>This claim carries no content.</Empty>;
    case 'loading':
      return <p className="note">reading {formatBytes(content.size)}…</p>;
    case 'too-large':
      return (
        <p className="note">
          {formatBytes(content.size)} — too much to show. The limit is{' '}
          {formatBytes(CONTENT_LIMIT)}, and the bytes are left where they are rather than fetched.
        </p>
      );
    case 'error':
      return <p className="note content-error">{content.error}</p>;
    case 'ready': {
      const data = content.bytes ?? new Uint8Array();
      const text = isTextual(content.encoding);
      return (
        <>
          <p className="note content-about">
            {text ? '' : ' — shown as bytes, the encoding naming no way to read them'}
          </p>
          <pre className={`content-body${text ? '' : ' is-hex'}`}>
            {text ? asText(data) : hexDump(data)}
          </pre>
        </>
      );
    }
  }
}

/** How many rows a list shows before it says how many it is not showing. */
const ROW_LIMIT = 40;

/**
 * RefList draws one side of a claim's edges. The arrow says which way the edge points, since the
 * two lists are the same rows read from opposite ends.
 */
function RefList({ rows, arrow, empty }: { rows: Reference[]; arrow: string; empty: string }) {
  if (rows.length === 0) return <Empty>{empty}</Empty>;
  return (
    <>
      <ul className="refs">
        {rows.slice(0, ROW_LIMIT).map((ref) => (
          <li key={ref.edge}>
            <button
              type="button"
              className="ref-edge"
              onClick={() => useExplorer.getState().selectEdge(ref.edge)}
              title="show this edge"
            >
              {ref.edgeType || '—'}
            </button>
            <span className="ref-arrow" aria-hidden="true">
              {arrow}
            </span>
            <button
              type="button"
              className="ref-id"
              onClick={() => revealClaim(ref.id)}
              title={ref.id}
            >
              <span className="ref-type">{ref.claimType || '—'}</span>
              <code>{shortId(ref.id)}</code>
            </button>
          </li>
        ))}
      </ul>
      {rows.length > ROW_LIMIT ? (
        <p className="note">
          {(rows.length - ROW_LIMIT).toLocaleString('en-US')} further rows not listed.
        </p>
      ) : null}
    </>
  );
}

export function SelectionPane() {
  const selected = useExplorer((s) => s.selection.selected);
  const detail = selected ? claimDetail(selected) : null;

  // Content is read on selection: it is the substance of most claims, and a size alone
  // answers nothing. Immutability makes the cache behind this safe, so re-selecting is free.
  useEffect(() => {
    if (selected) void fetchContent(selected);
  }, [selected]);

  if (!detail) {
    return (
      <div className="pane">
        <PaneTitle>claim</PaneTitle>
        <Empty>Click a claim to inspect it.</Empty>
      </div>
    );
  }

  return (
    <div className="pane">
      <PaneTitle hint={detail.claimType}>claim</PaneTitle>
      <KeyValue
        rows={[
          ['id', <code className="claim-id">{detail.id}</code>],
          ['type', detail.claimType || '—'],
          ['label', detail.label ? inlineLabel(detail.label) : '—'],
          ['contribution', detail.contribution.toLocaleString('en-US')],
          ['height', detail.height?.toLocaleString('en-US') ?? '—'],
          // As the claim states it, precision and all — never reformatted.
          [
            'created',
            detail.createdAtIso || new Date(detail.createdAt).toISOString(),
          ],
          ['content', contentSummary(detail.contentKind, detail.contentSize, detail.encoding)],
          ...(detail.contentHash
            ? ([['content hash', <code className="claim-id">{detail.contentHash}</code>]] as [
                string,
                React.ReactNode,
              ][])
            : []),
          ['degree', detail.degree.toLocaleString('en-US')],
          ['referenced by', detail.citedBy.toLocaleString('en-US')],
        ]}
      />

      <ExtensionFields fields={detail.fields} />

      <div className="row">
        <Button onClick={() => openClaimCbor(detail.id)}>show claim CBOR</Button>
      </div>

      <ContentBlock />

      <h2>references</h2>
      <p className="note">
        What this claim references — its own edges. Both halves of a row are things to ask about: the
        edge on the left, the claim it points at on the right.
      </p>
      <RefList rows={detail.references} arrow="→" empty="An initial node — it references nothing." />

      <h2>referenced by</h2>
      <p className="note">
        What references this claim — edges belonging to other claims, drawn in pink while this one
        is selected. A claim cannot know them when it is written, so they accrue.
      </p>
      <RefList rows={detail.citations} arrow="←" empty="Nothing loaded references this claim." />
    </div>
  );
}
