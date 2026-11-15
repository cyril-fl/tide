/** The plan of the account, and has(feature) to show a feature only where it is available. ready: the account is loaded, has() is final. */
export function usePro() {
  const { data: me } = useMe();
  const plan = computed(() => me.value?.plan ?? 'base');
  // The API says which features the plan has: the app only follows.
  const has = (feature: Feature) => !!me.value?.features.includes(feature);
  const ready = computed(() => !!me.value);
  return { plan, has, ready };
}
