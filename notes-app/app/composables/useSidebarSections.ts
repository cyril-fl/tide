import type { User } from '~/types/items';

/** The sidebar lists, saved to the account: the same on every device. Their order is the Pro mode's, hiding the folders the Ultra mode's. */
export function useSidebarSections() {
  const { data: me } = useNuxtData<User>('me');
  const { has } = usePro();
  const api = useApi();
  const saved = computed(() => mergeSidebar(me.value?.sidebar_sections ?? []));

  const sections = computed(() => {
    const ordered = has('sidebarOrder')
      ? saved.value
      : sidebarSectionIds.map((id) => saved.value.find((s) => s.id === id)!);
    return ordered.map((s) => (s.id === 'folders' && !has('hideFolders') ? { ...s, shown: true } : s));
  });
  const shown = (id: SidebarSectionId) => sections.value.some((s) => s.id === id && s.shown);

  // Saved from the account's list, not the shown one: a lower plan keeps the order and choices made above it.
  async function save(next: SidebarSection[]) {
    me.value = await api<User>('/api/me', { method: 'PATCH', body: { sidebar_sections: next } });
  }
  const toggle = (id: SidebarSectionId, value: boolean) =>
    save(saved.value.map((s) => (s.id === id ? { ...s, shown: value } : s)));
  const move = (draggedId: string, targetId: string) => save(moveSection(saved.value, draggedId, targetId));

  return { sections, shown, toggle, move };
}
