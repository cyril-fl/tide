// One choice for every view, kept across reloads.
export const useView = createGlobalState(() => useLocalStorage<'list' | 'grid'>('view', 'list'));

/** The order of a folder: title, created or updated, "-" first for the reverse; pinned items come first anyway. */
export const useSort = createGlobalState(() => useLocalStorage('sort', ''));

const previewState = createGlobalState(() => ({
  on: useLocalStorage('list-preview', false),
  shown: ref<string>(),
}));

/**
 * The read-only preview beside the lists of notes (Pro, #256): on is the choice kept, shown the note in it, active
 * whether the lists show it now; never below 1024px, where a note opens as before.
 */
export function usePreview() {
  const { has } = usePro();
  const { on, shown } = previewState();
  const wide = useMediaQuery('(min-width: 1024px)');
  const available = computed(() => wide.value && has('listPreview'));
  const active = computed(() => on.value && available.value);
  return { on, shown, available, active };
}
