import type { Layout } from '~/utils/tabs';

/** A note open in a tab of the editor, with its title as the editor last read it: a tab needs no request of its own. */
export type Tab = { id: string; title?: string };

const newId = () => Math.random().toString(36).slice(2, 10);

// The panels of the editor (#253). Before them, the first panel's tabs (note-tabs) and the second one's (note-beside-tabs,
// before it note-beside, a single note) become the first two panels. A note being created has no place after a reload.
const panelsState = createGlobalState(() => {
  const read = (key: string) => {
    const value = localStorage.getItem(key);
    localStorage.removeItem(key);
    return value;
  };
  const tabs: Tab[] = JSON.parse(read('note-tabs') ?? '[]');
  const oldBeside = read('note-beside');
  const beside: { tabs: Tab[]; active: string } = JSON.parse(
    read('note-beside-tabs') ?? JSON.stringify({ tabs: oldBeside ? [{ id: oldBeside }] : [], active: oldBeside ?? '' }),
  );
  const first = { id: newId(), tabs, active: tabs[0]?.id ?? '' };
  const panels = beside.tabs.length ? [first, { id: newId(), ...beside }] : [first];
  const layout = useLocalStorage<Layout<Tab>>('note-panels', { panels, current: first.id });
  layout.value = closeIn(layout.value, ['new']);
  return layout;
});
// The tab asked for on its own: the next note the address opens takes a tab after the shown one instead of its place.
const freshNote = createGlobalState(() => ref(''));
// What the editor of each panel does before it shows another note: what is typed goes first (NoteEditor).
const panelLeaves = createGlobalState(() => new Map<string, () => Promise<boolean>>());
// The note the address was last sent to by the panels themselves (follow, focus, settle): it reflects them, it changes
// nothing. One at a time, cleared by the next navigation.
const followedNote = createGlobalState(() => ({ id: '' }));
// The tab dragged, which the other panels' bars and edges take too.
const draggedTab = createGlobalState(() => ref<{ id: string; panel: string }>());

/** The Ultra panels of the editor, each with its tabs (#128, #242, #253). The address shows the note of the current one. */
export function useTabs() {
  const layout = panelsState();
  const fresh = freshNote();
  const leaves = panelLeaves();
  const dragged = draggedTab();
  const followed = followedNote();
  const current = computed(() => layout.value.panels.find((panel) => panel.id === layout.value.current)!);
  const panelOf = (id: string) => layout.value.panels.find((panel) => panel.tabs.some((tab) => tab.id === id));

  // The layout once the editors of the panels whose note changes or that go have saved: false when one refuses.
  async function commit(next: Layout<Tab>, leave = true) {
    if (leave) {
      for (const panel of layout.value.panels) {
        const after = next.panels.find((other) => other.id === panel.id);
        if (!panel.active || after?.active === panel.active) continue;
        if (!((await leaves.get(panel.id)?.()) ?? true)) return false;
      }
    }
    layout.value = next;
    return true;
  }

  // On the note page, the address follows the note of the current panel, the home page once no note is left. A note
  // being created keeps it; elsewhere (a folder trashing notes) the page stays.
  async function follow() {
    const active = current.value.active;
    const at = useRouter().currentRoute.value.path;
    if (active === 'new' || !at.startsWith('/notes/')) return true;
    const path = active ? `/notes/${active}` : '/';
    if (at === path) return true;
    followed.id = active;
    return (await navigateTo(path)) === undefined;
  }

  /** The layout applied, the address following: false when a note not saved refused it. */
  async function apply(next: Layout<Tab>, leave = true) {
    return (await commit(next, leave)) && follow();
  }

  /** The note the address asks for: the panel showing it becomes the current one, else it shows in the current panel,
   * in the tab of the note shown or after it when asked (openInTab). Called by the note page before the address changes. */
  async function route(id: string) {
    // The address following a panel: refused once that note left the panels meanwhile (a tab closed right after).
    const following = followed.id === id;
    followed.id = '';
    if (following) {
      const shown = layout.value.panels.find((panel) => panel.active === id);
      if (shown) layout.value = { ...layout.value, current: shown.id };
      return !!shown;
    }
    const holder = layout.value.panels.find((panel) => panel.active === id);
    const next = holder
      ? {
          panels: layout.value.panels.map((panel) => (panel.id === holder.id ? { ...panel, active: id } : panel)),
          current: holder.id,
        }
      : showIn(layout.value, current.value.id, { id }, fresh.value === id);
    fresh.value = '';
    return commit(next);
  }

  /** Once at an address, the note of the current panel if it changed meanwhile: a tab closed while the address was
   * following a click in its panel (the router still at the note before, follow saw nothing to do). */
  function settle(path: string, failed: boolean) {
    if (failed && followed.id && path === `/notes/${followed.id}`) followed.id = '';
    const active = current.value.active;
    if (!path.startsWith('/notes/') || !active || active === 'new' || path === `/notes/${active}`) return;
    followed.id = active;
    navigateTo(`/notes/${active}`, { replace: true });
  }

  /** The current panel moves to another one, which the address follows (a click in its editor) unless the caller
   * navigates itself. */
  function focus(panelId: string, follow = true) {
    if (panelId === layout.value.current) return;
    layout.value = { ...layout.value, current: panelId };
    const active = current.value.active;
    if (!follow) return;
    if (!active || active === 'new') return;
    followed.id = active;
    navigateTo(`/notes/${active}`, { replace: true });
  }

  // A leave refused (a note not saved) keeps the tab asked for from the next note opened.
  async function openInTab(id: string) {
    fresh.value = id;
    if ((await navigateTo(`/notes/${id}`)) !== undefined) fresh.value = '';
  }

  /** A note shown in a panel, in its tab or in a new one (fresh), the panel it was in giving it up. */
  function openIn(panelId: string, id: string, fresh = false) {
    return apply(showIn(layout.value, panelId, { id }, fresh));
  }

  /** A note in a new panel at an index (an edge of a panel, "open beside" with room left). */
  function openAt(at: number, id: string) {
    return apply(splitAt(layout.value, { id }, at, newId()));
  }

  /** A panel showing another of its tabs, and becoming the current one. */
  function select(panelId: string, id: string) {
    return apply({
      current: panelId,
      panels: layout.value.panels.map((panel) => (panel.id === panelId ? { ...panel, active: id } : panel)),
    });
  }

  /** The tabs of a panel in another order. */
  function reorder(panelId: string, tabs: Tab[]) {
    layout.value = {
      ...layout.value,
      panels: layout.value.panels.map((panel) => (panel.id === panelId ? { ...panel, tabs } : panel)),
    };
  }

  // A tab closed: the note open there gives way to its neighbour, an emptied panel goes, the last one to the home page.
  const closeTab = (id: string) => apply(closeIn(layout.value, [id]));
  const closeOthers = (panelId: string, id: string) =>
    apply({
      ...layout.value,
      panels: layout.value.panels.map((panel) =>
        panel.id === panelId ? { ...panel, tabs: panel.tabs.filter((tab) => tab.id === id), active: id } : panel,
      ),
    });
  const closeAll = (panelId: string) =>
    apply(
      closeIn(layout.value, layout.value.panels.find((panel) => panel.id === panelId)?.tabs.map((tab) => tab.id) ?? []),
    );

  // Notes trashed or deleted leave the tabs of every panel, without waiting for editors that would save them.
  const forget = (ids: string[]) => apply(closeIn(layout.value, ids), false);
  // A note no longer found loses its tab, its panel still saying so until another note opens there.
  function drop(id: string) {
    layout.value = {
      ...layout.value,
      panels: layout.value.panels.map((panel) => ({ ...panel, tabs: panel.tabs.filter((tab) => tab.id !== id) })),
    };
  }

  /** A note being created got its id: its tab and its panel keep it, without another editor. */
  function created(id: string) {
    layout.value = {
      ...layout.value,
      panels: layout.value.panels.map((panel) => ({
        ...panel,
        tabs: panel.tabs.map((tab) => (tab.id === 'new' ? { ...tab, id } : tab)),
        active: panel.active === 'new' ? id : panel.active,
      })),
    };
  }

  /** The title of a note's tab, as its editor read it. */
  function title(id: string, name?: string) {
    const tab = panelOf(id)?.tabs.find((tab) => tab.id === id);
    if (tab && tab.title !== name) tab.title = name;
  }

  /** The panels once the screen holds max of them: the tabs of the others go to the last one kept. */
  const fit = (max: number) => layout.value.panels.length > max && apply(fitPanels(layout.value, max));

  return {
    layout,
    current,
    leaves,
    dragged,
    panelOf,
    apply,
    route,
    settle,
    focus,
    openInTab,
    openIn,
    openAt,
    select,
    reorder,
    closeTab,
    closeOthers,
    closeAll,
    forget,
    drop,
    created,
    title,
    fit,
  };
}
