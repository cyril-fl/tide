<script setup lang="ts">
import type { Editor } from '@tiptap/vue-3';
import type { FetchError } from 'ofetch';
import type { RouteLocationNormalized } from 'vue-router';
import type { ForkOrigin, Item, ItemDetail, NotesGraph, User } from '~/types/items';
import type { Changes } from '~/utils/saver';

const { icons } = useIcons();
const { shortcuts } = useShortcuts();
const { has, ready } = usePro();
const focus = useState('focus', () => false);
const { locale, t } = useI18n();

// start: the starting text of a new note, saved only once typed in. note: the note already read by the page, shown at once.
// beside: the second panel of the split view, which leaves the page's title, the renaming and the tabs to the first.
const props = defineProps<{
  noteId?: string;
  parentId?: string;
  start?: string;
  note?: ItemDetail;
  beside?: boolean;
}>();
const emit = defineEmits<{ created: [id: string] }>();

const api = useApi();
const { confirm, confirmMerge, prompt, resolveConflict } = useDialogs();
const { removeItem } = useItems();
const { tabs, leaving, forget, show, setBeside } = useTabs();
const baseURL = useRuntimeConfig().public.apiBase;
const toast = useToast();

const id = ref(props.noteId);
const parent = ref(props.parentId);
const path = ref('');
const title = ref<string>();
if (!props.beside) useHead({ title: () => displayTitle(title.value, t('untitled')) });
const tags = ref<string[]>([]);
let links = '';
const initialContent = ref(props.start ?? '');
const loading = ref(!!props.noteId);
const notFound = ref(false);
// Locked read-only: the API refuses its content, the editor and the source mode only read.
const readonly = ref(false);
// The forks of the note, merged from here like a merge request, and the one whose merge can be undone (#177).
const { data: neighbors } = useNeighbors(id);
const me = useNuxtData<User>('me').data;

const failed = ref(false);
const saving = ref(false);
const isEmpty = ref(!props.noteId);
// The editor can change its document while loading: only what follows a user interaction is an edit,
// or merely opening a note would rewrite the markdown written with the CLI. Pointerdown too: a task checkbox takes no focus.
const touched = ref(false);
// The version the text is written over (#62): once another device changed the note, the API answers 409 and nothing more is sent.
let version: string | undefined;
// The text on the API as this editor last saw it: a moved note gets a new updated_at, not a new text.
let saved = '';
// The version saved, for the history in the context panel to read again.
const savedAt = ref<string>();
let stale = false;

const saver = createSaver(send);
const editorRef = useTemplateRef<{ editor?: Editor }>('editor');

async function send(changes: Changes): Promise<boolean | undefined> {
  if (id.value && Object.values(changes).every((value) => value === undefined)) return;
  // The API refuses a note with no content: a new one waits for its first text.
  if (!id.value && !changes.content) return false;
  if (stale) return false;

  saving.value = true;
  try {
    await (id.value ? patchNote(changes) : createNote(changes));
    failed.value = false;
  } catch (error) {
    failed.value = true;
    stale = (error as FetchError).status === 409;
    if (stale && changes.content !== undefined) void mergeConflict(changes.content);
    throw error;
  } finally {
    saving.value = false;
  }
}

async function createNote(changes: Changes) {
  const body = { ...changes, parent_id: parent.value };
  const item = await api<Item>('/api/notes', {
    method: 'POST',
    body,
    invalid: { [NO_PARENT]: t('items.parentInTrash') },
    tooLarge: t('note.tooLarge'),
  });
  id.value = item.id;
  emit('created', item.id);
  const note = await api<ItemDetail>(`/api/notes/${item.id}`);
  remember(note);
  await readTags(changes, note);
}

async function patchNote(changes: Changes) {
  const body = { ...changes, updated_at: version };
  const note = await api<ItemDetail>(`/api/notes/${id.value}`, {
    method: 'PATCH',
    body,
    quiet: [409], // mergeConflict tells
    tooLarge: t('note.tooLarge'),
  });
  remember(note);
  await readTags(changes, note);
}

// After a 409 (#78): the note as the API has it now, merged by lines with the text typed here, from the text both started from.
// Changes in different places merge on their own; in the same place the conflicts dialog asks, the text typed here kept in the history first.
// The merged text then replaces the text as if typed. A locked note or a failure: the toast offers to reload.
async function mergeConflict(mine: string) {
  try {
    const note = await api<ItemDetail>(`/api/notes/${id.value}`, { quiet: true });
    if (note.readonly) throw new Error('locked on another device');
    const parts = merge(saved, mine, note.content ?? '');
    let text = mergedText(parts);
    if (parts.some((part) => !('lines' in part))) {
      await api(`/api/notes/${id.value}/versions`, { method: 'POST', body: { content: mine }, quiet: true });
      text = (await resolveConflict(parts)) ?? text;
    } else toast.add({ title: t('note.merged'), color: 'success' });
    remember(note);
    saver.take(); // Refused, the merged text replaces it.
    stale = false;
    failed.value = false;
    setText(text);
  } catch {
    const actions = [{ label: t('note.reload'), onClick: reloadStale }];
    toast.add({ title: t('note.changedElsewhere'), color: 'error', duration: 0, actions });
  }
}

// When the merge fails (#78): the text typed here is kept as a version marked conflict, then the page reads the note again from the API.
// Not kept, nothing reloads: the text stays on screen, the toast of useApi says why.
async function reloadStale() {
  const content = source.value ?? cleanMarkdown(editorRef.value?.editor?.getMarkdown() ?? '');
  try {
    await api(`/api/notes/${id.value}/versions`, { method: 'POST', body: { content }, tooLarge: t('note.tooLarge') });
  } catch {
    return;
  }
  saver.take(); // Refused anyway: closing the page must not send it again.
  location.reload();
}

function remember(note: ItemDetail) {
  version = note.item.updated_at ?? note.item.created_at; // A note never edited has no updated_at.
  saved = note.content ?? '';
  savedAt.value = version;
}

// The API turns the #tags of the text into tags and its [[links]] into note ids: the sidebar, the related notes and the graph follow when they or the title change.
async function readTags(changes: Changes, note: ItemDetail) {
  if (changes.content === undefined) return;
  showResolvedLinks(changes.content, note.content ?? '');
  const changed =
    note.tags?.join() !== tags.value.join() ||
    linkedNotes(note.content ?? '') !== links ||
    note.item.title !== title.value;
  tags.value = note.tags ?? [];
  links = linkedNotes(note.content ?? '');
  title.value = note.item.title;
  if (changed) await refreshNuxtData();
}

// The API points the typed [[links]] to their note: the editor shows them resolved, without touching the cursor or the undo history.
function showResolvedLinks(sent: string, saved: string) {
  const editor = editorRef.value?.editor;
  const links = resolvedLinks(sent, saved);
  if (!editor || !links.size) return;
  const { tr } = editor.state;
  editor.state.doc.descendants((node, pos) => {
    const link =
      ['wikiLink', 'noteEmbed'].includes(node.type.name) &&
      !node.attrs.id &&
      links.get(node.attrs.target.toLowerCase());
    if (link) tr.setNodeMarkup(pos, undefined, link);
  });
  if (tr.docChanged) editor.view.dispatch(tr.setMeta('addToHistory', false));
}

async function load() {
  if (!props.noteId) return;
  try {
    // No await when the page read it: the first render already shows the note.
    const note = props.note ?? (await api<ItemDetail>(`/api/notes/${props.noteId}`, { quiet: true }));
    initialContent.value = note.content ?? '';
    tags.value = note.tags ?? [];
    links = linkedNotes(initialContent.value);
    title.value = note.item.title;
    readonly.value = !!note.readonly;
    remember(note);
    place(note);
    isEmpty.value = false;
  } catch {
    notFound.value = true;
    forget([props.noteId]);
  } finally {
    loading.value = false;
  }
}
load();

// The Ultra tabs follow the note shown: it takes the tab of the one before once loaded, renamed with it (#128).
watch(
  [id, title, loading, ready],
  ([note, name]) => {
    if (!note || loading.value || notFound.value || !ready.value) return;
    const tab = tabs.value.find((tab) => tab.id === note);
    if (!props.beside && has('tabs')) show({ id: note, title: name });
    else if (tab) tab.title = name;
  },
  { immediate: true },
);

function place(note: ItemDetail) {
  path.value = note.path.slice(0, note.path.lastIndexOf('/')) || '/';
  parent.value = note.item.parent_id ?? undefined;
}
// The note moved from the Cmd K palette: its path follows when the views read again, without holding them up, and its version when its text is the one saved.
const unhook = useNuxtApp().hook('app:data:refresh', () => {
  if (!id.value) return;
  api<ItemDetail>(`/api/notes/${id.value}`, { quiet: true }).then(
    (note) => {
      place(note);
      if (note.content === saved) remember(note);
    },
    () => {},
  );
});
onBeforeUnmount(unhook);

function save(content: string) {
  isEmpty.value = !content;
  saver.push({ content: content || undefined });
}

function onContent(markdown: string) {
  if (touched.value && !readonly.value) save(markdown);
}

// What is typed before goes first: the API refuses it once locked.
async function toggleLock() {
  await saver.flush();
  const note = await api<ItemDetail>(`/api/notes/${id.value}`, {
    method: 'PATCH',
    body: { readonly: !readonly.value },
  });
  readonly.value = !!note.readonly;
}
// A version from the history (#61) replaces the text as if typed: saved by the usual PATCH, undone by Cmd Z.
// What is typed before is saved first: the restore then replaces the text as saved, which the API keeps (#94).
async function restoreVersion(content: string) {
  await saver.flush();
  saver.push({ restore: true });
  setText(content);
}
// A fork into this note, like a merge request: a three-way merge by lines from the fork's base, its title line left
// out, conflicts asked in the conflicts dialog, then the merge dialog shows what the text becomes and asks whether the
// fork goes to the trash, unless the account chose for good (after_merge). The API saves it as a restore, the text
// kept in the history, and moves the base to the fork's text, so a second merge does not replay these changes.
// Changed meanwhile (409): merged again once, with the choice already made (chosen).
async function mergeIn(forkId: string, chosen?: boolean): Promise<void> {
  const again = chosen === undefined;
  await saver.flush();
  if (failed.value) return;
  const [origin, fork, original] = await Promise.all([
    api<ForkOrigin>(`/api/notes/${forkId}/fork`),
    api<ItemDetail>(`/api/notes/${forkId}`),
    api<ItemDetail>(`/api/notes/${id.value}`),
  ]);
  if (original.readonly) return void toast.add({ title: t('fork.originLocked'), color: 'warning' });

  const parts = mergeFork(origin.base, fork.content ?? '', original.content ?? '');
  const content = parts.every((part) => 'lines' in part) ? mergedText(parts) : await resolveConflict(parts, true);
  if (content === undefined) return; // Cancelled: nothing saved.
  let trash = chosen ?? me.value?.after_merge === 'delete';
  if (again && (me.value?.after_merge ?? 'ask') === 'ask') {
    const choice = await confirmMerge({
      fork: displayTitle(fork.item.title, t('untitled')),
      before: original.content ?? '',
      after: content,
      trash: false,
    });
    if (!choice) return;
    trash = choice.trash;
    if (choice.remember)
      me.value = await api<User>('/api/me', { method: 'PATCH', body: { after_merge: trash ? 'delete' : 'keep' } });
  }
  let note: ItemDetail;
  try {
    note = await api<ItemDetail>(`/api/notes/${forkId}/merge`, {
      method: 'POST',
      body: {
        content,
        base: fork.content ?? '',
        updated_at: original.item.updated_at ?? original.item.created_at,
        trash,
      },
      quiet: again ? [409] : false,
      conflict: {
        'trash or deleted': t('fork.originGone'),
        'read-only': t('fork.originLocked'),
        'changed elsewhere': t('fork.originChanged'),
      },
      tooLarge: t('note.tooLarge'),
    });
  } catch (error) {
    if (again && (error as FetchError).status === 409) return mergeIn(forkId, trash);
    return;
  }
  await showSaved(note);
  const undo = { label: t('fork.undo'), onClick: () => void unmerge(forkId) };
  toast.add({ title: t(trash ? 'fork.mergedTrashed' : 'fork.merged'), color: 'success', actions: [undo] });
}
// Undoes the last merge into this note, like git reset --hard HEAD~1 (#177): the note gets back its text from before
// it, kept in its history, and the fork its base and its place out of the trash, so the next merge offers again what
// that one refused. Refused (409) once the note was written since.
async function unmerge(forkId: string) {
  await saver.flush();
  let note: ItemDetail;
  try {
    note = await api<ItemDetail>(`/api/notes/${forkId}/unmerge`, {
      method: 'POST',
      conflict: {
        'no merge': t('fork.noMerge'),
        'trash or deleted': t('fork.originGone'),
        'read-only': t('fork.originLocked'),
        'changed elsewhere': t('fork.originWritten'),
      },
    });
  } catch {
    return;
  }
  await showSaved(note);
  toast.add({ title: t('fork.unmerged'), color: 'success' });
}
// The note as the API saved it, shown without being sent again; the sidebar, the related notes and the history follow.
async function showSaved(note: ItemDetail) {
  remember(note);
  tags.value = note.tags ?? [];
  links = linkedNotes(saved);
  title.value = note.item.title;
  touched.value = false;
  if (source.value !== undefined) source.value = saved;
  else editorRef.value?.editor?.commands.setContent(saved, { contentType: 'markdown' });
  await refreshNuxtData();
}
// The original's changes into its fork (#180), like a git rebase: the same three-way merge the other way, the fork keeping
// its title. The API saves it as a restore, the fork's text kept in its history, and moves the base to the original's
// text, so the next merge does not replay these changes, and drops the undo of the last merge. Changed meanwhile (409): again once.
async function rebase(again = true): Promise<void> {
  await saver.flush();
  if (failed.value) return;
  if (readonly.value) return void toast.add({ title: t('note.locked'), color: 'warning' });
  const [origin, fork] = await Promise.all([
    api<ForkOrigin>(`/api/notes/${id.value}/fork`),
    api<ItemDetail>(`/api/notes/${id.value}`),
  ]);
  const original = await api<ItemDetail>(`/api/notes/${origin.fork_of}`, { quiet: [404] }).catch((error) => {
    if ((error as FetchError).status !== 404) throw error;
  });
  if (!original) return void toast.add({ title: t('fork.rebaseGone'), color: 'error' });

  const parts = rebaseFork(origin.base, fork.content ?? '', original.content ?? '');
  const content = parts.every((part) => 'lines' in part) ? mergedText(parts) : await resolveConflict(parts, true);
  if (content === undefined) return; // Cancelled: nothing saved.
  let note: ItemDetail;
  try {
    note = await api<ItemDetail>(`/api/notes/${id.value}/rebase`, {
      method: 'POST',
      body: { content, base: original.content ?? '', updated_at: fork.item.updated_at ?? fork.item.created_at },
      quiet: again ? [409] : false,
      conflict: { 'read-only': t('note.locked'), 'changed elsewhere': t('fork.changed') },
      tooLarge: t('note.tooLarge'),
    });
  } catch (error) {
    if (again && (error as FetchError).status === 409) return rebase(false);
    return;
  }
  await showSaved(note);
  toast.add({ title: t('fork.rebased'), color: 'success' });
}
// A fork of the text as saved, beside the note, titled like a fork from the history: open to every plan, the history to Pro.
async function fork() {
  await saver.flush();
  if (failed.value) return;
  const note = await api<ItemDetail>(`/api/notes/${id.value}`);
  const date = new Date(note.item.updated_at ?? note.item.created_at).toLocaleString(locale.value, {
    dateStyle: 'medium',
    timeStyle: 'short',
  });
  const title = t('history.forkTitle', { title: displayTitle(note.item.title, t('untitled')), date });
  const item = await api<Item>(`/api/notes/${id.value}/forks`, {
    method: 'POST',
    body: { content: retitle(note.content ?? '', title) },
  });
  await navigateTo(`/notes/${item.id}`);
}
// Replaces the text as if typed.
function setText(content: string) {
  touched.value = true;
  if (source.value !== undefined) {
    source.value = content;
    return onSource(content);
  }
  editorRef.value?.editor?.commands.setContent(content, { contentType: 'markdown' });
  save(content); // The same text gives no update, yet must go: the API has another one after a conflict.
}

// The markdown as text, in place of the editor; undefined in the editor. Back in the editor, it starts from that text.
const source = ref<string>();
function toggleSource() {
  if (source.value !== undefined) {
    initialContent.value = source.value;
    source.value = undefined;
    return;
  }
  const markdown = editorRef.value?.editor?.getMarkdown();
  source.value = markdown === undefined ? initialContent.value : cleanMarkdown(markdown);
}
function onSource(text: string) {
  source.value = text;
  if (has('wordCount')) words.value = countWords(text);
  save(text.trim());
}

// The Cmd K palette renames the open note here, so the editor follows: its first heading takes the title, or one is added on top.
// TODO: the marks and #tags of that heading are replaced too, keep them if renamed headings lose tags.
const renaming = useState<(() => Promise<void>) | undefined>('rename-note');
async function rename() {
  const next = (
    await prompt({ title: t('items.renameNote'), value: displayTitle(title.value, ''), action: t('common.rename') })
  )?.trim();
  if (!next || next === title.value) return;
  if (readonly.value) return void toast.add({ title: t('note.locked'), color: 'warning' });
  touched.value = true;
  if (source.value !== undefined) return onSource(retitle(source.value, next));
  const editor = editorRef.value?.editor;
  if (!editor) return;
  const { tr, doc, schema } = editor.state;
  const first = outline.value[0];
  const node = first && doc.nodeAt(first.pos);
  if (first && node) tr.insertText(next, first.pos + 1, first.pos + node.nodeSize - 1);
  else tr.insert(0, schema.nodes.heading!.create({ level: 1 }, schema.text(next)));
  editor.view.dispatch(tr);
}
if (!props.beside) renaming.value = rename;
onBeforeUnmount(() => renaming.value === rename && (renaming.value = undefined));

function closeBody(changes: Changes) {
  return JSON.stringify(id.value ? { ...changes, updated_at: version } : { ...changes, parent_id: parent.value });
}
// A keepalive body is limited to 64 KiB (beacon too): beyond it the browser refuses the request.
const fitsKeepalive = (body: string) => new Blob([body]).size <= 64 * 1024;

function saveOnClose() {
  if (id.value && isEmpty.value) {
    saver.take();
    fetch(`${baseURL}/api/notes/${id.value}`, { method: 'DELETE', keepalive: true, credentials: 'include' });
    return;
  }
  if (saver.isBusy() && !id.value) return; // The note is being created: a second POST would duplicate it.
  const changes = saver.take();
  if (!changes || (!id.value && !changes.content)) return;

  const body = closeBody(changes);
  if (!fitsKeepalive(body)) {
    saver.push(changes);
    return void saver.flush(); // A plain request: the page hidden lives on, a closing tab was held by holdLongNote.
  }
  const url = `${baseURL}/api/notes${id.value ? `/${id.value}` : ''}`;
  // Refused or not sent, the changes go back in the queue: a page that lives on sends them again, its errors shown (#90).
  return fetch(url, {
    method: id.value ? 'PATCH' : 'POST',
    body,
    keepalive: true,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
  }).then(
    (response) => {
      if (!response.ok) saver.push(changes);
      return response;
    },
    () => void saver.push(changes),
  );
}
useEventListener(window, 'pagehide', saveOnClose);

// A closing tab cancels a plain request: for a note too long for keepalive, the browser asks to wait while it goes.
// TODO: iOS has no beforeunload, a note over 64 KiB closed from the app switcher within 1.5 s of typing loses those words; fix with a diff PATCH if it happens.
function holdLongNote(event: BeforeUnloadEvent) {
  const changes = saver.take();
  if (!changes) return;
  saver.push(changes);
  if (fitsKeepalive(closeBody(changes))) return;
  event.preventDefault();
  void saver.flush();
}
useEventListener(window, 'beforeunload', holdLongNote);

// iOS sends no pagehide on a lock or an app switch, and freezes the 1.5 s timer (#65). The page lives on:
// the answer gives the editor its new version, or the next PATCH would be stale. An emptied note is not trashed on a mere switch.
async function saveOnHide() {
  if (document.visibilityState !== 'hidden' || isEmpty.value || saver.isBusy()) return;
  const response = await saveOnClose();
  if (!response?.ok) return;
  if (id.value) return remember(await response.json());
  const item: Item = await response.json();
  id.value = item.id;
  emit('created', item.id);
  remember(await api<ItemDetail>(`/api/notes/${item.id}`));
}
useEventListener(document, 'visibilitychange', saveOnHide);

// Leaving the note: what is typed goes first, a save failed asks before it is lost.
async function leave() {
  if (isEmpty.value) {
    saver.take();
    return true;
  }
  await saver.flush();
  if (!failed.value) return true;
  const confirmed = await confirm({
    title: t('note.unsaved'),
    description: t('note.unsavedDescription'),
    action: t('note.leaveAnyway'),
  });
  return confirmed === true;
}
// The new note getting its address. Not the note of the second panel opened in the first: it goes there saved.
const beforeLeave = (to: RouteLocationNormalized) => (!props.beside && to.params.id === id.value) || leave();
onBeforeRouteLeave(beforeLeave);
onBeforeRouteUpdate(beforeLeave);

// The second panel has no route: its editor is replaced by key once left, an edit unsent would be lost with it.
if (props.beside) leaving.value = leave;
onBeforeUnmount(() => leaving.value === leave && (leaving.value = undefined));

// A note left empty goes to the trash, with its last saved content. Not while typing: clearing it to rewrite it must not close it.
onBeforeUnmount(async () => {
  if (!id.value || !isEmpty.value) return;
  await saver.flush(); // A save in progress would fail on a removed note.
  await removeItem({ id: id.value, kind: 'note' });
});

// The headings of the note, for the context panel: a click scrolls the text to one, without moving the cursor.
const panel = useContextPanel();
const panelDrawn = useState('context-drawn', () => false);
// Laid out by the server, only when the panel shows it.
const { data: graph, status: graphStatus } = useNoteGraph(() => (panel.value && has('graph') ? id.value : undefined));
// The graph of the previous note until this one's comes, rather than none (#154).
const lastGraph = useState<NotesGraph | null | undefined>('graph-shown');
watch(graph, (value) => value && (lastGraph.value = value), { immediate: true });
const nearGraph = computed(() => graph.value ?? (graphStatus.value === 'error' ? undefined : lastGraph.value));
const outline = ref<{ level: number; text: string; pos: number }[]>([]);
function readOutline(editor: Editor) {
  const headings: typeof outline.value = [];
  editor.state.doc.forEach((node, pos) => {
    if (node.type.name === 'heading' && node.textContent)
      headings.push({ level: node.attrs.level, text: node.textContent, pos });
  });
  outline.value = headings;
}
const words = ref(0);
function readText(editor: Editor) {
  readOutline(editor);
  if (has('wordCount')) words.value = countWords(editor.getText());
}
watch(
  () => editorRef.value?.editor,
  (editor) => {
    if (!editor) return;
    readText(editor);
    editor.on('update', () => readText(editor));
  },
);
function scrollTo(pos: number, behavior: ScrollBehavior = 'smooth') {
  (editorRef.value?.editor?.view.nodeDOM(pos) as HTMLElement | null)?.scrollIntoView({ behavior, block: 'start' });
}
// A [[Note#Heading]] link opens /notes/<id>#<heading>: the text scrolls to it once read, a frame later, once in the page.
const route = useRoute();
const stopHeading = watch(outline, (headings) => {
  const target = decodeURIComponent(route.hash.slice(1));
  const heading = headings.find((other) => sameHeading(other.text, target));
  if (!target || heading) stopHeading();
  if (heading) requestAnimationFrame(() => scrollTo(heading.pos, 'instant'));
});

const status = computed(() => {
  if (failed.value) return t('note.notSaved');
  if (isEmpty.value && id.value) return t('note.emptyWillTrash');
  if (saving.value) return t('note.saving');
  return '';
});
</script>

<template>
  <UEmpty
    v-if="notFound"
    :icon="icons.noteNotFound"
    :title="$t('note.notFound')"
    :description="$t('note.notFoundDescription')"
    :actions="[{ label: $t('common.backToNotes'), icon: icons.home, color: 'neutral', variant: 'subtle', to: '/' }]"
  />

  <!-- Only the text scrolls: a sticky toolbar inside the scroll moves with Safari's elastic bounce. -->
  <article
    v-else-if="!loading && ready"
    class="flex min-h-0 flex-1 flex-col"
    :class="{ 'pt-4': !beside }"
    :data-note-id="id"
    :data-beside="beside || undefined"
    @focusin="touched = true"
    @pointerdown.capture="touched = true"
  >
    <header
      v-if="beside"
      class="mb-4 flex shrink-0 items-center gap-1 border-b border-default py-1.5 pr-2 pl-5 text-sm"
    >
      <h2 class="truncate">{{ displayTitle(title, $t('untitled')) }}</h2>
      <UButton
        :icon="icons.close"
        color="neutral"
        variant="ghost"
        size="xs"
        class="ml-auto"
        :aria-label="$t('note.closeBeside')"
        @click="setBeside({ tabs: [], active: '' })"
      />
    </header>
    <!-- Lazy: CodeMirror is loaded at the first use of the source mode, not with the page. -->
    <LazyNoteSource
      v-if="source !== undefined"
      :model-value="source"
      autofocus
      :readonly="readonly"
      :label="$t('note.source')"
      @update:model-value="onSource"
    />
    <NoteWriter
      v-else
      ref="editor"
      :model-value="initialContent"
      :note-id="id"
      :readonly="readonly"
      :placeholder="$t('note.placeholder')"
      :autofocus="!noteId"
      @update:model-value="onContent"
      @touch="touched = true"
    />

    <footer class="shrink-0 border-t border-default px-8">
      <RelatedNotes v-if="id && !panel" :key="id" :note-id="id" />
      <div class="flex h-9 items-center gap-2 text-xs text-muted">
        <PathNav :path="path" />
        <span class="ml-auto shrink-0" :class="{ 'text-error': failed }" role="status">{{ status }}</span>
        <!-- The word count and the outline belong to the editor: hidden in the source mode. Dropping `source === undefined` shows the count, which follows the text; the outline would also need its headings read from the text. -->
        <span v-if="has('wordCount') && source === undefined" class="shrink-0 tabular-nums">{{
          $t('note.words', words)
        }}</span>
        <UTooltip v-if="id && has('noteLock')" :text="readonly ? $t('note.unlock') : $t('note.lock')">
          <UButton
            :icon="readonly ? icons.lock : icons.unlock"
            color="neutral"
            variant="ghost"
            size="xs"
            :aria-label="$t('note.lock')"
            :aria-pressed="readonly"
            @click="toggleLock"
          />
        </UTooltip>
        <NoteHistory
          v-if="id && has('noteHistory') && !panel"
          :note-id="id"
          :readonly="readonly"
          @restore="restoreVersion"
        />
        <UTooltip v-if="id" :text="$t('fork.create')">
          <UButton
            :icon="icons.fork"
            color="neutral"
            variant="ghost"
            size="xs"
            :aria-label="$t('fork.create')"
            @click="fork()"
          />
        </UTooltip>
        <UDropdownMenu
          v-if="neighbors?.forks.length"
          :ui="{ content: 'max-w-72' }"
          :items="
            neighbors.forks.map((fork) => ({
              label: displayTitle(fork.title, $t('untitled')),
              onSelect: () => void mergeIn(fork.id),
            }))
          "
        >
          <UTooltip :text="$t('fork.merge')">
            <UButton :icon="icons.merge" color="neutral" variant="ghost" size="xs" :aria-label="$t('fork.merge')" />
          </UTooltip>
        </UDropdownMenu>
        <UTooltip v-if="neighbors?.merged" :text="$t('fork.unmerge')">
          <UButton
            :icon="icons.undo"
            color="neutral"
            variant="ghost"
            size="xs"
            :aria-label="$t('fork.unmerge')"
            @click="unmerge(neighbors.merged)"
          />
        </UTooltip>
        <UTooltip v-if="neighbors?.original.length" :text="$t('fork.rebase')">
          <UButton
            :icon="icons.rebase"
            color="neutral"
            variant="ghost"
            size="xs"
            :aria-label="$t('fork.rebase')"
            @click="rebase()"
          />
        </UTooltip>
        <UTooltip v-if="has('sourceMode')" :text="source === undefined ? $t('note.source') : $t('note.editor')">
          <UButton
            :icon="icons.source"
            color="neutral"
            variant="ghost"
            size="xs"
            :aria-label="$t('note.source')"
            :aria-pressed="source !== undefined"
            @click="toggleSource"
          />
        </UTooltip>
        <UTooltip
          v-if="has('focusMode')"
          :text="focus ? $t('note.unfocus') : $t('note.focus')"
          :kbds="shortcuts.focusMode"
        >
          <UButton
            :icon="focus ? icons.unfocus : icons.focus"
            color="neutral"
            variant="ghost"
            size="xs"
            :aria-label="focus ? $t('note.unfocus') : $t('note.focus')"
            :aria-pressed="focus"
            class="hidden lg:inline-flex"
            @click="focus = !focus"
          />
        </UTooltip>
      </div>
    </footer>

    <!-- Drawn by the layout beside the note, like the sidebar. -->
    <Teleport v-if="panel && panelDrawn" to="#context-panel" defer>
      <nav v-if="source === undefined" :aria-label="$t('note.outline')">
        <h2 class="mb-2 text-xs font-medium text-muted">{{ $t('note.outline') }}</h2>
        <p v-if="!outline.length" class="text-xs text-dimmed">{{ $t('note.noOutline') }}</p>
        <button
          v-for="heading in outline"
          :key="heading.pos"
          type="button"
          class="block w-full truncate rounded py-1 text-left text-sm text-muted hover:text-default"
          :style="{ paddingLeft: `${(heading.level - 1) * 0.75}rem` }"
          @click="scrollTo(heading.pos)"
        >
          {{ heading.text }}
        </button>
      </nav>
      <RelatedNotes v-if="id" :key="id" :note-id="id" panel />
      <NoteHistory
        v-if="id && has('noteHistory')"
        :key="id"
        :note-id="id"
        :readonly="readonly"
        :saved-at="savedAt"
        panel
        @restore="restoreVersion"
      />
      <!-- At the bottom of the panel, kept in sight while the rest scrolls. -->
      <section
        v-if="has('graph') && nearGraph && nearGraph.notes.length > 1"
        class="sticky bottom-0 mt-auto -mb-4 bg-default pb-4"
      >
        <div class="mb-2 flex items-center justify-between">
          <h2 class="text-xs font-medium text-muted">{{ $t('common.graph') }}</h2>
        </div>
        <GraphView :graph="nearGraph" :selected="id" mini class="h-56!" />
      </section>
    </Teleport>
  </article>
</template>
