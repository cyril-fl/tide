/** The Ultra panels of the editor side by side (#253): how many the screen holds, and opening a note in another one. */
export function useSplit() {
  const { has } = usePro();
  const { icons } = useIcons();
  const { t } = useI18n();
  const tabs = useTabs();
  const route = useRoute();
  const focus = useState('focus', () => false);
  const can = computed(() => has('splitView') && !(focus.value && has('focusMode')));
  // Two stacked on a phone or a tablet held upright, then one more panel per wider screen.
  const breakpoints = useBreakpoints({ wide: 1700, wider: 2500 });
  const fits = computed(() =>
    breakpoints.greaterOrEqual('wider').value ? 4 : breakpoints.greaterOrEqual('wide').value ? 3 : 2,
  );
  const max = computed(() => (can.value ? fits.value : 1));
  // The panels drawn: the current one alone without the plan or in the focus mode, the others kept for later.
  const panels = computed(() => (can.value ? tabs.layout.value.panels : [tabs.current.value]));
  const shown = computed(() => panels.value.length > 1 && route.path.startsWith('/notes/'));
  const isShown = (id: string) => panels.value.some((panel) => panel.active === id);

  function focusShown(id: string) {
    const open = document.querySelector<HTMLElement>(`[data-note-id="${id}"] .ProseMirror`);
    open?.focus();
    return !!open;
  }

  // ⌘ + click on a link, "Open beside": the note in the panel after the one asked from, a new one when the screen has
  // room. One already on screen only takes the focus: two editors of the same note would refuse each other's saves (#62).
  // TODO: a [[Note#Heading]] opened beside does not scroll to its heading (read from the route), add it if it is missed.
  function openOther(id: string, from = tabs.current.value.id) {
    if (focusShown(id)) return;
    const where = route.path.startsWith('/notes/') && besideOf(tabs.layout.value, from, max.value);
    if (!where) return navigateTo(`/notes/${id}`);
    return 'panel' in where ? tabs.openIn(where.panel, id) : tabs.openAt(where.at, id);
  }

  // A plain click on a link of a panel opens the note there, which becomes the current one; a note shown in another
  // panel takes the focus there instead.
  function openHere(id: string, panelId: string, hash = '') {
    const shown = panels.value.find((panel) => panel.active === id);
    if (shown) return tabs.focus(shown.id);
    tabs.focus(panelId, false);
    return navigateTo(`/notes/${id}${hash}`);
  }

  function besideEntry(id: string, from?: string) {
    if (!can.value || isShown(id) || max.value < 2) return;
    return { label: t('note.openBeside'), icon: icons.beside, onSelect: () => openOther(id, from) };
  }

  // What right-clicking a note offers wherever it is listed (tabs, sidebar, folders, links): opening it beside, or in a
  // tab of its own in the panel it was asked from when it has none there.
  function openMenu(id: string, from?: string) {
    const panel = tabs.layout.value.panels.find((other) => other.id === from) ?? tabs.current.value;
    const tab =
      has('tabs') && !panel.tabs.some((tab) => tab.id === id)
        ? {
            label: t('note.openInTab'),
            icon: icons.newTab,
            onSelect: () => (panel.id === tabs.current.value.id ? tabs.openInTab(id) : tabs.openIn(panel.id, id, true)),
          }
        : undefined;
    const items = [besideEntry(id, from), tab].filter((item) => !!item);
    return items.length ? [items] : [];
  }

  // ⌘ + click on a link to a note (tab, sidebar) opens it beside, not in a new tab of the browser. Not in the lists,
  // where ⌘ + click selects.
  function besideClick(id: string, from?: string) {
    return (event: MouseEvent) => {
      const entry = besideEntry(id, from);
      if (!entry || !(event.metaKey || event.ctrlKey)) return;
      event.preventDefault();
      entry.onSelect();
    };
  }

  return { can, fits, max, panels, shown, openOther, openHere, openMenu, besideClick };
}
