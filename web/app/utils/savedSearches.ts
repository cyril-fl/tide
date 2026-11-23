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
