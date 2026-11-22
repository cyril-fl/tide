/**
 * Click and drag on the empty space around a list (mouse only) to select the rows the square touches; ⌘, Ctrl or ⇧
 * keeps the selection already made. The rows carry their id in `data-id`. Returns the square's style while dragging.
 */
export function useLasso(list: Ref<HTMLElement | null>, enabled: () => boolean = () => true) {
  const selection = useSelection();
  const square = ref<Record<string, string> | null>(null);
  let from: { x: number; y: number } | null = null;
  let base: string[] = [];

  useEventListener(
    () => list.value?.parentElement,
    'pointerdown',
    (event: PointerEvent) => {
      const target = event.target as HTMLElement;
      if (!enabled() || event.pointerType !== 'mouse' || event.button !== 0) return;
      if (target.closest('li, a, button, input, textarea, [role=button]')) return;
      from = { x: event.clientX, y: event.clientY };
      base = event.metaKey || event.ctrlKey || event.shiftKey ? selection.value : [];
      if (!base.length) selection.value = [];
    },
  );

  useEventListener(window, 'pointermove', (event: PointerEvent) => {
    if (!from || !list.value) return;
    const left = Math.min(from.x, event.clientX);
    const top = Math.min(from.y, event.clientY);
    const right = Math.max(from.x, event.clientX);
    const bottom = Math.max(from.y, event.clientY);
    if (!square.value && right - left < 4 && bottom - top < 4) return;
    square.value = { left: `${left}px`, top: `${top}px`, width: `${right - left}px`, height: `${bottom - top}px` };
    // TODO: only the rows rendered are seen, the pages not in sight are not (no scroll while dragging), when a long list needs it.
    const hit = [...list.value.querySelectorAll<HTMLElement>('li[data-id]')]
      .filter((li) => {
        const box = li.getBoundingClientRect();
        return box.left < right && box.right > left && box.top < bottom && box.bottom > top;
      })
      .map((li) => li.dataset.id!);
    selection.value = [...new Set([...base, ...hit])];
  });

  const stop = () => {
    from = null;
    square.value = null;
  };
  useEventListener(window, 'pointerup', stop);
  useEventListener(window, 'pointercancel', stop);
  // No text selected under the square.
  useEventListener(document, 'selectstart', (event) => from && event.preventDefault());

  return square;
}
