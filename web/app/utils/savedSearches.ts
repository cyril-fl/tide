import type { SavedSearch } from '~/types/items';

/** Whether the saved search id sits in the group ancestorId, at any depth. */
export function isInside(searches: SavedSearch[], id: string, ancestorId: string) {
  let parent = searches.find((s) => s.id === id)?.parent_id;
  for (let steps = 0; parent && steps <= searches.length; steps++) {
    if (parent === ancestorId) return true;
    parent = searches.find((s) => s.id === parent)?.parent_id;
  }
  return false;
}

/** The saved searches once dragged is dropped on target: inside it, at the end, when target is a group and dragged is
 * not; else right before it, in its group. Unchanged when one is missing or target sits inside dragged. */
export function reorderSearches(searches: SavedSearch[], draggedId: string, targetId: string): SavedSearch[] {
  const from = searches.findIndex((s) => s.id === draggedId);
  if (from < 0 || !searches.some((s) => s.id === targetId) || isInside(searches, targetId, draggedId)) return searches;
  const rest = [...searches];
  const [moved] = rest.splice(from, 1);
  const target = rest.find((s) => s.id === targetId)!;
  if (target.group && !moved!.group) return [...rest, { ...moved!, parent_id: target.id }];
  // Index after the removal: it shifts when the dragged one sat before the target.
  rest.splice(rest.indexOf(target), 0, { ...moved!, parent_id: target.parent_id });
  return rest;
}
