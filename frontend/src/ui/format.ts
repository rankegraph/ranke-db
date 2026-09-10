/**
 * package: ui / format
 * type:    logic
 * job:     write numbers and instants the way a reader reads them
 * limits:  presentation only; the values are core's (-> core)
 *
 * Formatting is the interface's business, so it lives here rather than in core — but in a
 * plain module rather than a component, so it can be tested without a DOM.
 */

/**
 * formatInstant writes an instant at the precision the visible span justifies. Reading
 * milliseconds across a decade says nothing, and reading a bare date across one minute says
 * nothing either.
 */
export function formatInstant(at: number, span: number): string {
  if (!Number.isFinite(at)) return '—';
  const iso = new Date(at).toISOString();
  if (span < 2_000) return iso.slice(11, 23);
  if (span < 2 * 60_000) return iso.slice(11, 19);
  if (span < 2 * 86_400_000) return iso.slice(11, 16);
  if (span < 60 * 86_400_000) return `${iso.slice(5, 10)} ${iso.slice(11, 16)}`;
  if (span < 3 * 365 * 86_400_000) return iso.slice(0, 10);
  return iso.slice(0, 7);
}

/**
 * formatBytes writes a content size. Claims and edges both declare one, and a read that left the
 * bytes where they are still says how many there were — so an absent size is a different fact
 * from a size of zero.
 */
export function formatBytes(size: number | undefined): string {
  if (size === undefined) return '—';
  if (size < 1024) return `${size} B`;
  if (size < 1048576) return `${(size / 1024).toFixed(1)} KiB`;
  return `${(size / 1048576).toFixed(1)} MiB`;
}

/**
 * contentSummary is a record's content declaration on one line: size, media type, and whether
 * the bytes are inline in the record or external in the Universe.
 */
export function contentSummary(
  kind: string,
  size: number | undefined,
  encoding: string | undefined,
): string {
  if (kind === 'none') return 'none';
  return `${formatBytes(size ?? 0)} · ${encoding || 'no encoding declared'} · ${kind}`;
}

/**
 * inlineLabel writes a caption on one line. A caption is drawn on the canvas as two — what the
 * claim is, then what it says — and a pane row has neither the room nor the reason for that.
 */
export function inlineLabel(label: string): string {
  return label.split('\n').filter((line) => line.length > 0).join(' · ');
}

/**
 * formatZoom writes a stretch factor as a reader would say it. The range runs over six orders of
 * magnitude, so the precision follows the number rather than being fixed.
 */
export function formatZoom(stretch: number): string {
  if (!Number.isFinite(stretch) || stretch <= 0) return '—';
  if (stretch >= 1000) return `${Math.round(stretch / 1000)}k`;
  if (stretch >= 10) return stretch.toFixed(0);
  if (stretch >= 1) return stretch.toFixed(1);
  return stretch.toFixed(2);
}

/**
 * formatExact writes an instant in full, to the millisecond. The ruler's labels are a scale
 * and shed precision the span does not justify; the cursor is a readout of one claim, so it
 * says exactly when — which is the only reason to point at a claim and ask.
 */
export function formatExact(at: number): string {
  if (!Number.isFinite(at)) return '—';
  const iso = new Date(at).toISOString();
  return `${iso.slice(0, 10)} ${iso.slice(11, 23)}`;
}

/** How many bytes a hex dump shows per line. */
const HEX_COLUMNS = 16;

/**
 * hexDump writes bytes the way a reader inspects them: offset, hex, and the printable ASCII
 * beside it. For content whose encoding says nothing about how to read it, this is the only
 * honest rendering — a byte is a byte.
 */
export function hexDump(bytes: Uint8Array): string {
  const lines: string[] = [];
  for (let at = 0; at < bytes.length; at += HEX_COLUMNS) {
    const row = bytes.subarray(at, at + HEX_COLUMNS);
    const hex = [...row].map((b) => b.toString(16).padStart(2, '0')).join(' ');
    const ascii = [...row].map((b) => (b >= 0x20 && b < 0x7f ? String.fromCharCode(b) : '·')).join('');
    lines.push(`${at.toString(16).padStart(6, '0')}  ${hex.padEnd(HEX_COLUMNS * 3 - 1)}  ${ascii}`);
  }
  return lines.join('\n');
}

// Whether bytes are text, and how they decode, is a fact about content rather than about the
// interface reading it — so both live with the bytes, re-exported here for the panes.
export { asText, isTextual } from '../core/content.ts';
