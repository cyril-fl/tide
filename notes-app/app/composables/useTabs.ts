/** A note open in the editor's tabs, with its title as the editor last read it: a tab needs no request of its own. */
export type Tab = { id: string; title?: string };

const openTabs = createGlobalState(() => useLocalStorage<Tab[]>('note-tabs', []));
const besideNote = createGlobalState(() => useLocalStorage('note-beside', ''));

/** The Ultra tabs of the editor, oldest first, and the note of the second panel of the split view (#128). */
export function useTabs() {
  const tabs = openTabs();
  const beside = besideNote();

  // A note trashed, deleted or no longer found leaves the tabs and the second panel.
  function forget(ids: string[]) {
    tabs.value = tabs.value.filter((tab) => !ids.includes(tab.id));
    if (ids.includes(beside.value)) beside.value = '';
  }

  return { tabs, beside, forget };
}
