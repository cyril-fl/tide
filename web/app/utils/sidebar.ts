/** The lists of the sidebar, in their default order. Repeated by domain.SidebarSectionIDs of core/domain. */
export const sidebarSectionIds = ['favorites', 'pinned', 'recent', 'folders', 'savedSearches', 'tags'] as const;
export type SidebarSectionId = (typeof sidebarSectionIds)[number];
export type SidebarSection = { id: SidebarSectionId; shown: boolean };

/** The saved sections in their order, the missing ones shown at the end, the unknown ones left out. */
export function mergeSidebar(saved: { id: string; shown: boolean }[]): SidebarSection[] {
  const known = saved.filter((s): s is SidebarSection => sidebarSectionIds.includes(s.id as SidebarSectionId));
  const missing = sidebarSectionIds.filter((id) => !known.some((s) => s.id === id));
  return [...known, ...missing.map((id) => ({ id, shown: true }))];
}

/** Moves the dragged section right before the target one. */
export function moveSection(sections: SidebarSection[], draggedId: string, targetId: string) {
  const next = [...sections];
  const from = next.findIndex((s) => s.id === draggedId);
  const to = next.findIndex((s) => s.id === targetId);
  if (from < 0 || to < 0 || from === to) return next;
  const [moved] = next.splice(from, 1);
  next.splice(to, 0, moved!);
  return next;
}
