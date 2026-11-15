// A page of a feature the account lacks, definePageMeta({ feature }), sends home: also when its URL is typed by hand.
export default defineNuxtRouteMiddleware(async (to) => {
  const feature = to.meta.feature as Feature | undefined;
  if (!feature) return;
  const me = await loadMe();
  if (me && !me.features.includes(feature)) return navigateTo('/');
});
