<script setup lang="ts">
import type { Tab } from '~/composables/useTabs';

const { icons } = useIcons();
const { has } = usePro();
const { t } = useI18n();
const tabs = useTabs();
const { dragged } = tabs;
const { openMenu, besideClick, panels } = useSplit();

// The tabs of one panel of the editor (#253); the current panel's open tab is underlined in the accent, the others' in grey.
const props = defineProps<{ panel: string }>();
const own = computed(() => tabs.layout.value.panels.find((panel) => panel.id === props.panel)!);
const isCurrent = computed(() => tabs.layout.value.current === props.panel);
const label = (tab: Tab) => displayTitle(tab.title, t('untitled'));

// ⌘ + click opens the tab beside. Capture: the link's own click navigates first. The panel becomes the current one.
function click(id: string, event: MouseEvent) {
  besideClick(id, props.panel)(event);
  if (event.defaultPrevented) return;
  event.preventDefault();
  tabs.select(props.panel, id);
}

// A tab dragged onto another takes its place; from another panel's bar, it moves to this panel, after the open tab.
function drop(index?: number) {
  const from = dragged.value;
  dragged.value = undefined;
  if (!from) return;
  if (from.panel !== props.panel) return tabs.openIn(props.panel, from.id, true);
  const at = own.value.tabs.findIndex((tab) => tab.id === from.id);
  if (index !== undefined) tabs.reorder(props.panel, moveTab(own.value.tabs, at, index));
}

// A single note in a single panel is no tab: the bar shows from a second tab or a second panel.
const shown = computed(() => has('tabs') && (own.value.tabs.length > 1 || panels.value.length > 1 || !!dragged.value));
</script>

<template>
  <nav
    v-if="shown"
    :aria-label="$t('note.tabs')"
    class="flex shrink-0 border-b border-default px-2"
    @dragover.prevent
    @drop.prevent="drop()"
  >
    <div class="flex min-w-0 flex-1 overflow-x-auto">
      <!-- A middle click closes the tab, as in a browser, instead of opening the note in a new one. The close button of
           the other tabs shows on hover, on a wide screen only: a touch screen has no hover. -->
      <UContextMenu
        v-for="(tab, index) in own.tabs"
        :key="tab.id"
        :items="[
          ...openMenu(tab.id, panel),
          [
            {
              label: $t('note.closeTab', { title: label(tab) }),
              icon: icons.close,
              onSelect: () => tabs.closeTab(tab.id),
            },
            {
              label: $t('note.closeOthers'),
              icon: icons.closeOthers,
              onSelect: () => tabs.closeOthers(panel, tab.id),
            },
          ],
        ]"
      >
        <div
          draggable="true"
          class="group -mb-px flex max-w-48 shrink-0 items-center border-b-2"
          :class="[
            tab.id !== own.active
              ? 'border-transparent text-muted hover:text-default'
              : isCurrent
                ? 'border-primary text-default'
                : 'border-accented text-default',
            { 'opacity-50': dragged?.id === tab.id },
          ]"
          @auxclick.middle.prevent="tabs.closeTab(tab.id)"
          @dragstart="dragged = { id: tab.id, panel }"
          @dragend="dragged = undefined"
          @dragover.prevent
          @drop.prevent.stop="drop(index)"
        >
          <NuxtLink
            :to="`/notes/${tab.id}`"
            :aria-current="tab.id === own.active ? (isCurrent ? 'page' : 'true') : undefined"
            draggable="false"
            class="truncate py-2 pr-1 pl-3 text-sm"
            @click.capture="click(tab.id, $event)"
          >
            {{ label(tab) }}
          </NuxtLink>
          <UButton
            :icon="icons.close"
            color="neutral"
            variant="ghost"
            size="xs"
            class="lg:group-hover:opacity-100 lg:focus-visible:opacity-100"
            :class="{ 'lg:opacity-0': tab.id !== own.active }"
            :aria-label="$t('note.closeTab', { title: label(tab) })"
            @click="tabs.closeTab(tab.id)"
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
        @click="tabs.closeAll(panel)"
      />
    </UTooltip>
  </nav>
</template>
