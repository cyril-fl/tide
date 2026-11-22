<script setup lang="ts">
import type { Tab } from '~/composables/useTabs';

const { icons } = useIcons();
const { has } = usePro();
const { t } = useI18n();
const route = useRoute();
const { tabs, besidePanel, setBeside } = useTabs();
const { openMenu, besideClick } = useSplit();
// The focus mode leaves only the note on screen: the tabs go with the sidebar.
const focus = useState('focus', () => false);

// beside: the tabs of the second panel of the split view, which has no route (#242).
const props = defineProps<{ beside?: boolean }>();
const list = computed({
  get: () => (props.beside ? besidePanel.value.tabs : tabs.value),
  set: (next) => (props.beside ? (besidePanel.value = { ...besidePanel.value, tabs: next }) : (tabs.value = next)),
});
const open = computed(() => (props.beside ? besidePanel.value.active : (route.params.id as string)));
const label = (tab: Tab) => displayTitle(tab.title, t('untitled'));

// A tab of the second panel opens there, ⌘ + click in the first one.
function click(id: string, event: MouseEvent) {
  besideClick(id)(event);
  if (!props.beside || event.defaultPrevented) return;
  event.preventDefault();
  setBeside({ ...besidePanel.value, active: id });
}

// The open note's tab closed opens its neighbour; a leave refused (a note not saved) keeps the tab.
async function close(id: string) {
  if (props.beside) return setBeside(withoutTab(besidePanel.value, id));
  if (id === open.value) {
    const next = tabAfterClose(
      tabs.value.map((tab) => tab.id),
      id,
    );
    if ((await navigateTo(next ? `/notes/${next}` : '/')) !== undefined) return;
  }
  tabs.value = tabs.value.filter((tab) => tab.id !== id);
}

// Every other tab closed opens the one kept; all of them closed leaves the notes for the home page.
async function closeOthers(id: string) {
  if (props.beside) return setBeside({ tabs: list.value.filter((tab) => tab.id === id), active: id });
  if (id !== open.value && (await navigateTo(`/notes/${id}`)) !== undefined) return;
  tabs.value = tabs.value.filter((tab) => tab.id === id);
}
async function closeAll() {
  if (props.beside) return setBeside({ tabs: [], active: '' });
  if ((await navigateTo('/')) === undefined) tabs.value = [];
}

// A tab dragged onto another takes its place.
const dragged = ref<number>();
function drop(index: number) {
  if (dragged.value !== undefined) list.value = moveTab(list.value, dragged.value, index);
  dragged.value = undefined;
}

// One note open is no tab: the bar shows from the second one.
const shown = computed(() => has('tabs') && list.value.length > 1 && !(focus.value && has('focusMode')));
</script>

<template>
  <nav
    v-if="shown"
    :aria-label="beside ? $t('note.besideTabs') : $t('note.tabs')"
    class="flex shrink-0 border-b border-default px-2"
  >
    <div class="flex min-w-0 flex-1 overflow-x-auto">
      <!-- A middle click closes the tab, as in a browser, instead of opening the note in a new one. The close button of
           the other tabs shows on hover, on a wide screen only: a touch screen has no hover. -->
      <UContextMenu
        v-for="(tab, index) in list"
        :key="tab.id"
        :items="[
          ...openMenu(tab.id, beside),
          [
            { label: $t('note.closeTab', { title: label(tab) }), icon: icons.close, onSelect: () => close(tab.id) },
            { label: $t('note.closeOthers'), icon: icons.closeOthers, onSelect: () => closeOthers(tab.id) },
          ],
        ]"
      >
        <div
          draggable="true"
          class="group -mb-px flex max-w-48 shrink-0 items-center border-b-2"
          :class="[
            tab.id === open ? 'border-primary text-default' : 'border-transparent text-muted hover:text-default',
            { 'opacity-50': dragged === index },
          ]"
          @auxclick.middle.prevent="close(tab.id)"
          @dragstart="dragged = index"
          @dragend="dragged = undefined"
          @dragover.prevent
          @drop.prevent="drop(index)"
        >
          <NuxtLink
            :to="`/notes/${tab.id}`"
            :aria-current="tab.id === open ? (beside ? 'true' : 'page') : undefined"
            draggable="false"
            class="truncate py-2 pr-1 pl-3 text-sm"
            @click="click(tab.id, $event)"
          >
            {{ label(tab) }}
          </NuxtLink>
          <UButton
            :icon="icons.close"
            color="neutral"
            variant="ghost"
            size="xs"
            class="lg:group-hover:opacity-100 lg:focus-visible:opacity-100"
            :class="{ 'lg:opacity-0': tab.id !== open }"
            :aria-label="$t('note.closeTab', { title: label(tab) })"
            @click="close(tab.id)"
          />
        </div>
      </UContextMenu>
    </div>
    <UTooltip :text="$t('note.closeAll')">
      <UButton
        :icon="icons.close"
        color="neutral"
        variant="ghost"
        size="xs"
        class="my-auto ml-1 shrink-0"
        :aria-label="$t('note.closeAll')"
        @click="closeAll"
      />
    </UTooltip>
  </nav>
</template>
