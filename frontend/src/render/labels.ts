/**
 * package: render / labels
 * type:    adapter
 * job:     draw a node's caption on the canvas
 * limits:  canvas drawing only; what the caption says is core's (-> core/claims); the hover
 *          tooltip is a DOM overlay, not canvas (-> ui/shell/App)
 *
 * Kept general over any number of lines a caption may carry, though a caption is one line today —
 * Sigma itself only draws one and stops at the first newline.
 */

import type { Settings } from 'sigma/settings';
import type { NodeDisplayData, PartialButFor } from 'sigma/types';

/** Room between the baselines of two lines, on top of the font size. */
const LINE_GAP = 2;

/**
 * How wide a caption may draw, in multiples of the label grid cell Sigma picked it in
 * (`settings.labelGridCellSize`). Sigma's grid governs *which* nodes get a label, never how much
 * room the text takes once drawn — a caption is free to run past its own cell into whatever is
 * beside it. Tying the cap to that same cell size keeps the two in step: widen the grid and
 * captions may run a little longer with it, without a second constant to retune by hand.
 */
const MAX_LABEL_WIDTH_CELLS = 1.4;

/**
 * Cells a caption may span where the node asks for the room (-> render/renderer, a provenance
 * caption). A closure is drawn with few claims and a line of what each one says, so the text is
 * the point and there is space beside it; six cells is about sixty characters at the stock font.
 */
const WIDE_LABEL_CELLS = 6;

const ELLIPSIS = '…';

type LabelData = PartialButFor<NodeDisplayData, 'x' | 'y' | 'size' | 'label' | 'color'> & {
  /** Set by the reducer on a caption whose text is worth the width (-> WIDE_LABEL_CELLS). */
  wideLabel?: boolean;
};

/** lines splits a caption into what is drawn, dropping the empties a stray newline leaves. */
function lines(label: string | undefined | null): string[] {
  if (!label) return [];
  return String(label)
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

/** face is the font Sigma was configured with, applied to the context before measuring. */
function face(context: CanvasRenderingContext2D, settings: Settings): number {
  context.font = `${settings.labelWeight} ${settings.labelSize}px ${settings.labelFont}`;
  return settings.labelSize;
}

/**
 * fitToWidth trims text to maxWidth in the context's current font, marking a cut with an
 * ellipsis — a longest-prefix binary search rather than a character count, since the two
 * captions most likely to collide (long identifiers, "vulnerability_scan" beside
 * "release_candidate") are exactly the ones a fixed character cap under- or over-trims.
 */
function fitToWidth(context: CanvasRenderingContext2D, text: string, maxWidth: number): string {
  if (context.measureText(text).width <= maxWidth) return text;
  let lo = 0;
  let hi = text.length;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (context.measureText(text.slice(0, mid) + ELLIPSIS).width <= maxWidth) lo = mid;
    else hi = mid - 1;
  }
  return lo === 0 ? ELLIPSIS : text.slice(0, lo) + ELLIPSIS;
}

/**
 * drawNodeLabel writes the caption beside its node. A block of lines is centred on the baseline
 * Sigma would have used for one, so adding a second line does not shift the first off the claim
 * it belongs to.
 */
export function drawNodeLabel(context: CanvasRenderingContext2D, data: LabelData, settings: Settings): void {
  const rows = lines(data.label);
  if (rows.length === 0) return;

  const size = face(context, settings);
  const attribute = settings.labelColor.attribute;
  context.fillStyle = attribute
    ? ((data as Record<string, unknown>)[attribute] as string) || settings.labelColor.color || '#000'
    : (settings.labelColor.color ?? '#000');

  const step = size + LINE_GAP;
  const first = data.y + size / 3 - ((rows.length - 1) * step) / 2;
  const cells = data.wideLabel ? WIDE_LABEL_CELLS : MAX_LABEL_WIDTH_CELLS;
  const maxWidth = settings.labelGridCellSize * cells;
  rows.forEach((row, i) =>
    context.fillText(fitToWidth(context, row, maxWidth), data.x + data.size + 3, first + i * step),
  );
}
