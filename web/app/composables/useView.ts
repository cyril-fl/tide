// One choice for every view, kept across reloads.
export const useView = createGlobalState(() => useLocalStorage<'list' | 'grid'>('view', 'list'));

/** The order of a folder: title, created or updated, "-" first for the reverse; pinned items come first anyway. */
export const useSort = createGlobalState(() => useLocalStorage('sort', ''));
