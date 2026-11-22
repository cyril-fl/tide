<script setup lang="ts">
import type { User } from '~/types/items';

const route = useRoute();
const api = useApi();
const { setLocale } = useI18n();
const { createFolder, undo, redo } = useItems();
const { key } = useShortcuts();
const searchOpen = useState('search-open', () => false);
const searchSnippets = useState('search-snippets', () => false);
const collapsed = ref(false);

// The focus mode leaves only the note: it ends with the page, or with the Pro mode. Wide screens only: below, the sidebar is already a closed drawer.
const { has } = usePro();
const focus = useState('focus', () => false);
const wide = useMediaQuery('(min-width: 1024px)');
const focused = computed(() => focus.value && has('focusMode') && wide.value);
watch(
  () => route.path,
  () => (focus.value = false),
);
// The note's editor fills it (a Teleport to #context-panel).
const contextPanel = useContextPanel();
const notePanel = computed(() => contextPanel.value && route.path.startsWith('/notes/'));
// The aside drawn: a note page shown before the layout follows the route would teleport into nothing.
const contextDrawn = useState('context-drawn', () => false);

// The account's language wins over the cookie's, which only remembers this browser's last one.
const { data: me } = useMe();
watch(
  () => me.value?.locale,
  (locale) => locale && setLocale(locale as 'fr' | 'en'),
  { immediate: true }, // me may be loaded already, by the settings page's middleware
);

// Theme, mode, view and sort follow the account; a change of mode, view or sort from anywhere (sidebar, folder header,
// settings) is saved to it.
const appConfig = useAppConfig();
const colorMode = useColorMode();
const view = useView();
const sort = useSort();
watch(
  me,
  (user) => {
    if (!user) return;
    const theme = themes.find((theme) => theme.id === user.theme) ?? themes[0]!;
    const { id: _, ...colors } = theme;
    Object.assign(appConfig.ui.colors, colors);
    colorMode.preference = user.color_mode;
    view.value = user.view as 'list' | 'grid';
    sort.value = user.sort;
  },
  { immediate: true },
);
watch([() => colorMode.preference, view, sort], async ([mode, next, order]) => {
  if (!me.value || (mode === me.value.color_mode && next === me.value.view && order === me.value.sort)) return;
  me.value = await api<User>('/api/me', { method: 'PATCH', body: { color_mode: mode, view: next, sort: order } });
});

// Option (Alt) + a letter: not a single key a stray hit fires; Cmd+N, Cmd+Shift+N and Cmd+T are the browser's.
// Nuxt UI matches alt shortcuts on e.code, so the Mac's dead keys (Option+N types ˜) still match. It skips them while typing in a field or the editor.
// Not in an open menu either (the / menu, the right click): it takes the focus out of the editor, the keys are for it.
const outsideMenu = (handler: () => unknown) => () => document.activeElement?.closest('[role="menu"]') || handler();
defineShortcuts({
  [key('searchSlash')]: outsideMenu(() => (searchOpen.value = true)),
  [key('newNote')]: outsideMenu(() => navigateTo(newNotePath(currentFolder(route)))),
  [key('newFolder')]: outsideMenu(() => createFolder(currentFolder(route))),
  [key('fromSnippet')]: outsideMenu(() => (searchSnippets.value = searchOpen.value = true)),
  [key('toggleSidebar')]: () => (collapsed.value = !collapsed.value),
  [key('focusMode')]: { usingInput: true, handler: () => has('focusMode') && (focus.value = !focus.value) },
  // Outside a field or the editor, which undo their own text.
  [key('undo')]: undo,
  [key('redo')]: redo,
  [key('redoShift')]: redo,
});
// The template: Nuxt UI's dashboard, a sidebar that resizes (its width kept in localStorage), collapses, and becomes a drawer on small screens.
</script>

<template>
  <UDashboardGroup unit="rem" storage="local" storage-key="notes-layout" class="gap-2 bg-muted p-2 text-default">
    <UDashboardSidebar
      v-if="!focused"
      v-model:collapsed="collapsed"
      collapsible
      resizable
      :min-size="12"
      :default-size="16"
      :max-size="28"
      :collapsed-size="3.5"
      :ui="{
        root: 'h-full min-h-0 min-w-0 overflow-hidden rounded-xl border-0 bg-default',
        body: 'gap-0 p-0',
      }"
    >
      <template #default="{ collapsed: rail }">
        <AppSidebar :collapsed="rail" />
      </template>
    </UDashboardSidebar>

    <UDashboardPanel
      :ui="{
        root: 'h-full min-h-0 flex-1 overflow-hidden rounded-xl border-0 bg-default',
        body: 'gap-0 p-0 sm:gap-0 sm:p-0',
      }"
    >
      <template #body>
        <!-- Only on a small screen, where the sidebar is a drawer. -->
        <div v-if="!focused" class="flex px-2 pt-2 lg:hidden">
          <UDashboardSidebarToggle color="neutral" variant="ghost" />
        </div>
        <main class="flex min-h-0 flex-1 flex-col">
          <slot />
        </main>
      </template>
    </UDashboardPanel>

    <!-- Not collapsible: the sidebar's collapse button would fold it too. -->
    <UDashboardSidebar
      v-if="notePanel"
      id="context"
      side="right"
      resizable
      :min-size="12"
      :default-size="16"
      :max-size="28"
      :ui="{
        root: 'h-full min-h-0 min-w-0 overflow-hidden rounded-xl border-0 bg-default',
        body: 'gap-0 p-0',
      }"
    >
      <aside
        id="context-panel"
        :ref="(el) => (contextDrawn = !!el)"
        class="flex flex-1 flex-col gap-6 p-4"
        :aria-label="$t('features.contextPanel')"
      />
    </UDashboardSidebar>

    <AppSearch />
  </UDashboardGroup>
</template>
