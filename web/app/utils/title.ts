/** The API gives "" or "untitled" for a note with no title: it shows as untitled, translated by the caller. */
export function displayTitle(title: string | undefined, untitled: string) {
  return !title || title === 'untitled' ? untitled : title;
}

const fence = /^[ \t]*(```|~~~)/;
const heading = /^ {0,3}#{1,6}[ \t]+(?=\S)/;

/** The index of the line of the title, the first heading as the API reads it (code skipped); -1 for none. */
export function titleLine(lines: string[]) {
  let code = false;
  return lines.findIndex((line) => {
    if (fence.test(line)) code = !code;
    return !code && heading.test(line);
  });
}

/** The markdown with title in its first heading, else a heading added on top. */
export function retitle(markdown: string, title: string) {
  const lines = markdown.split('\n');
  const index = titleLine(lines);
  if (index < 0) return markdown ? `# ${title}\n\n${markdown}` : `# ${title}`;
  lines[index] = lines[index]!.match(heading)![0] + title;
  return lines.join('\n');
}
