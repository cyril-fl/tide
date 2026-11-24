/** The note to open when the tab of the open one is closed: the tab before it, else the one after. */
export function tabAfterClose(tabs: string[], closed: string): string | undefined {
  const index = tabs.indexOf(closed);
  if (index < 0) return undefined;
  return tabs[index - 1] ?? tabs[index + 1];
}

/** The tabs once a note not in them opens: it replaces the tab of the note shown, or opens a tab after it when asked
 * (fresh) or when no tab shows a note. */
export function placeTab<T extends { id: string }>(tabs: T[], shown: string, tab: T, fresh: boolean): T[] {
  const at = tabs.findIndex((open) => open.id === shown);
  if (at < 0) return [...tabs, tab];
  return [...tabs.slice(0, fresh ? at + 1 : at), tab, ...tabs.slice(at + 1)];
}

/** The tabs with the one at from dropped on the place of the one at to. */
export function moveTab<T>(tabs: T[], from: number, to: number): T[] {
  const moved = [...tabs];
  moved.splice(to, 0, ...moved.splice(from, 1));
  return moved;
}

/** The tabs of a panel and the note it shows. */
export type Panel<T> = { tabs: T[]; active: string };

/** The panel without the tab of a note: the note it showed gives way to its neighbour (tabAfterClose), or to none. */
export function withoutTab<T extends { id: string }>(panel: Panel<T>, id: string): Panel<T> {
  if (!panel.tabs.some((tab) => tab.id === id)) return panel;
  const ids = panel.tabs.map((tab) => tab.id);
  const active = panel.active === id ? (tabAfterClose(ids, id) ?? '') : panel.active;
  return { tabs: panel.tabs.filter((tab) => tab.id !== id), active };
}

/** The panel showing a note: in its tab, else in one placed as in the first panel (placeTab). */
export function openTab<T extends { id: string }>(panel: Panel<T>, tab: T, fresh: boolean): Panel<T> {
  const open = panel.tabs.some((other) => other.id === tab.id);
  return { tabs: open ? panel.tabs : placeTab(panel.tabs, panel.active, tab, fresh), active: tab.id };
}

/** A panel of the editor: its id stays while it lives, its editor and its width keyed on it. */
export type NotePanel<T> = Panel<T> & { id: string };
/** The panels of the editor, left to right (top to bottom when stacked), and the one the address shows. */
export type Layout<T> = { panels: NotePanel<T>[]; current: string };

/** The panels that keep a tab or are the only one left: an emptied panel goes, its neighbour taking the current one. */
function prune<T>(layout: Layout<T>): Layout<T> {
  const panels = layout.panels.filter((panel) => panel.tabs.length);
  if (!panels.length) return { panels: layout.panels.slice(0, 1), current: layout.panels[0]?.id ?? '' };
  if (panels.some((panel) => panel.id === layout.current)) return { panels, current: layout.current };
  const at = layout.panels.findIndex((panel) => panel.id === layout.current);
  const before = layout.panels
    .slice(0, at)
    .reverse()
    .find((panel) => panels.includes(panel));
  return { panels, current: (before ?? panels[0]!).id };
}

/** The layout with a note shown in a panel, in its tab or one placed there (openTab); its tab elsewhere moves here: a
 * note has one tab (#62). */
export function showIn<T extends { id: string }>(
  layout: Layout<T>,
  panelId: string,
  tab: T,
  fresh: boolean,
): Layout<T> {
  const moved = layout.panels.flatMap((panel) => panel.tabs).find((open) => open.id === tab.id) ?? tab;
  const panels = layout.panels.map((panel) =>
    panel.id === panelId ? { ...panel, ...openTab(panel, moved, fresh) } : { ...panel, ...withoutTab(panel, tab.id) },
  );
  return prune({ ...layout, panels });
}
/** The layout without the tab of a note in every panel: an emptied panel goes, unless it is the last. */
export function closeIn<T extends { id: string }>(layout: Layout<T>, ids: string[]): Layout<T> {
  const panels = layout.panels.map((panel) => ({ ...panel, ...ids.reduce(withoutTab, panel as Panel<T>) }));
  return prune({ ...layout, panels });
}

/** The layout with a note in a new panel at an index, taken from where it was. */
export function splitAt<T extends { id: string }>(layout: Layout<T>, tab: T, at: number, id: string): Layout<T> {
  const moved = layout.panels.flatMap((panel) => panel.tabs).find((open) => open.id === tab.id) ?? tab;
  const panels = [...layout.panels];
  panels.splice(at, 0, { id, tabs: [moved], active: moved.id });
  const rest = panels.map((panel) => (panel.id === id ? panel : { ...panel, ...withoutTab(panel, tab.id) }));
  return prune({ ...layout, panels: rest });
}

/** The layout on a screen that holds max panels: the tabs of those that no longer fit go to the last one kept. */
export function fitPanels<T extends { id: string }>(layout: Layout<T>, max: number): Layout<T> {
  if (layout.panels.length <= max) return layout;
  const kept = layout.panels.slice(0, max);
  const last = kept[max - 1]!;
  const tabs = [...last.tabs, ...layout.panels.slice(max).flatMap((panel) => panel.tabs)];
  kept[max - 1] = { ...last, tabs, active: last.active || (tabs[0]?.id ?? '') };
  return { panels: kept, current: kept.some((panel) => panel.id === layout.current) ? layout.current : last.id };
}

/** Where "open beside" goes from a panel: the panel after it, a new one at the end when there is room, else the one
 * before it; undefined when the screen holds a single panel. */
export function besideOf<T>(
  layout: Layout<T>,
  from: string,
  max: number,
): { panel: string } | { at: number } | undefined {
  const at = layout.panels.findIndex((panel) => panel.id === from);
  const after = layout.panels[at + 1];
  if (after) return { panel: after.id };
  if (layout.panels.length < max) return { at: layout.panels.length };
  const before = layout.panels[at - 1];
  return before ? { panel: before.id } : undefined;
}
