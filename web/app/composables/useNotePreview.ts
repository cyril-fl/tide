import type { components } from '~/types/api';

type ReadNote = components['schemas']['readnotes.Note'];

// The start of a note by id and version, read once; the tiles drawn in the same tick are read in one call.
const previews = new Map<string, Ref<string | undefined>>();
let batch: { id: string; key: string; text: Ref<string | undefined> }[] = [];

async function flush(api: ReturnType<typeof useApi>) {
  const notes = batch;
  batch = [];
  for (let i = 0; i < notes.length; i += 50) {
    const chunk = notes.slice(i, i + 50);
    try {
      const read = await api<ReadNote[]>('/api/notes/read', {
        method: 'POST',
        body: { notes: chunk.map((note) => note.id), max_chars: 200 },
        quiet: true,
      });
      read.forEach((note, j) => (chunk[j]!.text.value = withoutTitle(note.content ?? '')));
    } catch {
      // A note trashed meanwhile fails its batch: those tiles keep their icon, retried when drawn again.
      for (const note of chunk) previews.delete(note.key);
    }
  }
}

/** The start of a note's text, without its title, for its tile; undefined while read or when source gives none. */
export function useNotePreview(source: () => { id: string; version?: string } | undefined) {
  const api = useApi();
  return computed(() => {
    const note = source();
    if (!note) return undefined;
    const key = `${note.id}:${note.version ?? ''}`;
    let text = previews.get(key);
    if (!text) {
      text = ref<string>();
      previews.set(key, text);
      if (!batch.length) setTimeout(() => flush(api));
      batch.push({ id: note.id, key, text });
    }
    return text.value;
  });
}
