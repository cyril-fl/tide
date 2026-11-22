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

/** The Ultra tabs of the editor, in their order, and those of the second panel of the split view (#128, #242). */
export function useTabs() {
  const tabs = openTabs();
  const besidePanel = besideTabs();
  const beside = computed(() => besidePanel.value.active);
  const shown = shownNote();
  const fresh = freshNote();
  const leaving = besideLeave();

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

  return { tabs, beside, besidePanel, leaving, forget, show, openInTab, setBeside };
}
