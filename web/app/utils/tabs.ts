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
