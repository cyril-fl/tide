<script setup lang="ts">
import type { Snippet } from '~/types/items';

const { icons } = useIcons();
const { t, locale } = useI18n();
const { has } = usePro();
const { data: me } = useMe();
const { confirm, snippet: editSnippet } = useDialogs();
const toast = useToast();

// The order is for looking through them here only: the ;; menu keeps the list's, and nothing is saved.
const sortKey = ref<'created' | 'updated' | 'title'>('created');
const descending = ref(false);
const sortKeys = computed(() =>
  (['created', 'updated', 'title'] as const).map((value) => ({
    label: t(`folder.sort${value[0]!.toUpperCase()}${value.slice(1)}`),
    value,
  })),
);
// A snippet saved before the dates existed has none: 0, first, the list keeping their order (the order they came in).
const time = (date?: string) => (date ? Date.parse(date) : 0);
const shown = computed(() => {
  const snippets = me.value?.snippets ?? [];
  if (!has('snippetSort')) return snippets;
  const key = sortKey.value;
  const sorted = snippets.toSorted((a, b) =>
    key === 'title' ? a.name.localeCompare(b.name, locale.value) : time(a[`${key}_at`]) - time(b[`${key}_at`]),
  );
  return descending.value ? sorted.reverse() : sorted;
});

// The snippet whose text shows under its row, one at a time.
const open = ref<string>();

// The API takes the whole list: an edit sends it back with the one changed. change makes it from the account's, and
// runs again on the fresh one after a 409; a snippet is found there by its name, as it has no id.
const editMe = useEditMe();
function saveSnippets(change: (snippets: Snippet[]) => Snippet[]) {
  return editMe((account) => ({ snippets: change([...(account.snippets ?? [])]) }));
}

async function addSnippet() {
  const added = await editSnippet({ title: t('settings.addSnippet') });
  if (added) await saveSnippets((snippets) => [...snippets, added]);
}

async function changeSnippet(old: Snippet) {
  const changed = await editSnippet({ title: t('settings.editSnippet'), value: old });
  if (!changed) return;
  await saveSnippets((snippets) => {
    const at = snippets.findIndex((s) => s.name === old.name);
    // Its created_at goes along, so a rename keeps the date the server gave it.
    snippets.splice(at < 0 ? snippets.length : at, 1, { ...old, ...changed });
    return snippets;
  });
}

async function removeSnippet(old: Snippet) {
  const confirmed = await confirm({
    title: t('settings.deleteSnippet', { name: old.name }),
    action: t('common.delete'),
  });
  if (!confirmed) return;
  await saveSnippets((snippets) => {
    const at = snippets.findIndex((s) => s.name === old.name);
    if (at >= 0) snippets.splice(at, 1);
    return snippets;
  });
}

// A snippet travels as a .md file named after it, without what a file system refuses in a name.
const fileName = (name: string) => `${name.replace(/[\\/:*?"<>|]/g, '-')}.md`;
const withoutExtension = (path: string) => path.replace(/^.*\//, '').replace(/\.(md|markdown|txt)$/i, '');

function download(name: string, data: BlobPart, type: string) {
  const url = URL.createObjectURL(new Blob([data], { type }));
  const link = Object.assign(document.createElement('a'), { href: url, download: name });
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000); // Safari reads the url after click returns
}

function exportSnippet(snippet: Snippet) {
  download(fileName(snippet.name), snippet.content, 'text/markdown');
}

function exportSnippets() {
  // Two names a file system writes alike get (2), (3)... rather than one file over the other.
  const taken = new Set<string>();
  const entries = (me.value?.snippets ?? []).map((snippet) => {
    let name = fileName(snippet.name);
    for (let n = 2; taken.has(name.toLowerCase()); n++) name = fileName(`${snippet.name} (${n})`);
    taken.add(name.toLowerCase());
    return { name, data: new TextEncoder().encode(snippet.content) };
  });
  download('snippets.zip', zip(entries), 'application/zip');
}

// .md files, one snippet each, or a zip of them (an export, another account's, a folder of templates).
const picker = useFileDialog({ accept: '.md,.markdown,.txt,.zip', multiple: true, reset: true });
picker.onChange((files) => files && importSnippets([...files]));

async function readSnippets(files: File[]) {
  const read: Snippet[] = [];
  for (const file of files) {
    if (!/\.zip$/i.test(file.name)) {
      read.push({ name: withoutExtension(file.name), content: await file.text() });
      continue;
    }
    for (const entry of await unzip(new Uint8Array(await file.arrayBuffer()))) {
      // macOS adds __MACOSX/ and ._ files to its zips: no snippets.
      if (!/\.(md|markdown|txt)$/i.test(entry.name) || /(^|\/)(__MACOSX\/|\.)/.test(entry.name)) continue;
      read.push({ name: withoutExtension(entry.name), content: new TextDecoder().decode(entry.data) });
    }
  }
  // Two files of one name: the last one read wins, as it would over a saved snippet.
  return [
    ...new Map(read.filter((s) => s.name.trim()).map((s) => [s.name.trim(), { ...s, name: s.name.trim() }])).values(),
  ];
}

async function importSnippets(files: File[]) {
  let read: Snippet[];
  try {
    read = await readSnippets(files);
  } catch {
    read = [];
  }
  if (!read.length) return toast.add({ title: t('settings.snippetsNoneFound'), color: 'warning' });
  const taken = read.filter((s) => me.value?.snippets.some((saved) => saved.name === s.name));
  let choice: boolean | 'alternative' | undefined = true;
  if (taken.length) {
    choice = await confirm({
      title: t('settings.snippetsConflict', taken.length),
      description: taken.map((s) => s.name).join(', '),
      action: t('settings.snippetsReplace'),
      alternative: t('settings.snippetsKeepBoth'),
    });
    if (!choice) return;
  }
  const saved = await saveSnippets((snippets) => {
    for (const snippet of read) {
      const at = snippets.findIndex((s) => s.name === snippet.name);
      // Replaced, it keeps its place and the server its created_at; kept beside, it takes the first free (n).
      if (at >= 0 && choice === true) snippets[at] = { ...snippets[at], ...snippet };
      else {
        let name = snippet.name;
        for (let n = 2; snippets.some((s) => s.name === name); n++) name = `${snippet.name} (${n})`;
        snippets.push({ ...snippet, name });
      }
    }
    return snippets;
  });
  if (saved) toast.add({ title: t('settings.snippetsImported', read.length), color: 'success' });
}
</script>

<template>
  <ul class="divide-y divide-default rounded-lg border border-default">
    <li class="flex flex-wrap items-center gap-4 px-4 py-3">
      <div class="flex-1 basis-48">
        <p class="text-sm">{{ $t('settings.snippets') }}</p>
        <p class="text-xs text-muted">{{ $t('settings.snippetsDescription') }}</p>
      </div>
      <!-- One group, so on a phone the controls wrap together under the text. -->
      <div class="flex items-center gap-1">
        <USelect
          v-if="has('snippetSort') && shown.length > 1"
          v-model="sortKey"
          :items="sortKeys"
          :aria-label="$t('folder.sort')"
          color="neutral"
          variant="ghost"
          class="w-40 max-sm:w-32"
        />
        <UTooltip
          v-if="has('snippetSort') && shown.length > 1"
          :text="descending ? $t('settings.sortDescending') : $t('settings.sortAscending')"
        >
          <UButton
            :icon="descending ? icons.descending : icons.ascending"
            color="neutral"
            variant="ghost"
            :aria-label="descending ? $t('settings.sortDescending') : $t('settings.sortAscending')"
            @click="descending = !descending"
          />
        </UTooltip>
        <UTooltip :text="$t('settings.importSnippets')">
          <UButton
            :icon="icons.import"
            color="neutral"
            variant="ghost"
            :aria-label="$t('settings.importSnippets')"
            :disabled="!me"
            @click="picker.open()"
          />
        </UTooltip>
        <UTooltip :text="$t('settings.exportSnippets')">
          <UButton
            :icon="icons.export"
            color="neutral"
            variant="ghost"
            :aria-label="$t('settings.exportSnippets')"
            :disabled="!me?.snippets.length"
            @click="exportSnippets"
          />
        </UTooltip>
        <UButton
          :label="$t('settings.addSnippet')"
          :ui="{ label: 'max-sm:sr-only' }"
          :icon="icons.noteAdd"
          color="neutral"
          variant="outline"
          :disabled="!me"
          class="ms-1"
          @click="addSnippet"
        />
      </div>
    </li>
    <li v-for="snippet in shown" :key="snippet.name">
      <div class="flex items-center gap-3 px-4 py-2 transition-colors hover:bg-elevated/50">
        <button
          type="button"
          class="flex min-w-0 flex-1 items-center gap-3 self-stretch text-start"
          :aria-expanded="open === snippet.name"
          @click="open = open === snippet.name ? undefined : snippet.name"
        >
          <UIcon
            :name="icons.snippet"
            class="size-4 shrink-0 transition-colors"
            :class="open === snippet.name ? 'text-primary' : 'text-muted'"
          />
          <span class="min-w-0 flex-1 truncate text-sm">{{ snippet.name }}</span>
          <!-- The date the list is sorted by, when it is one. -->
          <span
            v-if="has('snippetSort') && sortKey !== 'title' && snippet[`${sortKey}_at`]"
            class="shrink-0 text-xs text-muted max-sm:hidden"
          >
            {{ new Date(snippet[`${sortKey}_at`]!).toLocaleDateString(locale) }}
          </span>
        </button>
        <UTooltip :text="$t('settings.exportSnippet')">
          <UButton
            :icon="icons.export"
            color="neutral"
            variant="ghost"
            :aria-label="$t('settings.exportSnippet')"
            @click="exportSnippet(snippet)"
          />
        </UTooltip>
        <UButton
          :icon="icons.rename"
          color="neutral"
          variant="ghost"
          :aria-label="$t('settings.editSnippet')"
          @click="changeSnippet(snippet)"
        />
        <UButton
          :icon="icons.delete"
          color="neutral"
          variant="ghost"
          :aria-label="$t('common.delete')"
          @click="removeSnippet(snippet)"
        />
      </div>
      <!-- The text as a note shows it, read only; a long one scrolls. -->
      <div v-if="open === snippet.name" class="flex max-h-80 flex-col border-t border-default bg-elevated/25">
        <NoteWriter :model-value="snippet.content" readonly compact :label="snippet.name" />
      </div>
    </li>
    <li v-if="me && !me.snippets.length" class="px-4 py-3 text-xs text-muted">
      {{ $t('settings.snippetsEmpty') }}
    </li>
  </ul>
</template>
