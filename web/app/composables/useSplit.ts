/** The Ultra split view: a second note beside the open one, out of the focus mode; below the one under it (the note page). */
export function useSplit() {
  const { has } = usePro();
  const { icons } = useIcons();
  const { t } = useI18n();
  const { tabs, beside, besidePanel, openInTab, setBeside } = useTabs();
  const route = useRoute();
  const focus = useState('focus', () => false);
  const can = computed(() => has('splitView') && !(focus.value && has('focusMode')));
  const shown = computed(() => can.value && !!beside.value && route.path.startsWith('/notes/'));

  // ⌘ + click on a link: the note opens in the other panel. One already on screen only takes the focus: two editors
  // of the same note would refuse each other's saves (#62).
  function openOther(id: string, fromBeside: boolean) {
    const open = document.querySelector<HTMLElement>(`[data-note-id="${id}"] .ProseMirror`);
    if (open) return open.focus();
    if (fromBeside) return navigateTo(`/notes/${id}`);
    return setBeside(openTab(besidePanel.value, { id }, false));
  }

  // ⌘ + click or "Open beside": the note in the other panel, not one already on screen. Away from a note, there is
  // nothing to open it beside: it opens. fromBeside: a link of the second panel, whose other panel is the first.
  function besideEntry(id: string, fromBeside = false) {
    if (!can.value || id === route.params.id || id === beside.value) return;
    const open = () => (route.path.startsWith('/notes/') ? openOther(id, fromBeside) : navigateTo(`/notes/${id}`));
    return { label: t('note.openBeside'), icon: icons.beside, onSelect: open };
  }

  // What right-clicking a note offers wherever it is listed (tabs, sidebar, folders, links): opening it beside, or in a
  // tab of its own when it has none.
  function openMenu(id: string, fromBeside = false) {
    const tab =
      has('tabs') && !tabs.value.some((tab) => tab.id === id)
        ? { label: t('note.openInTab'), icon: icons.newTab, onSelect: () => openInTab(id) }
        : undefined;
    const items = [besideEntry(id, fromBeside), tab].filter((item) => !!item);
    return items.length ? [items] : [];
  }

  // ⌘ + click on a link to a note (tab, sidebar) opens it beside, not in a new tab of the browser. Not in the lists,
  // where ⌘ + click selects.
  function besideClick(id: string) {
    return (event: MouseEvent) => {
      const entry = besideEntry(id);
      if (!entry || !(event.metaKey || event.ctrlKey)) return;
      event.preventDefault();
      entry.onSelect();
    };
  }

  return { can, shown, openOther, openMenu, besideClick };
}
