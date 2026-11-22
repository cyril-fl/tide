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
