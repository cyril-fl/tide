/** Every keyboard shortcut of the app, by what it does: the one place to change a key, for its handler (keys joined by _) and the kbds shown. */
export function useShortcuts() {
  const shortcuts = {
    search: ['meta', 'k'],
    searchSlash: ['/'],
    newNote: ['alt', 'n'],
    newFolder: ['alt', 'shift', 'n'],
    fromSnippet: ['alt', 't'],
    toggleSidebar: ['meta', '\\'],
    focusMode: ['meta', '.'],
  };

  return { shortcuts, key: (name: keyof typeof shortcuts) => shortcuts[name].join('_') };
}
