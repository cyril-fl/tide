<script setup lang="ts">
import type { Tab } from '~/composables/useTabs';

const { icons } = useIcons();
const { has } = usePro();
const { t } = useI18n();
const route = useRoute();
const { tabs } = useTabs();
const { besideMenu } = useSplit();
// The focus mode leaves only the note on screen: the tabs go with the sidebar.
const focus = useState('focus', () => false);

const open = computed(() => route.params.id as string);
const label = (tab: Tab) => displayTitle(tab.title, t('untitled'));

// The open note's tab closed opens its neighbour; a leave refused (a note not saved) keeps the tab.
async function close(id: string) {
  if (id === open.value) {
    const next = tabAfterClose(
      tabs.value.map((tab) => tab.id),
      id,
    );
    if ((await navigateTo(next ? `/notes/${next}` : '/')) !== undefined) return;
  }
  tabs.value = tabs.value.filter((tab) => tab.id !== id);
}

const shown = computed(() => has('tabs') && tabs.value.length > 0 && !(focus.value && has('focusMode')));
</script>

<template>
  <nav v-if="shown" :aria-label="$t('note.tabs')" class="flex shrink-0 overflow-x-auto border-b border-default px-2">
    <!-- A middle click closes the tab, as in a browser, instead of opening the note in a new one. The close button of
         the other tabs shows on hover, on a wide screen only: a touch screen has no hover. -->
    <UContextMenu
      v-for="tab in tabs"
      :key="tab.id"
      :items="[
        ...besideMenu(tab.id),
        [{ label: $t('note.closeTab', { title: label(tab) }), icon: icons.close, onSelect: () => close(tab.id) }],
      ]"
    >
      <div
        class="group -mb-px flex max-w-48 shrink-0 items-center border-b-2"
        :class="tab.id === open ? 'border-primary text-default' : 'border-transparent text-muted hover:text-default'"
        @auxclick.middle.prevent="close(tab.id)"
      >
        <NuxtLink
          :to="`/notes/${tab.id}`"
          :aria-current="tab.id === open ? 'page' : undefined"
          class="truncate py-2 pr-1 pl-3 text-sm"
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
  </nav>
</template>
