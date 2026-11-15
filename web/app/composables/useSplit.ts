/** The Ultra split view: a second note beside the open one, on a wide screen and out of the focus mode, like the context panel. */
export function useSplit() {
  const { has } = usePro();
  const { icons } = useIcons();
  const { t } = useI18n();
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

  // What right-clicking a note offers wherever it is listed (tabs, sidebar, folders): opening it beside, not the
  // notes already on screen. Away from a note, there is nothing to open it beside: it opens.
  function besideMenu(id: string) {
    if (!can.value || id === route.params.id || id === beside.value) return [];
    const open = () => (route.path.startsWith('/notes/') ? openOther(id, false) : navigateTo(`/notes/${id}`));
    return [[{ label: t('note.openBeside'), icon: icons.beside, onSelect: open }]];
  }

  // ⌘ + click on a link to a note (tab, sidebar) opens it beside, not in a new tab of the browser. Not in the lists,
  // where ⌘ + click selects.
  function besideClick(id: string) {
    return (event: MouseEvent) => {
      const [[entry] = []] = besideMenu(id);
      if (!entry || !(event.metaKey || event.ctrlKey)) return;
      event.preventDefault();
      entry.onSelect();
    };
  }

  return { can, shown, openOther, besideMenu, besideClick };
}
