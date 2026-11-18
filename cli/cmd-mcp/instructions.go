package cmdmcp

// Instructions tells an agent the cheap way through a big account, which its client puts in its system prompt: in the
// benchmark of #198, reading whole notes and rewriting them to tag them made most of the cost. The writing tips are
// left out when readOnly, whose tools cannot write.
func Instructions(readOnly bool) string {
	text := "The account's markdown notes, in folders; name a note or folder by its uuid or its path from the root (/Folder/Note).\n" +
		"Work through many notes reading as little as possible:\n" +
		"- Call tags once for the account's tags (prefix or min_notes to narrow it) and keep the list.\n" +
		"- Find the candidates on the server instead of reading: search_notes with a text, tags_all, tags_any, tags_none, folder, not_text (the notes lacking a text), untagged, max_length (empty or title-only notes) or updated_before (stale notes), all at once.\n" +
		"- Read up to 50 notes per call with read_notes and max_chars first; in full only those that need it.\n" +
		"- Lists give 50 items and the total: split a big folder into list_folder pages (limit, offset), one per subagent if you have them.\n" +
		"- duplicate_notes lists the notes sharing a title: a path several notes share is ambiguous, use their ids.\n" +
		"- Subagents do not receive these instructions: put the lines they need in their prompt."
	if !readOnly {
		text += "\n- Tag with tag_notes, given the notes or the filters of search_notes (dry_run first), never by rewriting a note with edit_note, which replaces all of it.\n" +
			"- Correct a passage of a note with replace_in_note and its title with rename_note, not by rewriting it with edit_note.\n" +
			"- Tag in few calls: decide the tags of a whole batch first, then one tag_notes per change, all its notes at once (every note getting Obsolete in one call), not one call per note: it answers counts and each note's diff, no need to read the notes back. tags with a tag lists the notes already carrying it.\n" +
			"- Fold a tag into another with rename_tag and merge; replace a text in every note with replace_notes (dry_run first).\n" +
			"- Rewrites keep the old text in note_history, deletions go to the trash."
	}
	return text
}
