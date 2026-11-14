import { Extension } from '@tiptap/core';
import { search } from 'prosemirror-search';

/** Find and replace in the rendered text of the note (the source mode has CodeMirror's): Mod-f opens the bar. */
export function useNoteSearch() {
  const open = ref(false);
  const NoteSearch = Extension.create({
    name: 'noteSearch',
    addProseMirrorPlugins: () => [search()],
    // true: the browser's own search does not open over it.
    addKeyboardShortcuts: () => ({ 'Mod-f': () => (open.value = true) }),
  });
  return { open, module: { extensions: [NoteSearch] } satisfies EditorModule };
}
