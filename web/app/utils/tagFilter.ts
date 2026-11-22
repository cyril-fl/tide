/** The tags a tag page filters on: its notes carry every tag of and, none of not. */
export type TagFilter = { and: string[]; not: string[] };

function list(value: unknown) {
  return typeof value === 'string' && value ? value.split(',') : [];
}

/** The filter of a route: /tags/a?and=b,c&not=d, the first included tag in the path, /tags?not=d with none. */
export function readTagFilter(route: {
  path: string;
  params: Record<string, unknown>;
  query: Record<string, unknown>;
}) {
  if (!route.path.startsWith('/tags')) return { and: [], not: [] };
  const value = route.params.value;
  return {
    and: [...(typeof value === 'string' && value ? [value] : []), ...list(route.query.and)],
    not: list(route.query.not),
  };
}

/** A click on a tag of the sidebar: off, then included, then excluded, then off again. */
export function nextTagFilter(filter: TagFilter, value: string): TagFilter {
  if (filter.and.includes(value))
    return { and: filter.and.filter((tag) => tag !== value), not: [...filter.not, value] };
  if (filter.not.includes(value)) return { and: filter.and, not: filter.not.filter((tag) => tag !== value) };
  return { and: [...filter.and, value], not: filter.not };
}

/** Where a filter is shown; an empty one leaves the tags for the notes. */
export function tagFilterPath({ and: [first, ...rest], not }: TagFilter) {
  if (!first && !not.length) return '/';
  const query: Record<string, string> = {};
  if (rest.length) query.and = rest.join(',');
  if (not.length) query.not = not.join(',');
  return { path: first ? `/tags/${encodeURIComponent(first)}` : '/tags', query };
}
