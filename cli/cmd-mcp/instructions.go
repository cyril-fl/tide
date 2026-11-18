package cmdmcp

// Instructions tells an agent the cheap way through a big account, which its client puts in its system prompt: in the
// benchmark of #198, reading whole notes and rewriting them to tag them made most of the cost. It holds the recipes
// once, so the tool descriptions stay one sentence (#225). The writing tips are left out when readOnly, whose tools
// cannot write.
func Instructions(readOnly bool) string {
	text := "The account's markdown notes, in folders. Name a note or folder by its uuid or its path from the root (/Folder/Note); " +
		"a path several notes share is ambiguous (search_notes list=duplicates finds them): use their ids. Tags are named without the #.\n" +
		"Read as little as possible:\n" +
		"- Call tags once and keep the list; tags with prefix checks a tag exists, tags with a tag lists its notes.\n" +
		"- Find the candidates on the server: the filters of search_notes all hold at once, with or without a text: " +
		"tags_all, tags_any, tags_none (Idée AND (Cuisine OR Boisson) is tags_all [Idée], tags_any [Cuisine, Boisson]), folder, " +
		"not_text (the notes lacking a text), untagged, max_length (empty or title-only notes), updated_before (stale notes), empty_sections (headings left empty).\n" +
		"- Preview first: read_notes takes 50 notes, with max_chars, then in full only those that need it; list_folder with tags, length or preview sorts a folder unread.\n" +
		"- Lists give 50 items and the total: page a big folder with limit and offset, one page per subagent.\n" +
		"- note_links without a note lists the broken links.\n" +
		"- Subagents do not receive these instructions: put the lines they need in their prompt."
	if !readOnly {
		text += "\n- Never rewrite a whole note to change a part: tag_notes for tags, edit_note with old, section or title for a passage or the title.\n" +
			"- Tag in few calls: decide a batch's tags first, then one tag_notes for the whole batch, its ops a group of notes each, or on the search filters (dry_run first); " +
			"it answers counts and each note's diff, no need to read back.\n" +
			"- A #tag in code, a link or an address is not read: tag_notes refuses to write one there. A #hashtag in a snippet tags the notes it starts.\n" +
			"- Across notes: replace_notes replaces a text everywhere, rename_tag with merge folds a tag into another (dry_run first).\n" +
			"- A locked note (readonly in answers) refuses every write. Rewrites keep the earlier text in note_history; deletions go to the trash (list_folder trash, restore_item)."
	}
	return text
}
