/**
 * package: ui / shell
 * type:    view
 * job:     show the logo, the essential tools, and the badge naming the current source
 * limits:  presentation; the source is the connection store's (-> core/connections)
 *
 * Running a read belongs to the Query tab: a query has parameters a toolbar button
 * cannot carry. The pick/select/drag tools are declared but disabled until core has
 * interaction modes — inert is more honest than pretending.
 */

// The markup rather than a URL, so the topbar can hide the plate the tab icon needs.
import icon from '../icon.svg?raw';
import { useConnections } from '../../core/connections.ts';
import { discoverScopes, selectScope, showAll } from '../../core/session.ts';
import { axisXOf, instantAtX, stepFrom, timeEnds } from '../../core/timeline.ts';
import { fitHeight, graphXAt, panTo } from '../../render/camera.ts';
import { canvasWidth } from '../../render/instances.ts';
import { scopeOptions } from '../../core/scope.ts';
import { activeView, useExplorer } from '../../core/store.ts';
import { VERSION } from '../../version.ts';

/**
 * The navigation tools. Each is one click for something the wheel can only approximate, and
 * `show all` is the way back from anywhere — which is what makes zooming freely comfortable.
 */
const TOOLS: { id: string; glyph: string; label: string; run: () => void }[] = [
  { id: 'all', glyph: '⤢', label: 'Show all — the whole archive, both axes', run: showAll },
  { id: 'height', glyph: '⇕', label: 'Fit height — the strata, leaving time as it is', run: fitHeight },
  { id: 'oldest', glyph: '⇤', label: 'Jump to the oldest claim', run: () => jumpTo('from') },
  { id: 'prev', glyph: '◂', label: 'Step back one claim in time', run: () => step(-1) },
  { id: 'next', glyph: '▸', label: 'Step forward one claim in time', run: () => step(1) },
  { id: 'newest', glyph: '⇥', label: 'Jump to the newest claim', run: () => jumpTo('to') },
];

/**
 * step moves to the next claim along from the middle of the view. The centre rather than an edge,
 * because that is where a reader is looking, and stepping is for a sparse stretch where the next
 * claim may be off screen.
 */
function step(direction: 1 | -1): void {
  const width = canvasWidth();
  if (width <= 0) return;
  const here = graphXAt(width / 2);
  if (here === null) return;
  const at = instantAtX(here);
  if (at === null) return;
  const next = stepFrom(at, direction);
  if (next === null) return;
  const x = axisXOf(next);
  if (x !== null) panTo(x);
}

/** goTo pans to an instant a reader typed. */
function goTo(value: string): void {
  // The input has no zone, and every label in this client is UTC, so it is read as UTC.
  const at = Date.parse(`${value}Z`);
  if (!Number.isFinite(at)) return;
  const x = axisXOf(at);
  if (x !== null) panTo(x);
}

/** forInput writes an instant as the value a datetime-local input takes. */
function forInput(at: number): string {
  return new Date(at).toISOString().slice(0, 16);
}

/** jumpTo pans to one end of the archive in time. */
function jumpTo(end: 'from' | 'to'): void {
  const ends = timeEnds();
  if (!ends) return;
  const x = axisXOf(ends[end]);
  if (x !== null) panTo(x);
}

/**
 * JumpToDate pans to an instant, bounded by the archive's own ends so a date it does not cover
 * cannot be asked for. In UTC, as every other time in this client is.
 */
function JumpToDate() {
  const ends = timeEnds();
  if (!ends || ends.from === ends.to) return null;
  return (
    <input
      type="datetime-local"
      className="jump-date"
      aria-label="Jump to a date (UTC)"
      title={`jump to a date · ${forInput(ends.from)} to ${forInput(ends.to)} UTC`}
      min={forInput(ends.from)}
      max={forInput(ends.to)}
      onChange={(e) => goTo(e.target.value)}
    />
  );
}

/**
 * ScopePicker offers the scopes the archive holds. Selecting one narrows the view through
 * a reducer, so it costs a re-index rather than a read.
 *
 * A scope whose head never loaded is marked rather than left to draw as an empty view.
 */
function ScopePicker() {
  const scopes = useExplorer((s) => s.scopes);
  const view = useExplorer(activeView);
  const selected = view?.scope?.name ?? '';

  if (scopes.state === 'unknown' || scopes.state === 'error') {
    // An action, not a status: what went wrong is reported on the canvas.
    return (
      <div className="scope-picker">
        <button
          type="button"
          className="scope-load"
          onClick={() => void discoverScopes()}
          title="ask the archive again which branches it holds"
        >
          retry branches
        </button>
      </div>
    );
  }

  // While a listing or a read is in flight the picker stays put and goes inert. Progress
  // belongs on the canvas, where there is room to say which stage it is in.
  return (
    <div className="scope-picker">
      <select
        className="scope-select"
        value={selected}
        aria-label="Scope"
        disabled={scopes.state === 'loading'}
        onChange={(e) => {
          const next = scopes.scopes.find((s) => s.name === e.target.value) ?? null;
          void selectScope(next);
        }}
      >
        {scopeOptions(scopes.scopes, view?.scope ?? null).map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </div>
  );
}

export function Header() {
  const { connections, activeId } = useConnections();
  const active = connections.find((c) => c.id === activeId) ?? connections[0];
  const setSidePane = useExplorer((s) => s.setSidePane);

  return (
    <header className="topbar">
      <div className="brand">
        <span className="brand-mark" aria-hidden="true" dangerouslySetInnerHTML={{ __html: icon }} />
        <span className="brand-text">
          <span className="brand-name">Ranke Explorer</span>
          <span className="brand-version" title="this explorer build">{VERSION}</span>
        </span>
      </div>

      <div className="toolbar" role="toolbar" aria-label="Tools">
        {TOOLS.map((tool) => (
          <button
            key={tool.id}
            type="button"
            className="tool"
            onClick={tool.run}
            title={tool.label}
            aria-label={tool.label}
          >
            {tool.glyph}
          </button>
        ))}
      </div>

      <JumpToDate />

      <ScopePicker />

      <div className="topbar-spacer" />

      {/* The badge names the current source and is the way to change it. */}
      <button
        type="button"
        className="connection-chip"
        onClick={() => setSidePane('server')}
        title={
          active?.kind === 'mock'
            ? 'generated locally — no server. Click to configure sources.'
            : `${active?.baseUrl ?? ''} — click to configure sources`
        }
      >
        <span className={`dot ${active ? 'state-ok' : 'state-unknown'}`} aria-hidden="true" />
        {active ? active.name || active.baseUrl || 'mock' : 'no source'}
      </button>
    </header>
  );
}
