/**
 * What a read-only editor renders a note with: the tables, callouts and diagrams every editor reads, without their
 * menus. Its embeds stay links: a note that includes itself, or one that includes it back, stops there.
 */
export function useNoteReader() {
  const baseURL = useRuntimeConfig().public.apiBase;
  const [tables] = useNoteTables();
  const [callouts] = useNoteCallouts();
  const code = useNoteCode();
  return {
    starterKit: { ...callouts!.starterKit, ...code.starterKit },
    extensions: [
      WikiLink,
      noteEmbed(),
      noteImage(baseURL, async () => null),
      ...tables!.extensions!,
      ...callouts!.extensions!,
      ...code.extensions!,
      InlineMath,
      BlockMath,
      InlineComment,
      BlockComment,
    ],
  };
}
