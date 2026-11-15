import { diff3Merge, diffComm } from 'node-diff3';
import { titleLine } from './title';

// Lines both sides agree on, or a conflict: lines changed on both sides in the same place.
export type MergePart = { lines: string[] } | { mine: string[]; theirs: string[] };

/** A three-way merge by lines (#78): base is the text both sides started from. */
export function merge(base: string, mine: string, theirs: string): MergePart[] {
  return diff3Merge(mine.split('\n'), base.split('\n'), theirs.split('\n')).map((region) =>
    region.conflict ? { mine: region.conflict.a, theirs: region.conflict.b } : { lines: region.ok ?? [] },
  );
}

// Stands for the title line while merging a fork: no line of a note holds a NUL.
const title = '\u0000title';
const untitled = (text: string) => {
  const lines = text.split('\n');
  const index = titleLine(lines);
  if (index >= 0) lines[index] = title;
  return lines.join('\n');
};

// The fork with the line its base's title became marked, not merely its first heading: a title deleted in the fork
// (#176) is marked where it was, the next heading left as it is.
const untitledFork = (base: string, mine: string) => {
  const before = base.split('\n');
  const lines = mine.split('\n');
  const from = titleLine(before);
  const to = titleLine(lines);
  if (from < 0) return untitled(mine);
  let i = 0;
  let j = 0;
  for (const block of diffComm(before, lines)) {
    const [removed, added] = block.common
      ? [block.common.length, block.common.length]
      : [block.buffer1.length, block.buffer2.length];
    if (from < i + removed) {
      if (block.common) lines[j + from - i] = title;
      else if (to >= j && to < j + added) lines[to] = title;
      else lines.splice(j, 0, title);
      break;
    }
    i += removed;
    j += added;
  }
  return lines.join('\n');
};

/**
 * The merge of a fork back into its original (theirs): its title line is left out, the fork being retitled,
 * and the original keeps its own, where it is.
 */
export function mergeFork(base: string, mine: string, theirs: string): MergePart[] {
  const lines = theirs.split('\n');
  const kept = lines[titleLine(lines)];
  const put = (part: string[]) => part.flatMap((line) => (line !== title ? [line] : kept === undefined ? [] : [kept]));
  return merge(untitled(base), untitledFork(base, mine), untitled(theirs)).map((part) =>
    'lines' in part ? { lines: put(part.lines) } : { mine: put(part.mine), theirs: put(part.theirs) },
  );
}

export type Choice = 'mine' | 'theirs' | 'both';

// A line of a conflict compared from the other side (theirs) to this one (mine): in both, only theirs, only mine.
export type Row = { kind: Change['kind']; line: string };

/** The lines of a conflict in order, from the other side to this one. */
export function conflictRows(part: { mine: string[]; theirs: string[] }): Row[] {
  return diffComm(part.theirs, part.mine).flatMap((block): Row[] =>
    block.common
      ? block.common.map((line) => ({ kind: 'same', line }))
      : [
          ...block.buffer1.map((line) => ({ kind: 'removed' as const, line })),
          ...block.buffer2.map((line) => ({ kind: 'added' as const, line })),
        ],
  );
}

/** The lines a choice keeps: the lines in both always, this side's, the other's or all. */
export const choose = (rows: Row[], choice: Choice) =>
  rows.map((row) => row.kind === 'same' || choice === 'both' || (choice === 'mine') === (row.kind === 'added'));

/**
 * The text of a conflict from the lines kept. In a changed block this side's lines come first, then a blank line
 * between the two sides, or two paragraphs would read as one.
 */
export function pickedLines(rows: Row[], picked: boolean[]): string[] {
  const lines: string[] = [];
  let mine: string[] = [];
  let theirs: string[] = [];
  const flush = () => {
    lines.push(...mine, ...(mine.length && theirs.length ? [''] : []), ...theirs);
    mine = [];
    theirs = [];
  };
  rows.forEach((row, index) => {
    if (row.kind === 'same') {
      flush();
      lines.push(row.line);
    } else if (picked[index]) (row.kind === 'added' ? mine : theirs).push(row.line);
  });
  flush();
  return lines;
}

/** The text of the parts, each conflict with its lines picked, all of them by default. */
export function mergedText(parts: MergePart[], picks: string[][] = []) {
  let conflict = 0;
  return parts
    .flatMap((part) => {
      if ('lines' in part) return part.lines;
      const rows = conflictRows(part);
      return picks[conflict++] ?? pickedLines(rows, choose(rows, 'both'));
    })
    .join('\n');
}

export type Change = { kind: 'same' | 'removed' | 'added'; lines: string[] };

/** The lines kept, removed and added from before to after, for the comparison of a version (#78). */
export function lineChanges(before: string, after: string): Change[] {
  return diffComm(before.split('\n'), after.split('\n')).flatMap((block): Change[] =>
    block.common
      ? [{ kind: 'same', lines: block.common }]
      : [
          { kind: 'removed' as const, lines: block.buffer1 },
          { kind: 'added' as const, lines: block.buffer2 },
        ].filter((change) => change.lines.length),
  );
}
