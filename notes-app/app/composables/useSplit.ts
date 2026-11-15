/** The Ultra split view: a second note beside the open one, on a wide screen and out of the focus mode, like the context panel. */
export function useSplit() {
  const { has } = usePro();
  const { beside } = useTabs();
  const route = useRoute();
  const focus = useState('focus', () => false);
  const wide = useMediaQuery('(min-width: 1024px)');
  const can = computed(() => has('splitView') && wide.value && !(focus.value && has('focusMode')));
  const shown = computed(() => can.value && !!beside.value && route.path.startsWith('/notes/'));

  // ⌘ + click on a link: the note opens in the other panel. One already on screen only takes the focus: two editors
  // of the same note would refuse each other's saves (#62).
  function openOther(id: string, fromBeside: boolean) {
    const open = document.querySelector<HTMLElement>(`[data-note-id="${id}"] .ProseMirror`);
    if (open) return open.focus();
    if (fromBeside) return navigateTo(`/notes/${id}`);
    beside.value = id;
  }

  return { can, shown, openOther };
}
