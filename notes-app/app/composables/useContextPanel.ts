/** The context panel of a note: a Pro feature, beside the note on a wide screen, hidden by the focus mode and by the split view, too narrow for three. */
export function useContextPanel() {
  const { has } = usePro();
  const focus = useState('focus', () => false);
  const wide = useMediaQuery('(min-width: 1024px)');
  const split = useSplit();
  return computed(() => has('contextPanel') && !(focus.value && has('focusMode')) && wide.value && !split.shown.value);
}
