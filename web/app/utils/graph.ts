/** The node of a tag: notes have uuids, no clash. The server names them the same (forcelayout.TagNode). */
export const tagNode = (tag: string) => `#${tag}`;

/**
 * The other named notes offered after [[, this one left out.
 * neighbors (Pro): the ones sharing the most tags with it first, those holding query joined to the few dozen notes the
 * API found; the sort being stable, the API's order stays between equals.
 */
export function linkTargets<N extends { id: string; title: string; path: string }>(
  notes: N[] | null | undefined,
  id: string | undefined,
  neighbors?: (N & { shared: number })[],
  query = '',
) {
  const shared = new Map(neighbors?.map((note) => [note.id, note.shared]));
  const found = new Set(notes?.map((note) => note.id));
  const near = (neighbors ?? []).filter(
    (note) => !found.has(note.id) && `${note.title}\n${note.path}`.toLowerCase().includes(query.toLowerCase()),
  );
  return [...(notes ?? []), ...near]
    .filter((note) => note.id !== id && note.title)
    .sort((a, b) => (shared.get(b.id) ?? 0) - (shared.get(a.id) ?? 0));
}
