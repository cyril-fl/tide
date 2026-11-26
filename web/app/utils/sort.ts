/** The next sort after a click on key: ascending (added last), then descending (same place), then off. */
export function cycleSort(sort: string, key: string) {
  const list = sort ? sort.split(',') : [];
  const index = list.findIndex((k) => k.replace(/^-/, '') === key);
  if (index < 0) return [...list, key].join(',');
  if (!list[index]!.startsWith('-')) return list.with(index, `-${key}`).join(',');
  return list.toSpliced(index, 1).join(',');
}

/** Where key stands in a sort: false when it is not in it. */
export function sortOf(sort: string, key: string): false | 'asc' | 'desc' {
  const found = sort.split(',').find((k) => k.replace(/^-/, '') === key);
  return found ? (found.startsWith('-') ? 'desc' : 'asc') : false;
}
