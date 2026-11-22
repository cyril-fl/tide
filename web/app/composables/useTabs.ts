/** A note open in the editor's tabs, with its title as the editor last read it: a tab needs no request of its own. */
export type Tab = { id: string; title?: string };

const openTabs = createGlobalState(() => useLocalStorage<Tab[]>('note-tabs', []));
const besideNote = createGlobalState(() => useLocalStorage('note-beside', ''));
// The note of the first panel, whose tab a note opened replaces, and the note asked for in a tab of its own.
const shownNote = createGlobalState(() => useSessionStorage('note-shown', ''));
const freshNote = createGlobalState(() => ref(''));

/** The Ultra tabs of the editor, in their order, and the note of the second panel of the split view (#128). */
export function useTabs() {
  const tabs = openTabs();
  const beside = besideNote();
  const shown = shownNote();
  const fresh = freshNote();

  // A note trashed, deleted or no longer found leaves the tabs and the second panel.
  function forget(ids: string[]) {
    tabs.value = tabs.value.filter((tab) => !ids.includes(tab.id));
    if (ids.includes(beside.value)) beside.value = '';
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

  return { tabs, beside, forget, show, openInTab };
}
