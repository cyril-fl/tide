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

/** How a smart folder keeps a tag: the notes carry it, carry one of the "any" tags, or do not carry it. */
export type TagState = 'all' | 'any' | 'not';
export type TagChip = { id: string; state: TagState };

/** A click on a tag of a smart folder: must have, then one of, then without, then must have again. */
export const nextTagState: Record<TagState, TagState> = { all: 'any', any: 'not', not: 'all' };

/** The tags of a saved search; tag_ids are one of them unless tag_mode is "all" (the API's default is "any"). */
export function toTagChips(search?: Partial<SavedSearch>): TagChip[] {
  const state: TagState = search?.tag_mode === 'all' ? 'all' : 'any';
  return [
    ...(search?.tag_ids ?? []).map((id) => ({ id, state })),
    ...(search?.any_tag_ids ?? []).map((id) => ({ id, state: 'any' as const })),
    ...(search?.no_tag_ids ?? []).map((id) => ({ id, state: 'not' as const })),
  ];
}

/** The tags as the API saves them: a AND (b OR c) without d is tag_ids [a], any_tag_ids [b, c], no_tag_ids [d]. */
export function tagCriteria(chips: TagChip[]) {
  const ids = (state: TagState) => chips.filter((c) => c.state === state).map((c) => c.id);
  return { tag_ids: ids('all'), tag_mode: 'all', any_tag_ids: ids('any'), no_tag_ids: ids('not') };
}
