import type { Panel } from '~/utils/tabs';

/** A note open in the editor's tabs, with its title as the editor last read it: a tab needs no request of its own. */
export type Tab = { id: string; title?: string };

const openTabs = createGlobalState(() => useLocalStorage<Tab[]>('note-tabs', []));
// The second panel showed one note (note-beside) before it had tabs (#242): that note is its first tab.
const besideTabs = createGlobalState(() => {
  const old = localStorage.getItem('note-beside');
  localStorage.removeItem('note-beside');
  return useLocalStorage<Panel<Tab>>('note-beside-tabs', { tabs: old ? [{ id: old }] : [], active: old ?? '' });
});
// The note of the first panel, whose tab a note opened replaces, and the note asked for in a tab of its own.
const shownNote = createGlobalState(() => useSessionStorage('note-shown', ''));
const freshNote = createGlobalState(() => ref(''));
// What the editor of the second panel does before it shows another note: no route guards it (NoteEditor).
const besideLeave = createGlobalState(() => shallowRef<() => Promise<boolean>>());
// The tab dragged, which the other panel's bar takes too: it shows while it is dragged, even with one note.
const draggedTab = createGlobalState(() => ref<{ id: string; beside: boolean }>());

/** The Ultra tabs of the editor, in their order, and those of the second panel of the split view (#128, #242). */
export function useTabs() {
  const tabs = openTabs();
  const besidePanel = besideTabs();
  const beside = computed(() => besidePanel.value.active);
  const shown = shownNote();
  const fresh = freshNote();
  const leaving = besideLeave();
  const dragged = draggedTab();

  // A note trashed, deleted or no longer found leaves the tabs of both panels.
  function forget(ids: string[]) {
    tabs.value = tabs.value.filter((tab) => !ids.includes(tab.id));
    besidePanel.value = ids.reduce(withoutTab, besidePanel.value);
  }

  // The first panel shows a note: its tab takes the title, or the note takes the tab of the note it replaces.
  function show(tab: Tab) {
    const open = tabs.value.find((open) => open.id === tab.id);
    if (open) open.title = tab.title;
    else tabs.value = placeTab(tabs.value, shown.value, tab, fresh.value === tab.id);
    shown.value = tab.id;
    fresh.value = '';
  }

  // A leave refused (a note not saved) keeps the tab asked for from the next note opened.
  async function openInTab(id: string) {
    fresh.value = id;
    if ((await navigateTo(`/notes/${id}`)) !== undefined) fresh.value = '';
  }

  // The second panel shows another note, or none, once what is typed in the one it shows is saved: false if refused.
  async function setBeside(next: Panel<Tab>) {
    if (next.active !== besidePanel.value.active && !((await leaving.value?.()) ?? true)) return false;
    besidePanel.value = next;
    return true;
  }

  // A tab of the first panel closed: the note open there gives way to its neighbour, the last one to the home page. A leave
  // refused (a note not saved) keeps the tab: false.
  async function closeTab(id: string) {
    if (id === useRouter().currentRoute.value.params.id) {
      const next = tabAfterClose(
        tabs.value.map((tab) => tab.id),
        id,
      );
      if ((await navigateTo(next ? `/notes/${next}` : '/')) !== undefined) return false;
    }
    tabs.value = tabs.value.filter((tab) => tab.id !== id);
    return true;
  }

  // The second panel shows a note, in the tab of the one it shows or in a tab of its own (fresh). Its tab in the first
  // panel moves here: a note has one tab (#62). The note of the first one opening in the second, the first goes on.
  async function showBeside(id: string, fresh = false) {
    if (id === besidePanel.value.active || !((await leaving.value?.()) ?? true)) return;
    const moved = tabs.value.find((tab) => tab.id === id);
    if (moved && !(await closeTab(id))) return;
    besidePanel.value = openTab(besidePanel.value, moved ?? { id }, fresh);
  }

  return {
    tabs,
    beside,
    besidePanel,
    leaving,
    dragged,
    forget,
    show,
    openInTab,
    setBeside,
    closeTab,
    showBeside,
  };
}
